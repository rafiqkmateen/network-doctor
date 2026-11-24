//go:build !windows

package store

// isSharingViolation is never true off Windows: files there can be replaced while open.
func isSharingViolation(error) bool { return false }
