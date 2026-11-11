//go:build windows

// Package notify shows a message on the screen. It only displays text.
package notify

import (
	"os/exec"
	"strings"
	"syscall"
)

func esc(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "'", "''", "\"", "&quot;")
	return r.Replace(s)
}

// Screen shows a Windows toast.
func Screen(title, msg string) error {
	script := `[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] | Out-Null
$x = [Windows.Data.Xml.Dom.XmlDocument]::new()
$x.LoadXml('<toast><visual><binding template="ToastText02"><text id="1">` + esc(title) + `</text><text id="2">` + esc(msg) + `</text></binding></visual></toast>')
$t = [Windows.UI.Notifications.ToastNotification]::new($x)
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('Network Doctor').Show($t)`
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Run()
}
