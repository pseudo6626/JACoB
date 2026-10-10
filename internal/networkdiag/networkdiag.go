package networkdiag

// Profile describes an active Windows network profile.
type Profile struct {
	Name             string `json:"name"`
	InterfaceAlias   string `json:"interfaceAlias"`
	NetworkCategory  string `json:"networkCategory"`
	IPv4Connectivity string `json:"ipv4Connectivity"`
}

// Report contains the host-side network checks relevant to JACoB LAN access.
type Report struct {
	Supported           bool      `json:"supported"`
	FirewallRulePresent bool      `json:"firewallRulePresent"`
	FirewallOK          bool      `json:"firewallOK"`
	FirewallEnabled     bool      `json:"firewallEnabled"`
	FirewallProfiles    string    `json:"firewallProfiles,omitempty"`
	FirewallProgram     string    `json:"firewallProgram,omitempty"`
	FirewallPort        string    `json:"firewallPort,omitempty"`
	FirewallRemote      []string  `json:"firewallRemote,omitempty"`
	Profiles            []Profile `json:"profiles,omitempty"`
	Error               string    `json:"error,omitempty"`
}
