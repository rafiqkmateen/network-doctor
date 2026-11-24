//go:build windows

package app

import (
	"io"
	"os/exec"
	"syscall"
	"unsafe"
)

// hideWindow keeps a helper program from flashing a console window.
func hideWindow(c *exec.Cmd) { c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} }

// showNoBrowser shows the address in a message box, since the icon's program has no window to
// print in. The page stays open while the box is up; pressing OK ends the program, so it never
// waits unseen (ND-263).
func showNoBrowser(url string, out io.Writer) bool {
	printNoBrowser(url, out)
	text, _ := syscall.UTF16PtrFromString("Network Doctor could not open your web browser.\r\n\r\n" +
		"Open this address in your web browser:\r\n" + url + "\r\n\r\n" +
		"Press OK when you are done. Network Doctor then stops.")
	title, _ := syscall.UTF16PtrFromString("Network Doctor")
	const mbOK, mbIconInfo, mbSetForeground = 0x0, 0x40, 0x10000
	syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW").Call(0,
		uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), mbOK|mbIconInfo|mbSetForeground)
	return true
}
