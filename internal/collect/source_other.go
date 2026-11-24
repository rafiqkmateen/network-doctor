//go:build !windows && !linux && !darwin

package collect

// NewSource returns the collector for this system. Systems other than Windows,
// Linux and Mac have only the checks that work the same everywhere.
func NewSource() Source { return Generic{} }
