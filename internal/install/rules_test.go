package install

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// onBlock is the text of a workflow's top-level "on:" block.
func onBlock(wf string) string {
	var b strings.Builder
	in := false
	for _, l := range strings.Split(wf, "\n") {
		if strings.HasPrefix(l, "on:") {
			in = true
			continue
		}
		if in && l != "" && !strings.HasPrefix(l, " ") && !strings.HasPrefix(l, "#") {
			break
		}
		if in {
			b.WriteString(l + "\n")
		}
	}
	return b.String()
}

// ND-313: both parity workflows start only by hand or by a push to one work branch; the Mac job has
// a time limit of 30 minutes or less; ci.yml has no Mac job.
func TestParityWorkflowsAreManual(t *testing.T) {
	for file, branch := range map[string]string{"parity-linux.yml": "parity-work", "parity-mac.yml": "parity-work-mac"} {
		wf := repoFile(t, ".github/workflows/"+file)
		on := onBlock(wf)
		if !regexp.MustCompile(`(?m)^  workflow_dispatch:`).MatchString(on) || !regexp.MustCompile(`(?m)^  push:`).MatchString(on) {
			t.Errorf("%s: needs workflow_dispatch and push:\n%s", file, on)
		}
		if strings.Contains(on, "pull_request") || strings.Contains(on, "schedule") || strings.Contains(on, "workflow_run") {
			t.Errorf("%s: started by something other than a person or a push:\n%s", file, on)
		}
		m := regexp.MustCompile(`(?m)^    branches:\s*\[([^\]]*)\]\s*$`).FindAllStringSubmatch(on, -1)
		if len(m) != 1 || strings.TrimSpace(m[0][1]) != branch {
			t.Errorf("%s: push must be limited to the one branch %s: %v", file, branch, m)
		}
		if strings.Contains(on, "tags") || strings.Contains(on, "branches-ignore") {
			t.Errorf("%s: push reaches more than the work branch", file)
		}
	}
	mac := repoFile(t, ".github/workflows/parity-mac.yml")
	jobs := len(regexp.MustCompile(`(?m)^\s+runs-on:`).FindAllString(mac, -1))
	limits := regexp.MustCompile(`(?m)^\s+timeout-minutes:\s*(\d+)\s*$`).FindAllStringSubmatch(mac, -1)
	if jobs == 0 || len(limits) < jobs {
		t.Errorf("parity-mac.yml: %d jobs, %d time limits", jobs, len(limits))
	}
	for _, l := range limits {
		if n, _ := strconv.Atoi(l[1]); n > 30 || n <= 0 {
			t.Errorf("parity-mac.yml: timeout-minutes %d", n)
		}
	}
	if regexp.MustCompile(`(?i)macos`).MatchString(repoFile(t, ".github/workflows/ci.yml")) {
		t.Error("ci.yml has a Mac job")
	}
}

// ND-315: install and uninstall send nothing off the computer except the fetch of Ookla's file
// from install.speedtest.net. The only other addresses are Ookla's license links, the local page
// on 127.0.0.1, and the Info.plist type name Apple's tools read (never fetched).
func TestInstallSendsNothingAway(t *testing.T) {
	allowedHost := map[string]bool{"install.speedtest.net": true, "www.speedtest.net": true, "127.0.0.1": true, "localhost": true}
	const plistDoctype = "http://www.apple.com/DTDs/PropertyList-1.0.dtd"
	files, _ := filepath.Glob("*.go")
	extra, _ := filepath.Glob("*.html")
	fset := token.NewFileSet()
	urlRe := regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+.-]*://[^\s"'\x60)<>\\]*`)
	for _, f := range append(files, extra...) {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, at := range urlRe.FindAllStringIndex(string(b), -1) {
			u, after := string(b[at[0]:at[1]]), string(b[at[1]:])
			if u == plistDoctype {
				continue
			}
			host := strings.SplitN(strings.SplitN(u, "://", 2)[1], "/", 2)[0]
			host = strings.Split(host, ":")[0]
			// An address built from a name: only Ookla's fixed host or the local retire address.
			switch {
			case host == "" && u == "https://" && strings.HasPrefix(after, `" + ooklaHost`):
				host = ooklaHost
			case host == "" && u == "http://" && strings.HasPrefix(after, `"+addr`):
				host = "127.0.0.1" // addr is "127.0.0.1:" + the port
			}
			if !allowedHost[host] {
				t.Errorf("%s: outside address %s", f, u)
			}
			if host != "127.0.0.1" && host != "localhost" && !strings.HasPrefix(u, "https://") {
				t.Errorf("%s: %s is not https", f, u)
			}
		}
		if !strings.HasSuffix(f, ".go") {
			continue
		}
		af, err := parser.ParseFile(fset, f, b, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, im := range af.Imports {
			switch p, _ := strconv.Unquote(im.Path.Value); p {
			case "net/smtp", "mime/multipart", "net/rpc", "net/http/httputil":
				t.Errorf("%s imports %s", f, p)
			}
		}
		// Every reach outward sits in one of these: the Ookla fetch (and its client), the local page
		// server, and the retire request to the local open page.
		allowedFunc := map[string]bool{"addOokla": true, "fill": true, "Serve": true, "retireOpenPage": true}
		for _, d := range af.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				x, ok := sel.X.(*ast.Ident)
				if !ok {
					return true
				}
				name := x.Name + "." + sel.Sel.Name
				outward := map[string]bool{"http.Get": true, "http.Post": true, "http.PostForm": true, "http.Head": true,
					"http.NewRequest": true, "http.NewRequestWithContext": true, "http.Client": true, "http.DefaultClient": true,
					"net.Dial": true, "net.DialTimeout": true, "net.Dialer": true, "net.Listen": true, "e.HTTP": true}
				if outward[name] && !allowedFunc[fd.Name.Name] {
					t.Errorf("%s: %s used in %s", f, name, fd.Name.Name)
				}
				return true
			})
		}
	}
	// The local addresses really are local: the page listens on 127.0.0.1 and retire calls 127.0.0.1.
	srv, work := repoFile(t, "internal/install/server.go"), repoFile(t, "internal/install/work.go")
	if !strings.Contains(srv, `net.Listen("tcp", "127.0.0.1:0")`) || !strings.Contains(work, `addr := "127.0.0.1:" + st.Port`) {
		t.Error("the local page or the retire request is not on 127.0.0.1")
	}
	_ = io.Discard
}

// An empty leftover ookla folder does not count as Ookla's tool.
func TestEmptyOoklaFolderDoesNotCount(t *testing.T) {
	s := newScratch(t, "linux")
	Install(s.env, Choice{}, io.Discard)
	l := LayoutFor(s.env)
	os.MkdirAll(filepath.Join(l.ProgramDir, "ookla"), 0o755)
	Install(s.env, Choice{}, io.Discard)
	if choicesIn(t, l.ProgramDir)["ookla"] {
		t.Error("an empty ookla folder marked Ookla as there")
	}
}

// A retired open page may take a while to hear; the wait before stopping it is 12 seconds.
func TestRetireWaitIsTwelveSeconds(t *testing.T) {
	e := Env{}
	e.fill()
	if e.RetireWait != 12*time.Second {
		t.Errorf("RetireWait %v", e.RetireWait)
	}
}
