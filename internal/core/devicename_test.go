package core

import (
	"strings"
	"testing"
)

// ND-264: only a real hardware address, six pairs of hex digits, is accepted.
func TestValidMAC(t *testing.T) {
	good := []string{"02:00:00:00:00:20", "02-00-00-00-00-2a", "AA:BB:CC:DD:EE:FF"}
	bad := []string{"", "02:00:00:00:00", "02:00:00:00:00:2g", "02:00:00:00:00:20:11", "0200000000 20", "02:00:00:00:00:20\n#", "<b>x</b>"}
	for _, m := range good {
		if !ValidMAC(m) {
			t.Errorf("%q should be accepted", m)
		}
	}
	for _, m := range bad {
		if ValidMAC(m) {
			t.Errorf("%q should be refused", m)
		}
	}
}

// ND-264: a name is plain text of at most 40 characters with no control characters; markup is
// kept as the plain characters typed, never turned into anything else.
func TestCleanName(t *testing.T) {
	if got := CleanName(strings.Repeat("a", 60)); len([]rune(got)) != 40 {
		t.Errorf("a long name must be cut to 40 characters, got %d", len([]rune(got)))
	}
	if got := CleanName(strings.Repeat("é", 50)); len([]rune(got)) != 40 {
		t.Errorf("the limit counts characters, not bytes: got %d", len([]rune(got)))
	}
	if got := CleanName("my\nphone\t\x00\x1b[31m"); got != "myphone[31m" {
		t.Errorf("control characters must be removed: %q", got)
	}
	if got := CleanName("  <b>tv</b>  "); got != "<b>tv</b>" {
		t.Errorf("markup stays plain text, trimmed: %q", got)
	}
}
