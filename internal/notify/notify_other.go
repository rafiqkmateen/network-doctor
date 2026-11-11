//go:build !windows

// Package notify shows a message on the screen. It only displays text.
package notify

import "errors"

// Screen is not built for this system yet.
func Screen(title, msg string) error {
	return errors.New("screen alerts are not built for this system yet")
}
