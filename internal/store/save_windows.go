//go:build windows

package store

import (
	"errors"
	"syscall"
)

// isSharingViolation is true when another program has the file open (Windows error 32).
func isSharingViolation(err error) bool {
	var en syscall.Errno
	return errors.As(err, &en) && en == 32
}
