package commands

import (
	"bytes"
	"encoding/json"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/Coding-for-Machine/cfm-sendbox/internal/cli/ws"
)

type fakeAPI struct {
	mu       sync.Mutex
	packages []map[string]any
	calls    []string // "METHOD path body"
	execReq  map[string]any
	execResp string
	wsInit   chan map[string]any
	wsHook   func(c *ws.Conn)
}

func (f *fakeAPI) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/packages", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(f.packages)
			return
		}
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		b, _ := json.Marshal(body)
		f.calls = append(f.calls, r.Method+" "+string(b))
		if body["language"] == "broken" {
			w.WriteHeader(404)
			w.Write([]byte(`{"message":"Requested package broken-* does not exist"}`))
			return
		}
		v := body["version"]
		if v == "*" {
			v = "9.9.9"
		}
		json.NewEncoder(w).Encode(map[string]string{"language": body["language"], "version": v})
	})
	mux.HandleFunc("/api/v2/runtimes", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"language":"python","version":"3.12.0","aliases":["py","python3"]},{"language":"go","version":"1.23.4","aliases":[]}]`))
	})
	mux.HandleFunc("/api/v2/execute", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		json.NewDecoder(r.Body).Decode(&f.execReq)
		w.Write([]byte(f.execResp))
	})
	mux.HandleFunc("/api/v2/connect", func(w http.ResponseWriter, r *http.Request) {
		c, err := ws.Accept(w, r)
		if err != nil {
			return
		}
		_, data, err := c.ReadMessage()
		if err != nil {
			return
		}
		var init map[string]any
		json.Unmarshal(data, &init)
		f.wsInit <- init
		f.wsHook(c)
	})
	return mux
}

func newFake(t *testing.T) (*fakeAPI, string) {
	f := &fakeAPI{wsInit: make(chan map[string]any, 1)}
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)
	return f, srv.URL
}

func run(t *testing.T, stdin string, args ...string) (code int, out, errOut string) {
	t.Helper()
	var o, e bytes.Buffer
	env := Env{In: strings.NewReader(stdin), Out: &o, Err: &e, Getenv: func(string) string { return "" }}
	code = Run(args, env)
	return code, o.String(), e.String()
}

func TestRuntimes(t *testing.T) {
	_, url := newFake(t)
	code, out, _ := run(t, "", "-u", url, "runtimes")
	want := "python 3.12.0 (py, python3)\ngo 1.23.4\n"
	if code != 0 || out != want {
		t.Fatalf("code %d, out %q", code, out)
	}
}

func TestPpmanListInstallUninstall(t *testing.T) {
	f, url := newFake(t)
	f.packages = []map[string]any{
		{"language": "python", "language_version": "3.12.0", "installed": true},
		{"language": "go", "language_version": "1.23.4", "installed": false},
	}

	code, out, _ := run(t, "", "--cfm-url="+url, "ppman", "list")
	if code != 0 || out != "python 3.12.0 (INSTALLED)\ngo 1.23.4\n" {
		t.Fatalf("list: code %d out %q", code, out)
	}

	// CFM_URL env var + alias "pkg i", version defaults to "*", flags after args
	var o, e bytes.Buffer
	env := Env{In: strings.NewReader(""), Out: &o, Err: &e,
		Getenv: func(k string) string {
			if k == "CFM_URL" {
				return url
			}
			return ""
		}}
	code = Run([]string{"pkg", "i", "python=3.12.0", "go", "broken", "-u", url}, env)
	if code != 1 {
		t.Fatalf("install exit code = %d, want 1 (one package failed)", code)
	}
	if !strings.Contains(o.String(), "Installation succeeded: python 3.12.0") ||
		!strings.Contains(o.String(), "Installation succeeded: go 9.9.9") {
		t.Fatalf("stdout: %q", o.String())
	}
	if !strings.Contains(e.String(), "Requested package broken-* does not exist") {
		t.Fatalf("stderr: %q", e.String())
	}
	want := []string{
		`POST {"language":"python","version":"3.12.0"}`,
		`POST {"language":"go","version":"*"}`,
		`POST {"language":"broken","version":"*"}`,
	}
	if !reflect.DeepEqual(f.calls, want) {
		t.Fatalf("calls = %v", f.calls)
	}

	f.calls = nil
	code, out, _ = run(t, "", "-u", url, "ppman", "uninstall", "python=>=3.10")
	if code != 0 || !strings.Contains(out, "Uninstallation succeeded") ||
		f.calls[0] != `DELETE {"language":"python","version":"\u003e=3.10"}` {
		t.Fatalf("uninstall: code %d out %q calls %v", code, out, f.calls)
	}
}

func TestPlanSpec(t *testing.T) {
	pkg := func(l, v string, inst bool) map[string]any {
		return map[string]any{"language": l, "language_version": v, "installed": inst}
	}
	f, url := newFake(t)
	f.packages = []map[string]any{
		pkg("python", "2.7.18", false), pkg("python", "3.9.1", false),
		pkg("python", "3.12.0", false), pkg("python", "3.10.0-alpha.7", false),
		pkg("go", "1.16.2", true), pkg("go", "1.23.4", false),
		pkg("node", "20.11.1", true), pkg("node", "22.1.0", false),
		pkg("ruby", "3.4.0", false),
	}
	dir := t.TempDir()
	spec := filepath.Join(dir, "spec.pps")
	os.WriteFile(spec, []byte(`#!/usr/bin/env -S cfm ppman spec
# comment
* *
!ruby *
!python *
  python 3.*
python 2.*
!node 22.*
`), 0o644)

	code, out, errOut := run(t, "", "-u", url, "ppman", "spec", spec)
	if code != 0 {
		t.Fatalf("code %d: %s", code, errOut)
	}
	// "* *" selects only the newest version of each language, so node 22.1.0 is
	// wanted and then removed by "!node 22.*"; the installed node 20.11.1 is
	// not wanted any more and is uninstalled, like go 1.16.2.
	for _, want := range []string{
		"Install go 1.23.4", "Install python 3.12.0", "Install python 2.7.18",
		"Uninstall go 1.16.2", "Uninstall node 20.11.1",
		"Installed go 1.23.4", "Uninstalled go 1.16.2", "Uninstalled node 20.11.1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	for _, bad := range []string{"ruby", "alpha", "22.1.0", "3.9.1"} {
		if strings.Contains(out, bad) {
			t.Errorf("output must not mention %q:\n%s", bad, out)
		}
	}

	// Nothing to do -> "None"
	f.packages = []map[string]any{pkg("go", "1.23.4", true)}
	os.WriteFile(spec, []byte("go *\n"), 0o644)
	_, out, _ = run(t, "", "-u", url, "ppman", "spec", spec)
	if !strings.Contains(out, "None") {
		t.Fatalf("expected None, got %q", out)
	}
}

func TestRunBatch(t *testing.T) {
	f, url := newFake(t)
	f.execResp = `{"language":"python","version":"3.12.0",
		"run":{"stdout":"hello\nworld\n","stderr":"","output":"hello\nworld\n","code":0,"signal":null}}`
	dir := t.TempDir()
	main := filepath.Join(dir, "main.py")
	helper := filepath.Join(dir, "helper.bin")
	os.WriteFile(main, []byte("print('hi')\n"), 0o644)
	os.WriteFile(helper, []byte{0xff, 0xfe, 0x00}, 0o644) // invalid UTF-8

	// flags before the file, program args after it (including dash options)
	code, out, errOut := run(t, "input data", "-u", url, "run", "-l", "3.x", "-i", "-r", "1500",
		"-f", helper, "python", main, "a", "-b", "--c")
	if code != 0 {
		t.Fatalf("code %d, stderr %s", code, errOut)
	}
	if !strings.Contains(out, "== Run ==") || !strings.Contains(out, "STDOUT\nhello\n    world\n") ||
		!strings.Contains(out, "Exit Code: 0") {
		t.Fatalf("unexpected output:\n%s", out)
	}
	if strings.Contains(out, "== Compile ==") {
		t.Fatalf("no compile stage expected:\n%s", out)
	}

	r := f.execReq
	if r["language"] != "python" || r["version"] != "3.x" || r["stdin"] != "input data" ||
		r["run_timeout"] != float64(1500) {
		t.Fatalf("request: %v", r)
	}
	if _, set := r["compile_timeout"]; set {
		t.Fatalf("compile_timeout must be omitted when not given: %v", r)
	}
	if !reflect.DeepEqual(r["args"], []any{"a", "-b", "--c"}) {
		t.Fatalf("args: %v", r["args"])
	}
	files := r["files"].([]any)
	first := files[0].(map[string]any)
	second := files[1].(map[string]any)
	if first["name"] != "main.py" || first["encoding"] != "utf8" || first["content"] != "print('hi')\n" {
		t.Fatalf("main file must be first: %v", first)
	}
	if second["name"] != "helper.bin" || second["encoding"] != "base64" || second["content"] != "//4A" {
		t.Fatalf("binary file: %v", second)
	}
}

func TestRunBatchCompileError(t *testing.T) {
	f, url := newFake(t)
	stage := `{"stdout":"","stderr":"main.c:1: error\n","output":"main.c:1: error\n","code":1,"signal":null}`
	f.execResp = `{"language":"gcc","version":"15.2.0","compile":` + stage + `,"run":` + stage + `}`
	file := filepath.Join(t.TempDir(), "main.c")
	os.WriteFile(file, []byte("x"), 0o644)

	code, out, _ := run(t, "", "-u", url, "run", "gcc", file)
	if code != 0 || strings.Count(out, "main.c:1: error") != 1 || strings.Contains(out, "== Run ==") {
		t.Fatalf("compile error must be printed once:\n%s", out)
	}
	if !strings.Contains(out, "Exit Code: 1") {
		t.Fatalf("missing exit code: %s", out)
	}
}

func TestRunInteractive(t *testing.T) {
	f, url := newFake(t)
	gotStdin := make(chan string, 1)
	f.wsHook = func(c *ws.Conn) {
		send := func(v map[string]any) {
			b, _ := json.Marshal(v)
			c.WriteText(string(b))
		}
		send(map[string]any{"type": "runtime", "language": "python", "version": "3.12.0"})
		send(map[string]any{"type": "stage", "stage": "run"})
		send(map[string]any{"type": "data", "stream": "stdout", "data": "name? "})
		// the client types a name
		_, data, _ := c.ReadMessage()
		gotStdin <- string(data)
		send(map[string]any{"type": "data", "stream": "stdout", "data": "hello bob\n"})
		send(map[string]any{"type": "data", "stream": "stderr", "data": "warn\n"})
		send(map[string]any{"type": "exit", "stage": "run", "code": 0, "signal": nil})
		c.CloseWith(4999, "Job Completed")
	}

	dir := t.TempDir()
	file := filepath.Join(dir, "main.py")
	os.WriteFile(file, []byte("print(input())"), 0o644)

	var o, e bytes.Buffer
	env := Env{In: strings.NewReader("bob\n"), Out: &o, Err: &e, StatusTTY: true,
		Getenv: func(string) string { return "" }}
	code := Run([]string{"-u", url, "run", "-t", "-s", "-c", "2000", "python", file, "arg1"}, env)
	if code != 0 {
		t.Fatalf("code %d, stderr %q", code, e.String())
	}
	if o.String() != "name? hello bob\n" {
		t.Fatalf("stdout %q", o.String())
	}
	for _, want := range []string{"warn\n", "Connected", "Runtime: python 3.12.0", "Stage: run",
		"exited with code 0", `Reason: "Job Completed"`, "Code: 4999"} {
		if !strings.Contains(e.String(), want) {
			t.Errorf("stderr lacks %q:\n%s", want, e.String())
		}
	}

	init := <-f.wsInit
	if init["type"] != "init" || init["language"] != "python" || init["compile_timeout"] != float64(2000) {
		t.Fatalf("init message: %v", init)
	}
	if _, set := init["run_timeout"]; set {
		t.Fatalf("run_timeout must be omitted: %v", init)
	}
	if !reflect.DeepEqual(init["args"], []any{"arg1"}) {
		t.Fatalf("args: %v", init["args"])
	}
	if s := <-gotStdin; !strings.Contains(s, `"stream":"stdin"`) || !strings.Contains(s, `"data":"bob\n"`) {
		t.Fatalf("stdin message: %s", s)
	}
}

func TestRunInteractiveServerError(t *testing.T) {
	f, url := newFake(t)
	f.wsHook = func(c *ws.Conn) {
		c.WriteText(`{"type":"error","message":"python-9.x runtime is unknown"}`)
		c.CloseWith(4002, "Notified Error")
	}
	file := filepath.Join(t.TempDir(), "m.py")
	os.WriteFile(file, []byte("x"), 0o644)
	code, _, errOut := run(t, "", "-u", url, "run", "-t", "-l", "9.x", "python", file)
	if code != 1 || !strings.Contains(errOut, "python-9.x runtime is unknown") {
		t.Fatalf("code %d stderr %q", code, errOut)
	}
}

func TestUsageAndErrors(t *testing.T) {
	if code, out, _ := run(t, "", "--help"); code != 0 || !strings.Contains(out, "Usage:") {
		t.Fatalf("help: %d %q", code, out)
	}
	if code, out, _ := run(t, "", "version"); code != 0 || !strings.HasPrefix(out, "cfm-cli ") {
		t.Fatalf("version: %d %q", code, out)
	}
	if code, _, e := run(t, ""); code != 2 || !strings.Contains(e, "Usage:") {
		t.Fatalf("no args: %d", code)
	}
	if code, _, e := run(t, "", "bogus"); code != 2 || !strings.Contains(e, "unknown command") {
		t.Fatalf("bogus: %d %q", code, e)
	}
	if code, _, _ := run(t, "", "run", "python"); code != 2 {
		t.Fatalf("run without file: %d", code)
	}
	if code, _, _ := run(t, "", "run", "python", "/does/not/exist"); code != 1 {
		t.Fatalf("missing file: %d", code)
	}
	// connection refused
	if code, _, e := run(t, "", "-u", "http://127.0.0.1:1", "runtimes"); code != 1 || e == "" {
		t.Fatalf("refused: %d %q", code, e)
	}
}

func TestParseInterleaved(t *testing.T) {
	newFS := func() (*flag.FlagSet, *string, *bool) {
		fs := flag.NewFlagSet("t", flag.ContinueOnError)
		fs.SetOutput(&bytes.Buffer{})
		s := fs.String("l", "*", "")
		b := fs.Bool("i", false, "")
		return fs, s, b
	}
	cases := []struct {
		args      []string
		max       int
		pos, rest []string
		l         string
		i         bool
	}{
		{[]string{"python", "-l", "3", "f.py"}, 2, []string{"python", "f.py"}, nil, "3", false},
		{[]string{"-i", "python", "f.py", "-l", "x"}, 2, []string{"python", "f.py"}, []string{"-l", "x"}, "*", true},
		{[]string{"python", "f.py", "--", "-i"}, 2, []string{"python", "f.py"}, []string{"-i"}, "*", false},
		{[]string{"--", "python", "f.py", "x"}, 2, []string{"python", "f.py"}, []string{"x"}, "*", false},
		{[]string{"a", "-l=5", "b", "c"}, -1, []string{"a", "b", "c"}, nil, "5", false},
	}
	for _, c := range cases {
		fs, l, i := newFS()
		pos, rest, err := parseInterleaved(fs, c.args, c.max)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(pos, c.pos) || !reflect.DeepEqual(rest, c.rest) || *l != c.l || *i != c.i {
			t.Errorf("%v: pos=%v rest=%v l=%s i=%v", c.args, pos, rest, *l, *i)
		}
	}
}
