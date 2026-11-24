//go:build windows

package fix

import (
	"errors"
	"os/exec"
	"strings"
	"syscall"
)

type realSystem struct{}

// Real is the computer this program runs on.
func Real() System { return realSystem{} }

const inetKey = `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`

func run(name string, args ...string) (string, error) {
	c := exec.Command(name, args...)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	b, err := c.CombinedOutput()
	return string(b), err
}

func (realSystem) FlushDNS() error {
	// ipconfig /flushdns needs administrator rights; this cmdlet does not.
	_, err := run("powershell", "-NoProfile", "-Command", "Clear-DnsClientCache -ErrorAction Stop")
	return err
}

// regValue reads one value of the current account's proxy settings; a value that is not set is no
// error, any failed read is.
func regValue(name string) (string, error) {
	out, err := run("reg", "query", inetKey, "/v", name)
	v, _, perr := parseRegValue(out, err, name)
	return v, perr
}

// Proxy reads the current account's proxy. A failed read is an error, never "off with no server".
func (realSystem) Proxy() (ProxyState, error) {
	en, err := regValue("ProxyEnable")
	if err != nil {
		return ProxyState{}, err
	}
	srv, err := regValue("ProxyServer")
	if err != nil {
		return ProxyState{}, err
	}
	return ProxyState{Enabled: en == "0x1", Server: srv}, nil
}

func (realSystem) SetProxy(s ProxyState) error {
	v := "0"
	if s.Enabled {
		v = "1"
	}
	if _, err := run("reg", "add", inetKey, "/v", "ProxyEnable", "/t", "REG_DWORD", "/d", v, "/f"); err != nil {
		return err
	}
	if s.Server != "" {
		_, err := run("reg", "add", inetKey, "/v", "ProxyServer", "/t", "REG_SZ", "/d", s.Server, "/f")
		return err
	}
	return nil
}

const activeAdapter = `$i=(Get-NetRoute -DestinationPrefix '0.0.0.0/0' | Sort-Object RouteMetric | Select-Object -First 1).InterfaceAlias;`

func (realSystem) PowerSaving() (string, error) {
	out, err := run("powershell", "-NoProfile", "-Command", activeAdapter+`(Get-NetAdapterPowerManagement -Name $i).AllowComputerToTurnOffDevice`)
	if err != nil {
		return "", err
	}
	switch strings.TrimSpace(out) {
	case "Enabled":
		return "on", nil
	case "Disabled":
		return "off", nil
	}
	return "", errors.New("this adapter does not report a power saving setting")
}

// SetPowerSaving asks Windows for administrator rights at this moment only,
// through the normal Windows prompt, then makes the one change. The Manager reads
// the setting back afterwards (ND-243).
func (realSystem) SetPowerSaving(v string) error {
	state := "Disabled"
	if v == "on" {
		state = "Enabled"
	}
	return elevated(activeAdapter + `Set-NetAdapterPowerManagement -Name $i -AllowComputerToTurnOffDevice ` + state)
}

// PowerSavingDropsLink stays true until a measured test shows that turning it
// off never drops the link in use (Stage 2 plan, fix 3). Until then the fix
// only says what to change by hand.
func (realSystem) PowerSavingDropsLink() bool { return true }

// Elevate does nothing by itself: Windows asks at the moment of the change.
func (realSystem) Elevate() error { return nil }
