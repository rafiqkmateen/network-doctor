package core

// Check for ND-244, written before the build: public-dns is never
// offered behind a sign-in page.

import "testing"

func TestND244PublicDNSNotOfferedBehindASignInPage(t *testing.T) {
	r := Reading{Kind: WiFi, AdapterUp: true, Online: true, DNSFails: true, SignIn: true}
	for _, f := range SuggestFixes(r, Settings{}, DropReport{}) {
		if f == FixPublicDNS {
			t.Errorf("public-dns must not be offered behind a sign-in page: %v", SuggestFixes(r, Settings{}, DropReport{}))
		}
	}
	r.SignIn = false
	found := false
	for _, f := range SuggestFixes(r, Settings{}, DropReport{}) {
		found = found || f == FixPublicDNS
	}
	if !found {
		t.Error("without a sign-in page, names failing while the internet answers still offers public-dns")
	}
}
