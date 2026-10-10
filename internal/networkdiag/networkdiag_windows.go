//go:build windows

package networkdiag

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const firewallRuleName = "JACoB Local Network"

func psQuote(v string) string {
	return "'" + strings.ReplaceAll(v, "'", "''") + "'"
}

func Inspect(exe string, port int) Report {
	report := Report{Supported: true}
	script := fmt.Sprintf(`
$ErrorActionPreference='SilentlyContinue'
$wantProgram=%s
$wantPort=%d
$profiles=@(Get-NetConnectionProfile | Where-Object {$_.IPv4Connectivity -ne 'Disconnected'} | ForEach-Object {
  [pscustomobject]@{name=$_.Name;interfaceAlias=$_.InterfaceAlias;networkCategory=[string]$_.NetworkCategory;ipv4Connectivity=[string]$_.IPv4Connectivity}
})
$rule=Get-NetFirewallRule -DisplayName %s | Select-Object -First 1
$present=$null -ne $rule
$enabled=$false
$profileText=''
$program=''
$localPort=''
$remote=@()
$portOK=$false
$programOK=$false
$remoteOK=$false
$profileOK=$false
$directionOK=$false
$actionOK=$false
if($present){
  $enabled=([string]$rule.Enabled -eq 'True')
  $profileText=[string]$rule.Profile
  $directionOK=([string]$rule.Direction -eq 'Inbound')
  $actionOK=([string]$rule.Action -eq 'Allow')
  $pf=$rule | Get-NetFirewallPortFilter | Select-Object -First 1
  if($pf){$localPort=[string]$pf.LocalPort;$portOK=([string]$pf.Protocol -eq 'TCP' -and $localPort -eq [string]$wantPort)}
  $af=$rule | Get-NetFirewallApplicationFilter | Select-Object -First 1
  if($af){$program=[string]$af.Program;$programOK=($program -and ([IO.Path]::GetFullPath($program) -ieq [IO.Path]::GetFullPath($wantProgram)))}
  $rf=$rule | Get-NetFirewallAddressFilter | Select-Object -First 1
  if($rf){$remote=@($rf.RemoteAddress | ForEach-Object {[string]$_});$remoteOK=($remote -contains 'LocalSubnet')}
  $profileOK=(($profileText -match 'Private' -or $profileText -match 'Domain') -and $profileText -notmatch 'Public' -and $profileText -notmatch 'Any')
}
[pscustomobject]@{
  supported=$true
  firewallRulePresent=$present
  firewallOK=($present -and $enabled -and $directionOK -and $actionOK -and $portOK -and $programOK -and $remoteOK -and $profileOK)
  firewallEnabled=$enabled
  firewallProfiles=$profileText
  firewallProgram=$program
  firewallPort=$localPort
  firewallRemote=$remote
  profiles=$profiles
} | ConvertTo-Json -Depth 5 -Compress
`, psQuote(exe), port, psQuote(firewallRuleName))
	out, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script).Output()
	if err != nil {
		report.Error = err.Error()
		return report
	}
	if err := json.Unmarshal(out, &report); err != nil {
		report.Error = "parse Windows network diagnostics: " + err.Error()
	}
	report.Supported = true
	return report
}

func elevatedScript(body string) error {
	f, err := os.CreateTemp("", "jacob-network-*.ps1")
	if err != nil {
		return err
	}
	path := f.Name()
	defer os.Remove(path)
	if _, err = f.WriteString(body); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	abs, _ := filepath.Abs(path)
	command := fmt.Sprintf(`$a='-NoProfile -NonInteractive -ExecutionPolicy Bypass -File "' + %s + '"'; $p=Start-Process powershell.exe -Verb RunAs -Wait -PassThru -ArgumentList $a; exit $p.ExitCode`, psQuote(abs))
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", command)
	if out, runErr := cmd.CombinedOutput(); runErr != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return fmt.Errorf("network repair failed: %s", msg)
		}
		return fmt.Errorf("network repair failed: %w", runErr)
	}
	return nil
}

func EnsureInteractive(exe string, port int) error {
	if port <= 0 || port > 65535 {
		return fmt.Errorf("invalid port %d", port)
	}
	if r := Inspect(exe, port); r.FirewallOK {
		return nil
	}
	body := fmt.Sprintf(`$ErrorActionPreference='Stop'
$ruleName=%s
Get-NetFirewallRule -DisplayName $ruleName -ErrorAction SilentlyContinue | Remove-NetFirewallRule -ErrorAction SilentlyContinue
New-NetFirewallRule -DisplayName $ruleName -Group 'JACoB' -Direction Inbound -Action Allow -Program %s -Protocol TCP -LocalPort %s -RemoteAddress LocalSubnet -Profile Domain,Private -EdgeTraversalPolicy Block | Out-Null
`, psQuote(firewallRuleName), psQuote(exe), strconv.Itoa(port))
	if err := elevatedScript(body); err != nil {
		return err
	}
	r := Inspect(exe, port)
	if !r.FirewallOK {
		if r.Error != "" {
			return fmt.Errorf("firewall rule was not verified: %s", r.Error)
		}
		return fmt.Errorf("firewall rule was created but did not match the required JACoB LAN scope")
	}
	return nil
}

func RemoveInteractive() error {
	r := Inspect("", 6626)
	if !r.FirewallRulePresent {
		return nil
	}
	body := fmt.Sprintf(`$ErrorActionPreference='SilentlyContinue'
Get-NetFirewallRule -DisplayName %s | Remove-NetFirewallRule
`, psQuote(firewallRuleName))
	return elevatedScript(body)
}
