package install

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// OoklaFile is one of Ookla's files: where it is and the SHA-256 it must have.
type OoklaFile struct {
	URL    string
	SHA256 string
}

// ooklaHost is the only place the install fetches from (ND-302, ND-315).
const ooklaHost = "install.speedtest.net"

// The fixed SHA-256 of each of Ookla's files.
//
// Filled 2026-11-20 from a real download of each file on GitHub's clean Linux machine (the fingerprint job), over https from Ookla, and
// the file names confirmed there. While a value is empty the install
// refuses to fetch that file and says the speed tool was not added.
const (
	ooklaSHA256MacUniversal = "c9f8192149ebc88f8699998cecab1ce144144045907ece6f53cf50877f4de66f" // ookla-speedtest-1.2.0-macosx-universal.tgz
	ooklaSHA256LinuxX8664   = "5690596c54ff9bed63fa3732f818a05dbc2db19ad36ed68f21ca5f64d5cfeeb7" // ookla-speedtest-1.2.0-linux-x86_64.tgz
	ooklaSHA256LinuxAarch64 = "3953d231da3783e2bf8904b6dd72767c5c6e533e163d3742fd0437affa431bd3" // ookla-speedtest-1.2.0-linux-aarch64.tgz
	ooklaSHA256LinuxArmhf   = "e45fcdebbd8a185553535533dd032d6b10bc8c64eee4139b1147b9c09835d08d" // ookla-speedtest-1.2.0-linux-armhf.tgz
)

const ooklaBase = "https://" + ooklaHost + "/app/cli/ookla-speedtest-1.2.0-"

// defaultOoklaFiles is the table, by system and chip. One Mac file serves both chips.
var defaultOoklaFiles = map[string]OoklaFile{
	"darwin":      {URL: ooklaBase + "macosx-universal.tgz", SHA256: ooklaSHA256MacUniversal},
	"linux/amd64": {URL: ooklaBase + "linux-x86_64.tgz", SHA256: ooklaSHA256LinuxX8664},
	"linux/arm64": {URL: ooklaBase + "linux-aarch64.tgz", SHA256: ooklaSHA256LinuxAarch64},
	"linux/arm":   {URL: ooklaBase + "linux-armhf.tgz", SHA256: ooklaSHA256LinuxArmhf},
}

// ooklaFileFor picks the file for this system and chip.
func ooklaFileFor(table map[string]OoklaFile, goos, goarch string) (OoklaFile, bool) {
	if goos == "darwin" {
		f, ok := table["darwin"]
		return f, ok
	}
	f, ok := table[goos+"/"+goarch]
	return f, ok
}

// ooklaRedirect lets a redirect stay only on Ookla's own https address.
func ooklaRedirect(req *http.Request, via []*http.Request) error {
	if !ooklaPlace(req.URL) {
		return errors.New("redirect away from Ookla refused")
	}
	if len(via) >= 5 {
		return errors.New("too many redirects")
	}
	return nil
}

func ooklaPlace(u *url.URL) bool {
	return u != nil && u.Scheme == "https" && strings.EqualFold(u.Host, ooklaHost)
}

// The biggest file and the biggest unpacked file taken (Ookla's are about 1 to 2 megabytes).
const (
	ooklaMaxDownload = 50 << 20
	ooklaMaxFile     = 50 << 20
)

// addOokla fetches Ookla's file, checks it against its fixed SHA-256, unpacks it into the ookla
// folder beside the program only when it matches, and gives the license switches to the tool
// once (ND-302). A file that does not match is deleted.
func addOokla(e Env, programDir string) error {
	f, ok := ooklaFileFor(e.OoklaFiles, e.GOOS, e.GOARCH)
	if !ok {
		return fmt.Errorf("Ookla has no file for %s %s", e.GOOS, e.GOARCH)
	}
	if len(f.SHA256) != 64 {
		return errors.New("no fixed SHA-256 for this file yet, so it is not fetched")
	}
	u, err := url.Parse(f.URL)
	if err != nil || !ooklaPlace(u) {
		return fmt.Errorf("refused address %q: only https on %s", f.URL, ooklaHost)
	}
	tmp, err := os.CreateTemp(programDir, ".ookla-*.tgz")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	res, err := e.HTTP.Get(u.String())
	if err != nil {
		tmp.Close()
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		tmp.Close()
		return fmt.Errorf("Ookla answered %s", res.Status)
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(res.Body, ooklaMaxDownload+1))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if n > ooklaMaxDownload {
		return errors.New("the file is too big")
	}
	if got := hex.EncodeToString(h.Sum(nil)); !strings.EqualFold(got, f.SHA256) {
		return fmt.Errorf("the file does not match its fixed SHA-256 (got %s)", got)
	}
	dest := filepath.Join(programDir, "ookla")
	if err := unpackTgz(e.Home, tmpPath, dest); err != nil {
		removeIn(e.Home, dest)
		return err
	}
	tool := filepath.Join(dest, "speedtest")
	if _, err := os.Stat(tool); err != nil {
		removeIn(e.Home, dest)
		return errors.New("the file holds no speedtest tool")
	}
	os.Chmod(tool, 0o755)
	// Only now, after the person's own tick, is the tool told they accepted its license.
	if _, err := e.Run(tool, "--accept-license", "--accept-gdpr", "-f", "json", "--progress=no", "--servers"); err != nil {
		return fmt.Errorf("the tool did not run: %w", err)
	}
	return nil
}

// unpackTgz unpacks the plain files at the top of a gzip tar file into dest. Folders, links and
// any name with a folder in it are skipped, so nothing lands outside dest.
func unpackTgz(home, src, dest string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	if err := mkdirIn(home, dest); err != nil {
		return err
	}
	for {
		hd, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := path.Clean(strings.TrimPrefix(hd.Name, "./"))
		if hd.Typeflag != tar.TypeReg || strings.ContainsAny(name, `/\`) || name == "." || name == ".." ||
			strings.HasPrefix(name, ".") || hd.Size > ooklaMaxFile {
			continue
		}
		b, err := io.ReadAll(io.LimitReader(tr, ooklaMaxFile))
		if err != nil {
			return err
		}
		if err := writeBytesIn(home, filepath.Join(dest, name), b, 0o644); err != nil {
			return err
		}
	}
}
