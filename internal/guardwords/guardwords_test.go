package guardwords

import (
	"os"
	"path/filepath"
	"testing"
)

func writeList(t *testing.T, body string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "conf"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "conf", FileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// A folder with no list is not an error: the public copy has none, and the generic checks still run.
func TestMissingListIsEmptyNotAnError(t *testing.T) {
	l, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if l.Found || len(l.Hard)+len(l.Phrase)+len(l.Doc)+len(l.Skip) != 0 {
		t.Fatalf("expected an empty list, got %+v", l)
	}
}

func TestLoadReadsEachKindAndIgnoresCommentsAndBlankLines(t *testing.T) {
	root := writeList(t, "# note\n\nhard (?i)secret[ _-]?word\nphrase (?i)\\bhim\\b\ndoc Some Phrase\nskip docs/archive/\n")
	l, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if !l.Found || len(l.Hard) != 1 || len(l.Phrase) != 1 || len(l.Doc) != 1 || len(l.Skip) != 1 {
		t.Fatalf("unexpected list %+v", l)
	}
	if !l.Hard[0].MatchString("A Secret_Word here") {
		t.Error("hard pattern should match ignoring case")
	}
	if !l.Doc[0].MatchString("the some phrase is") || l.Doc[0].MatchString("awesome phrases") {
		t.Error("doc entries match whole words, ignoring case")
	}
	if !l.Skipped("docs/archive/x.md") || l.Skipped("docs/GUIDE.md") {
		t.Error("skip entries are path prefixes")
	}
}

func TestLoadRejectsAnUnknownKindOrABadPattern(t *testing.T) {
	if _, err := Load(writeList(t, "oops something\n")); err == nil {
		t.Error("an unknown kind should be an error")
	}
	if _, err := Load(writeList(t, "hard (unclosed\n")); err == nil {
		t.Error("a bad pattern should be an error")
	}
}
