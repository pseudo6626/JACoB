$ErrorActionPreference = "Stop"

$bindDir = Join-Path $env:LOCALAPPDATA "Frontier Developments\Elite Dangerous\Options\Bindings"

if (-not (Test-Path $bindDir)) {
    throw "Elite Dangerous bindings directory not found: $bindDir"
}

if (Get-Process EliteDangerous64 -ErrorAction SilentlyContinue) {
    throw "Elite Dangerous is running. Close Elite before restoring bindings."
}

$jacobProcesses = @(Get-Process -ErrorAction SilentlyContinue | Where-Object { $_.ProcessName -like "jacob*" })
if ($jacobProcesses.Count -gt 0) {
    throw "JACoB is still running. Close it before restoring bindings."
}

$backups = @(
    Get-ChildItem $bindDir -File |
    Where-Object {
        $_.Name -match '\.binds\.jacob-backup-\d{8}-\d{6}(?:\.\d{3})?$'
    } |
    Sort-Object LastWriteTime
)

if ($backups.Count -eq 0) {
    Write-Host "No JACoB backup files were found." -ForegroundColor Yellow
    Write-Host "Looking for older backup files created by the earlier binding scripts..."

    $backups = @(
        Get-ChildItem $bindDir -File |
        Where-Object {
            $_.Name -match '\.binds\.backup-\d{8}-\d{6}$'
        } |
        Sort-Object LastWriteTime -Descending
    )
}

if ($backups.Count -eq 0) {
    throw "No JACoB/EDBridge .binds backups were found in $bindDir"
}

# Group backups by the original .binds filename and restore the OLDEST JACoB
# backup for each file. That is the copy from before JACoB first modified it,
# which is safer if more than one prototype version touched the same preset.
$groups = @{}

foreach ($backup in $backups) {
    $originalName = $backup.Name `
        -replace '\.jacob-backup-\d{8}-\d{6}(?:\.\d{3})?$','' `
        -replace '\.backup-\d{8}-\d{6}$',''

    if (-not $groups.ContainsKey($originalName)) {
        $groups[$originalName] = $backup
    }
}

$stamp = Get-Date -Format "yyyyMMdd-HHmmss"

Write-Host ""
Write-Host "Elite bindings recovery" -ForegroundColor Cyan
Write-Host "Directory: $bindDir"
Write-Host ""

foreach ($originalName in ($groups.Keys | Sort-Object)) {
    $backup = $groups[$originalName]
    $target = Join-Path $bindDir $originalName

    Write-Host "Restoring $originalName" -ForegroundColor Cyan
    Write-Host "  from: $($backup.Name)"

    if (Test-Path $target) {
        $damagedCopy = "$target.pre-recovery-$stamp"
        Move-Item $target $damagedCopy -Force
        Write-Host "  current file preserved as: $(Split-Path $damagedCopy -Leaf)"
    }

    Copy-Item $backup.FullName $target -Force
    Write-Host "  restored." -ForegroundColor Green
    Write-Host ""
}

foreach ($selectorName in @("StartPreset.4.start", "StartPreset.start")) {
    $selector = Join-Path $bindDir $selectorName
    if (Test-Path $selector) {
        Write-Host "$selectorName was left untouched:" -ForegroundColor Green
        Get-Content $selector | ForEach-Object { Write-Host "  $_" }
    }
}

Write-Host ""
Write-Host "Recovery complete." -ForegroundColor Green
Write-Host "Start Elite Dangerous and check Controls before running JACoB again."
