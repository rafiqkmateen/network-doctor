package collect

import (
	"regexp"
	"testing"
)

// ND-01: every command the collector runs only reads.
func TestScriptsOnlyRead(t *testing.T) {
	bad := regexp.MustCompile(`(?i)\b(Set|New|Remove|Restart|Enable|Disable|Stop|Start|Add|Clear|Install|Uninstall|Register|Unregister|Invoke|Update|Rename|Move|Copy|Out|Write|Reset|Repair|Disconnect|Connect|Publish)-[A-Za-z]+|netsh\s+\S+\s+(set|add|delete)|reg(\.exe)?\s+(add|delete)|ipconfig\s+/(release|renew|flushdns)|route\s+(add|delete|change)`)
	for name, s := range map[string]string{
		"adapter": scriptAdapter, "events": scriptEvents, "throughput": scriptThroughput,
		"neighbors": scriptNeighbors, "settings": scriptSettings,
	} {
		if m := bad.FindString(s); m != "" {
			t.Errorf("script %s uses %q", name, m)
		}
	}
}

func TestParsePingWindows(t *testing.T) {
	out := "Packets: Sent = 10, Received = 9, Lost = 1 (10% loss),\r\nApproximate round trip times in milli-seconds:\r\n    Minimum = 1ms, Maximum = 5ms, Average = 3ms"
	r := ParsePing(out)
	if r.LossPct != 10 || r.AvgMs != 3 {
		t.Fatalf("%+v", r)
	}
}

func TestParsePingUnix(t *testing.T) {
	out := "10 packets transmitted, 10 received, 0% packet loss, time 9012ms\nrtt min/avg/max/mdev = 1.1/2.5/4.0/0.3 ms"
	r := ParsePing(out)
	if r.LossPct != 0 || r.AvgMs != 2.5 {
		t.Fatalf("%+v", r)
	}
}

// ND-41: read the system's own Wi-Fi report.
func TestParseWlan(t *testing.T) {
	ifc := "    Name                   : Wi-Fi\r\n    State                  : connected\r\n    Band                   : 2.4 GHz\r\n    Channel                : 6\r\n    Signal                 : 35%\r\n"
	info := ParseWlanInterfaces(ifc)
	if info.SignalPct != 35 || info.Channel != 6 || info.Band != "2.4 GHz" {
		t.Fatalf("%+v", info)
	}
	nets := "SSID 1 : Home\r\n    BSSID 1  : aa\r\n         Channel   : 6\r\nSSID 2 : Next\r\n    BSSID 1  : bb\r\n         Channel   : 6\r\nSSID 3 : Far\r\n    BSSID 1  : cc\r\n         Channel   : 11\r\n"
	got := ParseWlanChannels(nets, 6)
	if len(got) != 2 || got[0] != 6 || got[1] != 11 {
		t.Fatalf("own network must be left out: %v", got)
	}
}

func TestParseThroughput(t *testing.T) {
	one := `{"Rx":21500000,"Tx":400000,"Procs":{"Name":"chrome","Conns":9}}`
	t1, err := ParseThroughput([]byte(one), 1000)
	if err != nil || len(t1.Procs) != 1 || t1.RxMBps != 21.5 || t1.BusyPct != 17 {
		t.Fatalf("%+v %v", t1, err)
	}
	many := `{"Rx":0,"Tx":0,"Procs":[{"Name":"a","Conns":2},{"Name":"a","Conns":3},{"Name":"b","Conns":1}]}`
	t2, _ := ParseThroughput([]byte(many), 1000)
	if len(t2.Procs) != 2 {
		t.Fatalf("same-name programs should be added together: %+v", t2.Procs)
	}
	empty := `{"Rx":1,"Tx":1,"Procs":[]}`
	if _, err := ParseThroughput([]byte(empty), 1000); err != nil {
		t.Fatal(err)
	}
}

func TestPowerState(t *testing.T) {
	if k, _ := PowerState("", ""); k {
		t.Fatal("nothing read means unknown")
	}
	if k, s := PowerState("Enabled", ""); !k || !s {
		t.Fatal("enabled")
	}
	if k, s := PowerState("", "Enabled"); !k || !s {
		t.Fatal("energy option on counts as saving")
	}
	if k, s := PowerState("Disabled", "Disabled"); !k || s {
		t.Fatal("disabled")
	}
}

func TestMaker(t *testing.T) {
	if Maker("B8-27-EB-00-00-01") != "Raspberry Pi" {
		t.Fatal("raspberry pi")
	}
	if Maker("DA:A1:19:00:00:01") == "unknown" {
		t.Fatal("locally administered address should be called private")
	}
	if Maker("00:11:22:33:44:55") != "unknown" {
		t.Fatal("unlisted should be unknown")
	}
}

func TestLooseList(t *testing.T) {
	var a []struct{ IP string }
	if err := looseList([]byte(`{"IP":"1.1.1.1"}`), &a); err != nil || len(a) != 1 {
		t.Fatal("single object")
	}
	if err := looseList([]byte(`[{"IP":"1"},{"IP":"2"}]`), &a); err != nil || len(a) != 2 {
		t.Fatal("list")
	}
}
