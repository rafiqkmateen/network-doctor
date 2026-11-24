package fix

// DNSState is one adapter's name servers: the servers it uses now, or that it takes them
// from the router (Automatic).
type DNSState struct {
	Adapter   string   `json:"adapter"`
	IfIndex   int      `json:"if_index"`
	Servers   []string `json:"servers"`
	Automatic bool     `json:"automatic"`
	// InUse is true for the adapter that carries the connection (the default route on an adapter that is up).
	InUse bool `json:"in_use"`
	// IgnoreAuto, on Linux only, says whether the router's servers are left out (NetworkManager's
	// ipv4.ignore-auto-dns), so an undo puts that back exactly too. Nil where it is not read.
	IgnoreAuto *bool `json:"ignore_auto_dns,omitempty"`
	// ConnUUID, on Linux only, is the unique id of the NetworkManager connection, so nmcli is told
	// exactly which connection to change even when two have the same name.
	ConnUUID string `json:"conn_uuid,omitempty"`
}

// ExtraSystem is what the Windows system adds for the round-two fixes (ND-235 to ND-237).
// A system without it refuses those fixes, as on Mac and Linux (ND-238). Every step that needs
// administrator rights is checked by reading the result back (ND-243).
type ExtraSystem interface {
	// DNS reads the name servers of the adapters in use.
	DNS() ([]DNSState, error)
	// SetDNS sets the name servers of the given adapters and reads them back; it fails when
	// the result is not there.
	SetDNS([]DNSState) error
	// Renew asks the router for a new address for the adapter in use and reads it back.
	Renew() error
	// ResetAdapter switches the adapter in use off and on and checks it came back.
	ResetAdapter() error
}

// Conditions is what a system reads right at the press of a button, so a button left on the card
// after the trouble is gone does nothing (ND-244).
type Conditions struct {
	AdapterUp       bool
	HasAddress      bool // a usable address (not none, not 169.254)
	RouterAnswers   bool
	InternetAnswers bool
}

// ConditionSystem is optional: a system that has it is asked at apply time. reset-adapter is refused
// when the router or the internet answers; renew-address is refused when a usable address is there;
// when the conditions cannot be read, both are refused with a plain note.
type ConditionSystem interface {
	Conditions() (Conditions, error)
}
