package store

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// saveMu keeps saves in this program one at a time.
var saveMu sync.Mutex

// lockWait is how long a save waits for another program's save to finish; a lock older than
// lockStale was left by a program that stopped in the middle, and is taken over. A temporary file
// older than tmpStale was left the same way, and is removed.
const (
	lockWait  = 5 * time.Second
	lockStale = 15 * time.Second
	tmpStale  = time.Minute
)

// saveFile changes one of the tool's small files safely. It holds a lock file beside it so two
// programs never save at once, reads the current content, lets edit make the new content, writes
// that to a new temporary file in the same folder and renames it into place, so a reader always
// sees the whole old file or the whole new one (ND-264, ND-265).
func saveFile(path string, edit func(old []byte) ([]byte, error)) error {
	saveMu.Lock()
	defer saveMu.Unlock()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	unlock, err := lockFile(path + ".lock")
	if err != nil {
		return err
	}
	defer unlock()
	cleanOldTemps(path)
	old, err := readSoon(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	b, err := edit(old)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	_, werr := tmp.Write(b)
	if cerr := tmp.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Chmod(name, 0o644)
	}
	if werr == nil {
		werr = renameSoon(name, path)
	}
	if werr != nil {
		os.Remove(name)
	}
	return werr
}

// cleanOldTemps removes this file's temporary files that a stopped save left behind. A fresh one
// may belong to a save in progress, so only those over a minute old go.
func cleanOldTemps(path string) {
	old, _ := filepath.Glob(path + ".tmp*")
	for _, f := range old {
		if st, err := os.Stat(f); err == nil && time.Since(st.ModTime()) > tmpStale {
			os.Remove(f)
		}
	}
}

func token() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// lockFile makes the lock file, holding a random token, and waits while another save holds it. The
// returned function removes the lock only while it still holds this save's token. A lock older than
// lockStale was left by a program that stopped; it is taken over (see takeOver). Any error other
// than "a lock is there" is returned at once, so a folder that cannot be written says so.
func lockFile(lock string) (func(), error) {
	mine := token()
	end := time.Now().Add(lockWait)
	for {
		f, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_, werr := f.WriteString(mine)
			f.Close()
			if werr != nil {
				os.Remove(lock)
				return nil, werr
			}
			return func() {
				if b, err := os.ReadFile(lock); err == nil && string(b) == mine {
					os.Remove(lock)
				}
			}, nil
		}
		st, serr := os.Stat(lock)
		if !errors.Is(err, os.ErrExist) && errors.Is(serr, os.ErrNotExist) {
			// No lock is there, yet it could not be made: the folder cannot be written, or worse.
			// (A lock being removed on Windows refuses both for a moment; that is waited out.)
			return nil, err
		}
		stale := serr == nil && time.Since(st.ModTime()) > lockStale
		if stale && takeOver(lock) {
			continue // the old lock is gone: try to make ours at once
		}
		// Whatever happened, the 5-second limit holds, so a save never waits on without end.
		if time.Now().After(end) {
			if stale {
				return nil, errors.New("an old lock file from a program that stopped could not be removed: " +
					lock + "; remove it and try again")
			}
			return nil, errors.New("another program is saving the same file; try again")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// takeOver moves a stale lock aside and removes it. Only one waiter at a time may do this (it holds
// lock+".takeover"), and it first checks that the lock is still the same stale one, so a lock
// another waiter has just made is never removed. It reports whether the stale lock is gone; false
// means try again later (another waiter is taking over, or the lock could not be moved).
func takeOver(lock string) bool {
	guard := lock + ".takeover"
	g, err := os.OpenFile(guard, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if st, serr := os.Stat(guard); serr == nil && time.Since(st.ModTime()) > lockStale {
			os.Remove(guard) // left by a program that stopped during a takeover
		}
		return false
	}
	g.Close()
	defer os.Remove(guard)
	st, err := os.Stat(lock)
	if err != nil {
		return true // gone already
	}
	if time.Since(st.ModTime()) <= lockStale {
		return false // a fresh lock: leave it
	}
	aside := lock + ".stale-" + token()
	if os.Rename(lock, aside) != nil {
		return false // cannot be moved (no permission, or held open): the time limit decides
	}
	os.Remove(aside)
	return true
}

// renameSoon puts the new file in place. On Windows a rename fails while another program has the
// file open for a moment, so it is tried again briefly.
func renameSoon(from, to string) error {
	var err error
	for i := 0; i < 50; i++ {
		if err = os.Rename(from, to); err == nil || !retryable(err) {
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}
	return err
}

// readSoon reads a file, trying again briefly while a save is putting the new one in place.
func readSoon(path string) ([]byte, error) {
	var (
		b   []byte
		err error
	)
	for i := 0; i < 50; i++ {
		if b, err = os.ReadFile(path); err == nil || !retryable(err) {
			return b, err
		}
		time.Sleep(5 * time.Millisecond)
	}
	return b, err
}

// retryable is true for the brief "in use" errors a rename or read can meet on Windows.
func retryable(err error) bool {
	return errors.Is(err, os.ErrPermission) || isSharingViolation(err)
}
