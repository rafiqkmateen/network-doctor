//go:build !windows

package fix

import (
	"errors"
	"os/exec"
	"runtime"
)

type realSystem struct{}

// Real is the computer this program runs on. Mac and Linux get the name cache
// fix at first; the proxy and power saving fixes come later.
func Real() System { return realSystem{} }

var errLater = errors.New("this fix is not available on this system yet")

func (realSystem) FlushDNS() error {
	if runtime.GOOS == "darwin" {
		return exec.Command("dscacheutil", "-flushcache").Run()
	}
	return exec.Command("resolvectl", "flush-caches").Run()
}
func (realSystem) Proxy() (ProxyState, error)   { return ProxyState{}, errLater }
func (realSystem) SetProxy(ProxyState) error    { return errLater }
func (realSystem) PowerSaving() (string, error) { return "", errLater }
func (realSystem) SetPowerSaving(string) error  { return errLater }
func (realSystem) PowerSavingDropsLink() bool   { return true }
func (realSystem) Elevate() error               { return nil }
