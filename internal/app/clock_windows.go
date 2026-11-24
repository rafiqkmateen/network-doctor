//go:build windows

package app

import (
	"syscall"
	"unsafe"
)

// readClock24h reads the person's own Windows time setting (HKCU\Control Panel\International).
func readClock24h() *bool {
	return parseWindowsClock(internationalValue("sShortTime"), internationalValue("sTimeFormat"), internationalValue("iTime"))
}

// internationalValue reads one text value of HKCU\Control Panel\International, or "".
func internationalValue(name string) string {
	key, _ := syscall.UTF16PtrFromString(`Control Panel\International`)
	var k syscall.Handle
	if syscall.RegOpenKeyEx(syscall.HKEY_CURRENT_USER, key, 0, syscall.KEY_READ, &k) != nil {
		return ""
	}
	defer syscall.RegCloseKey(k)
	n, _ := syscall.UTF16PtrFromString(name)
	var typ, size uint32
	if syscall.RegQueryValueEx(k, n, nil, &typ, nil, &size) != nil || typ != syscall.REG_SZ || size < 2 || size > 512 {
		return ""
	}
	buf := make([]uint16, size/2+1)
	if syscall.RegQueryValueEx(k, n, nil, &typ, (*byte)(unsafe.Pointer(&buf[0])), &size) != nil {
		return ""
	}
	return syscall.UTF16ToString(buf)
}
