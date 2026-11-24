//go:build windows

package fix

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/rafiqkmateen/network-doctor/internal/collect"
)

// The Windows computer has the round-two abilities (ND-235 to ND-237).
var (
	_ ExtraSystem     = realSystem{}
	_ ConditionSystem = realSystem{}
)

// Exit codes of the elevated helper.
const (
	exitRefused = 1 // the change itself failed
	exitNoAdmin = 5 // the person said no at the Windows prompt, or it could not start
)

// encodePS turns a script into the form PowerShell's -EncodedCommand takes (UTF-16LE, base64),
// so no quoting can break it.
func encodePS(script string) string {
	u := utf16.Encode([]rune(script))
	b := make([]byte, 2*len(u))
	for i, c := range u {
		binary.LittleEndian.PutUint16(b[2*i:], c)
	}
	return base64.StdEncoding.EncodeToString(b)
}

// elevated runs one script with administrator rights through the normal Windows prompt, waits
// for it, and checks its exit code (ND-243). The old way waited but never looked at the result.
func elevated(script string) error {
	inner := `$ErrorActionPreference='Stop'; try { ` + script + ` ; exit 0 } catch { exit 1 }`
	outer := `try { $p = Start-Process powershell -Verb RunAs -PassThru -Wait -WindowStyle Hidden -ArgumentList '-NoProfile','-EncodedCommand','` +
		encodePS(inner) + `' -ErrorAction Stop; exit $p.ExitCode } catch { exit 5 }`
	_, err := run("powershell", "-NoProfile", "-Command", outer)
	if err == nil {
		return nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		switch ee.ExitCode() {
		case exitNoAdmin:
			return errors.New("administrator rights were not given")
		case exitRefused:
			return errors.New("Windows refused the change")
		}
		return fmt.Errorf("Windows refused the change (code %d)", ee.ExitCode())
	}
	return err
}

// dnsScript lists the IPv4 name servers of each adapter that has a way to the internet. An
// adapter with no servers typed in its settings takes them from the router (Automatic).
const dnsScript = `$out=@(); foreach($x in (Get-NetRoute -DestinationPrefix '0.0.0.0/0' -ErrorAction SilentlyContinue | Sort-Object RouteMetric | Select-Object -ExpandProperty ifIndex -Unique)) {
$a=Get-NetAdapter -InterfaceIndex $x -ErrorAction SilentlyContinue; if(-not $a){continue}
$ns=(Get-ItemProperty -Path ('HKLM:\SYSTEM\CurrentControlSet\Services\Tcpip\Parameters\Interfaces\'+$a.InterfaceGuid) -Name NameServer -ErrorAction SilentlyContinue).NameServer
$s=@((Get-DnsClientServerAddress -InterfaceIndex $x -AddressFamily IPv4 -ErrorAction SilentlyContinue).ServerAddresses)
$rm=(Get-NetRoute -DestinationPrefix '0.0.0.0/0' -InterfaceIndex $x -ErrorAction SilentlyContinue | Sort-Object RouteMetric | Select-Object -First 1).RouteMetric
$im=(Get-NetIPInterface -InterfaceIndex $x -AddressFamily IPv4 -ErrorAction SilentlyContinue).InterfaceMetric
$out+=[pscustomobject]@{adapter=$a.Name; if_index=[int]$x; servers=$s; automatic=[string]::IsNullOrWhiteSpace($ns); route_metric=[int]$rm; interface_metric=[int]$im; status=[string]$a.Status}
}
ConvertTo-Json -InputObject @($out) -Compress`

func (realSystem) DNS() ([]DNSState, error) {
	out, err := run("powershell", "-NoProfile", "-Command", dnsScript)
	if err != nil {
		return nil, fmt.Errorf("could not read the name servers: %v", err)
	}
	var raw []struct {
		DNSState
		RouteMetric     int    `json:"route_metric"`
		InterfaceMetric int    `json:"interface_metric"`
		Status          string `json:"status"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &raw); err != nil {
		return nil, fmt.Errorf("could not read the name servers: %v", err)
	}
	// The adapter in use is chosen the same way the check chooses it: up, and the lowest route
	// plus interface metric (ND-244).
	rows := make([]collect.RouteRow, len(raw))
	got := make([]DNSState, len(raw))
	for i, r := range raw {
		rows[i] = collect.RouteRow{IfIndex: r.IfIndex, RouteMetric: r.RouteMetric, InterfaceMetric: r.InterfaceMetric, Status: r.Status}
		got[i] = r.DNSState
		got[i].InUse = false
	}
	if best, ok := collect.PickDefaultRoute(rows); ok {
		for i := range got {
			got[i].InUse = got[i].IfIndex == best.IfIndex
		}
	}
	return got, nil
}

// SetDNS sets the servers of the given adapters in one elevated step, then reads them back.
func (r realSystem) SetDNS(in []DNSState) error {
	var b strings.Builder
	for _, s := range in {
		if s.IfIndex <= 0 {
			return errors.New("an adapter number is missing")
		}
		idx := strconv.Itoa(s.IfIndex)
		if s.Automatic {
			b.WriteString(`Set-DnsClientServerAddress -InterfaceIndex ` + idx + ` -ResetServerAddresses; `)
			continue
		}
		if len(s.Servers) == 0 {
			return errors.New("no servers were given for " + s.Adapter)
		}
		quoted := make([]string, len(s.Servers))
		for i, ip := range s.Servers {
			if net.ParseIP(ip) == nil {
				return fmt.Errorf("%q is not a server address", ip)
			}
			quoted[i] = "'" + ip + "'"
		}
		b.WriteString(`Set-DnsClientServerAddress -InterfaceIndex ` + idx + ` -ServerAddresses (` + strings.Join(quoted, ",") + `); `)
	}
	if b.Len() == 0 {
		return errors.New("no adapter was given")
	}
	if err := elevated(b.String()); err != nil {
		return err
	}
	back, err := r.DNS()
	if err != nil {
		return err
	}
	if !allMatch(in, back) {
		return errors.New("Windows said yes, but the servers did not change when read back")
	}
	return nil
}

// pickAdapter finds the adapter in use: the one with the best way to the internet, or, when
// there is none (no address yet), the first physical adapter that is up.
func pickAdapter() (int, error) {
	out, err := run("powershell", "-NoProfile", "-Command",
		`$r=Get-NetRoute -DestinationPrefix '0.0.0.0/0' -ErrorAction SilentlyContinue | Sort-Object RouteMetric | Select-Object -First 1; if($r){$r.ifIndex} else {(Get-NetAdapter -Physical | Where-Object Status -eq 'Up' | Sort-Object ifIndex | Select-Object -First 1).ifIndex}`)
	if err != nil {
		return 0, fmt.Errorf("could not find the adapter in use: %v", err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil || n <= 0 {
		return 0, errors.New("no network adapter in use was found")
	}
	return n, nil
}

// goodAddress is true when the adapter has an IPv4 address that does not start with 169.254.
func goodAddress(idx int) bool {
	out, err := run("powershell", "-NoProfile", "-Command",
		`@(Get-NetIPAddress -InterfaceIndex `+strconv.Itoa(idx)+` -AddressFamily IPv4 -ErrorAction SilentlyContinue | Where-Object { $_.IPAddress -notlike '169.254.*' }).Count`)
	if err != nil {
		return false
	}
	n, _ := strconv.Atoi(strings.TrimSpace(out))
	return n > 0
}

// adapterUp is true when Windows reports the adapter as up.
func adapterUp(idx int) bool {
	out, err := run("powershell", "-NoProfile", "-Command", `(Get-NetAdapter -InterfaceIndex `+strconv.Itoa(idx)+` -ErrorAction SilentlyContinue).Status`)
	return err == nil && strings.TrimSpace(out) == "Up"
}

// Renew asks the router for a new address, then checks a usable one is there.
func (realSystem) Renew() error {
	idx, err := pickAdapter()
	if err != nil {
		return err
	}
	script := `$a=Get-NetAdapter -InterfaceIndex ` + strconv.Itoa(idx) + `; $o=ipconfig /renew $a.Name; if($LASTEXITCODE -ne 0){ throw 'renew failed' }`
	if err := elevated(script); err != nil {
		return err
	}
	for i := 0; i < 10; i++ {
		if goodAddress(idx) {
			return nil
		}
		time.Sleep(time.Second)
	}
	return errors.New("the router did not give a usable address")
}

// ResetAdapter switches the adapter in use off and on in one elevated step. It is always turned
// back on, even when turning it off fails. Then it checks the adapter came back up.
func (realSystem) ResetAdapter() error {
	idx, err := pickAdapter()
	if err != nil {
		return err
	}
	n := strconv.Itoa(idx)
	script := `try { Get-NetAdapter -InterfaceIndex ` + n + ` | Disable-NetAdapter -Confirm:$false; Start-Sleep -Seconds 2 } finally { Get-NetAdapter -InterfaceIndex ` + n + ` | Enable-NetAdapter -Confirm:$false }`
	if err := elevated(script); err != nil {
		return err
	}
	for i := 0; i < 30; i++ {
		if adapterUp(idx) {
			return nil
		}
		time.Sleep(time.Second)
	}
	return errors.New("the adapter did not come back on")
}

// Conditions reads the situation now (ND-244): the adapter in use, its address, and a short
// ping of the router and of the internet. It only reads, using the check's own read code.
func (realSystem) Conditions() (Conditions, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return readConditions(ctx, collect.Windows{})
}
