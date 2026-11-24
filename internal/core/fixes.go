package core

// The three small fixes the tool can make. The fix package uses the same names.
const (
	FixNameCache   = "name-cache"
	FixProxy       = "proxy"
	FixPowerSaving = "power-saving"
	// Round two (ND-235 to ND-237).
	FixPublicDNS    = "public-dns"
	FixRenewAddress = "renew-address"
	FixResetAdapter = "reset-adapter"
)

// SuggestFixes names the fixes that fit what a check found: clearing the name list when web
// names cannot be looked up, turning off a proxy that is on, and turning off adapter power
// saving when it is on and a drop was seen, Cloudflare's name servers when names fail while the
// internet answers, a new address when the adapter has none, and turning the adapter off and on
// when neither the internet nor the router answers. It only reads; it never changes anything.
func SuggestFixes(r Reading, s Settings, d DropReport) []string {
	out := []string{}
	if r.DNSFails {
		out = append(out, FixNameCache)
	}
	if s.ProxyOn {
		out = append(out, FixProxy)
	}
	if s.PowerKnown && s.PowerSaving && d.Count > 0 {
		out = append(out, FixPowerSaving)
	}
	// Round two: offered only when the connection already does not work, so nothing in use is lost.
	// Behind a sign-in page names fail for that reason, so public-dns is not offered (ND-244).
	if r.DNSFails && r.Online && r.AdapterUp && !r.SignIn {
		out = append(out, FixPublicDNS)
	}
	if r.AdapterUp && r.NoAddress {
		out = append(out, FixRenewAddress)
	}
	if !r.Online && r.RouterLoss >= 100 {
		out = append(out, FixResetAdapter)
	}
	return out
}
