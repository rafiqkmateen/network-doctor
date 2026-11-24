// Command netdoctor is Network Doctor.
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/rafiqkmateen/network-doctor/internal/app"
	"github.com/rafiqkmateen/network-doctor/internal/collect"
	"github.com/rafiqkmateen/network-doctor/internal/store"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	dir := store.DefaultDir()
	if d := os.Getenv("NETDOCTOR_DIR"); d != "" {
		dir = d // keep one copy's files apart, for example on a USB stick
	}
	os.Exit(app.RunWith(ctx, os.Args[1:], collect.NewSource(), dir, app.Terminal{In: os.Stdin, Keyboard: isKeyboard()}, os.Stdout))
}

// isKeyboard is true when a person is typing into this window and reading its
// output, not when input or output is a pipe or the window has no console.
func isKeyboard() bool {
	in, err1 := os.Stdin.Stat()
	out, err2 := os.Stdout.Stat()
	return err1 == nil && err2 == nil && in.Mode()&os.ModeCharDevice != 0 && out.Mode()&os.ModeCharDevice != 0
}
