package app

import (
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"time"
)

// How long the open command waits before it quits by itself (ND-263). Tests make these short.
var (
	openIdle   = 3 * time.Minute // no heartbeat from a page that had loaded (a hidden tab beats slowly)
	openNoPage = 5 * time.Minute // the page never loaded
)

// openBrowser opens the address in the default browser. Tests replace it so no browser starts.
var openBrowser = startBrowser

// noBrowser tells the person the address when no browser could be started. It returns true when
// the person has finished with it and the program should end (the Windows message box), false when
// the address was only printed. Tests replace it so no message box appears.
var noBrowser = showNoBrowser

// printNoBrowser prints the address; it is seen only when a console window exists.
func printNoBrowser(url string, out io.Writer) bool {
	fmt.Fprintf(out, "Could not open your web browser. Open this address in it: %s\n", url)
	return false
}

// browserCommand is the system's own way to open an address in the default browser. On Windows
// rundll32 hands it to the browser without a console window.
func browserCommand(goos, url string) (string, []string) {
	switch goos {
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		return "open", []string{url}
	}
	return "xdg-open", []string{url}
}

// startBrowser starts the browser and does not wait for it.
func startBrowser(url string) error {
	name, args := browserCommand(runtime.GOOS, url)
	c := exec.Command(name, args...)
	hideWindow(c)
	if err := c.Start(); err != nil {
		return err
	}
	go c.Wait() // reap it; the browser lives on its own
	return nil
}
