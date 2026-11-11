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
	os.Exit(app.Run(ctx, os.Args[1:], collect.NewSource(), dir, os.Stdout))
}
