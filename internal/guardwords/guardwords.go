// Package guardwords reads the optional private word list that the guard tests use.
//
// The list lives in conf/guard_words.txt, which is not tracked, so the words themselves are never
// in the tree. A checkout without the file (a fresh clone) runs only the generic
// checks. Each line is "kind value": hard and phrase take a regular expression, doc takes a
// plain phrase that the guide and README must not hold, skip takes a path prefix to leave out.
package guardwords

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// FileName is the list's name inside the conf folder.
const FileName = "guard_words.txt"

// List is what the file held; Found is false when there was no file.
type List struct {
	Found  bool
	Hard   []*regexp.Regexp // no tracked file or path may match
	Phrase []*regexp.Regexp // no code line may match
	Doc    []*regexp.Regexp // the guide and README must not hold it (whole words, any case)
	Skip   []string         // tracked paths with these prefixes are not checked
}

// Load reads conf/guard_words.txt under root. A missing file gives an empty list.
func Load(root string) (List, error) {
	f, err := os.Open(filepath.Join(root, "conf", FileName))
	if errors.Is(err, os.ErrNotExist) {
		return List{}, nil
	}
	if err != nil {
		return List{}, err
	}
	defer f.Close()
	l := List{Found: true}
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		kind, value, ok := strings.Cut(line, " ")
		if !ok || strings.TrimSpace(value) == "" {
			return List{}, fmt.Errorf("%s line %d: expected \"kind value\"", FileName, n)
		}
		value = strings.TrimSpace(value)
		switch kind {
		case "skip":
			l.Skip = append(l.Skip, value)
			continue
		case "doc":
			value = `(?i)\b` + regexp.QuoteMeta(value) + `\b`
		case "hard", "phrase":
		default:
			return List{}, fmt.Errorf("%s line %d: unknown kind %q", FileName, n, kind)
		}
		re, err := regexp.Compile(value)
		if err != nil {
			return List{}, fmt.Errorf("%s line %d: %v", FileName, n, err)
		}
		switch kind {
		case "hard":
			l.Hard = append(l.Hard, re)
		case "phrase":
			l.Phrase = append(l.Phrase, re)
		case "doc":
			l.Doc = append(l.Doc, re)
		}
	}
	return l, sc.Err()
}

// Skipped reports whether a tracked path is left out of the private-word checks.
func (l List) Skipped(rel string) bool {
	for _, p := range l.Skip {
		if strings.HasPrefix(rel, p) {
			return true
		}
	}
	return false
}
