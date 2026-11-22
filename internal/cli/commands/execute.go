package commands

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/Coding-for-Machine/cfm-sendbox/internal/cli/client"
	"github.com/Coding-for-Machine/cfm-sendbox/internal/cli/term"
	"github.com/Coding-for-Machine/cfm-sendbox/internal/cli/ws"
)

type executeOpts struct {
	language  string
	version   string
	file      string
	extra     stringList
	args      []string
	stdin     bool
	status    bool
	inter     bool
	compileTO *int
	runTO     *int
}

// intFlag registers an optional integer flag under several names. The target
// stays nil unless the flag is given.
func intFlag(fs interface {
	Func(name, usage string, fn func(string) error)
}, target **int, usage string, names ...string) {
	for _, n := range names {
		fs.Func(n, usage, func(s string) error {
			v, err := strconv.Atoi(s)
			if err != nil {
				return fmt.Errorf("%q is not a number", s)
			}
			*target = &v
			return nil
		})
	}
}

func runExecute(args []string, env Env, url string) int {
	o := executeOpts{version: "*"}
	fs := newFlagSet("run", env)
	fs.Usage = func() { fmt.Fprint(env.Err, executeUsage) }

	for _, n := range []string{"l", "language_version", "language-version"} {
		fs.StringVar(&o.version, n, o.version, "")
	}
	for _, n := range []string{"i", "stdin"} {
		fs.BoolVar(&o.stdin, n, false, "")
	}
	for _, n := range []string{"t", "interactive"} {
		fs.BoolVar(&o.inter, n, false, "")
	}
	for _, n := range []string{"s", "status"} {
		fs.BoolVar(&o.status, n, false, "")
	}
	for _, n := range []string{"f", "files"} {
		fs.Var(&o.extra, n, "")
	}
	for _, n := range []string{"u", "cfm-url"} {
		fs.StringVar(&url, n, url, "")
	}
	intFlag(fs, &o.runTO, "", "r", "rt", "run_timeout", "run-timeout")
	intFlag(fs, &o.compileTO, "", "c", "ct", "compile_timeout", "compile-timeout")

	pos, rest, err := parseInterleaved(fs, args, 2)
	if err != nil {
		return parseCode(err)
	}
	if len(pos) < 2 {
		fmt.Fprint(env.Err, executeUsage)
		return 2
	}
	o.language, o.file, o.args = pos[0], pos[1], rest

	files, err := loadFiles(append([]string{o.file}, o.extra...))
	if err != nil {
		fmt.Fprintln(env.Err, err)
		return 1
	}

	req := client.ExecuteRequest{
		Language:       o.language,
		Version:        o.version,
		Files:          files,
		Args:           o.args,
		CompileTimeout: o.compileTO,
		RunTimeout:     o.runTO,
	}
	if req.Args == nil {
		req.Args = []string{}
	}

	if o.inter {
		return runInteractive(env, url, req, o.status)
	}
	return runBatch(env, url, req, o.stdin)
}

// loadFiles reads the files for the request. A file that is not valid UTF-8 is
// sent base64 encoded.
func loadFiles(paths []string) ([]client.File, error) {
	files := make([]client.File, 0, len(paths))
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		f := client.File{Name: filepath.Base(p)}
		if utf8.Valid(data) {
			f.Content, f.Encoding = string(data), "utf8"
		} else {
			f.Content, f.Encoding = base64.StdEncoding.EncodeToString(data), "base64"
		}
		files = append(files, f)
	}
	return files, nil
}

func printStage(env Env, name string, s *client.Stage) {
	p := env.paint()
	fmt.Fprintln(env.Out, p.Bold("== "+name+" =="))
	if s.Stdout != "" {
		fmt.Fprintln(env.Out, p.Bold("STDOUT"))
		fmt.Fprintln(env.Out, term.Indent(strings.TrimSuffix(s.Stdout, "\n")))
	}
	if s.Stderr != "" {
		fmt.Fprintln(env.Out, p.Bold("STDERR"))
		fmt.Fprintln(env.Out, term.Indent(strings.TrimSuffix(s.Stderr, "\n")))
	}
	if s.Message != nil && *s.Message != "" {
		fmt.Fprintln(env.Out, p.Bold("Message:"), p.Yellow(*s.Message))
	}
	if s.Code != nil {
		code := strconv.Itoa(*s.Code)
		if *s.Code != 0 {
			code = p.BoldRed(code)
		} else {
			code = p.BoldGreen(code)
		}
		fmt.Fprintln(env.Out, p.Bold("Exit Code:"), code)
	}
	if s.Signal != nil {
		fmt.Fprintln(env.Out, p.Bold("Signal:"), p.BoldYellow(*s.Signal))
	}
}

func runBatch(env Env, url string, req client.ExecuteRequest, readStdin bool) int {
	if readStdin {
		data, err := io.ReadAll(env.In)
		if err != nil {
			fmt.Fprintln(env.Err, err)
			return 1
		}
		req.Stdin = string(data)
	}

	res, err := env.newClient(url).Execute(req)
	if err != nil {
		fmt.Fprintln(env.Err, err)
		return 1
	}
	if res.Compile != nil {
		printStage(env, "Compile", res.Compile)
	}
	// When compilation fails the server repeats the compile result as "run".
	if res.Run != nil && !(res.Compile != nil && reflect.DeepEqual(res.Compile, res.Run)) {
		printStage(env, "Run", res.Run)
	}
	return 0
}

func signalName(s os.Signal) string {
	switch s {
	case syscall.SIGINT:
		return "SIGINT"
	case syscall.SIGTERM:
		return "SIGTERM"
	case syscall.SIGHUP:
		return "SIGHUP"
	case syscall.SIGQUIT:
		return "SIGQUIT"
	}
	return ""
}

func runInteractive(env Env, url string, req client.ExecuteRequest, status bool) int {
	logf := func(format string, a ...any) {
		if env.StatusTTY && status {
			fmt.Fprintf(env.Err, format+"\n", a...)
		}
	}
	p := term.Painter{On: env.Color}

	conn, err := ws.Dial(context.Background(), env.wsURL(strings.TrimRight(url, "/")+"/api/v2/connect"))
	if err != nil {
		fmt.Fprintln(env.Err, err)
		return 1
	}
	defer conn.Close()

	send := func(v any) error {
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		return conn.WriteText(string(b))
	}

	initMsg := map[string]any{
		"type":     "init",
		"language": req.Language,
		"version":  req.Version,
		"files":    req.Files,
		"args":     req.Args,
	}
	if req.CompileTimeout != nil {
		initMsg["compile_timeout"] = *req.CompileTimeout
	}
	if req.RunTimeout != nil {
		initMsg["run_timeout"] = *req.RunTimeout
	}
	if err := send(initMsg); err != nil {
		fmt.Fprintln(env.Err, err)
		return 1
	}
	logf("%s", p.Bold("Connected"))

	// stdin -> server
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := env.In.Read(buf)
			if n > 0 {
				if send(map[string]any{"type": "data", "stream": "stdin", "data": string(buf[:n])}) != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// signals -> server. Pressing Ctrl-C twice within two seconds gives up.
	var aborted atomic.Bool
	sigs := env.Signals
	if sigs == nil {
		ch := make(chan os.Signal, 8)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
		defer signal.Stop(ch)
		sigs = ch
	}
	go func() {
		var lastInt time.Time
		for s := range sigs {
			name := signalName(s)
			if name == "" {
				continue
			}
			if name == "SIGINT" {
				if !lastInt.IsZero() && time.Since(lastInt) < 2*time.Second {
					aborted.Store(true)
					conn.Close()
					return
				}
				lastInt = time.Now()
			}
			_ = send(map[string]any{"type": "signal", "signal": name})
		}
	}()

	exit := 0
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			var ce *ws.CloseError
			if errors.As(err, &ce) {
				logf("%s %s %q %s %d", p.Bold("Disconnected:"), p.Bold("Reason:"), ce.Reason, p.Bold("Code:"), ce.Code)
			} else if !aborted.Load() {
				fmt.Fprintln(env.Err, err)
				exit = 1
			}
			break
		}

		var msg struct {
			Type     string  `json:"type"`
			Stage    string  `json:"stage"`
			Stream   string  `json:"stream"`
			Data     string  `json:"data"`
			Language string  `json:"language"`
			Version  string  `json:"version"`
			Message  string  `json:"message"`
			Code     *int    `json:"code"`
			Signal   *string `json:"signal"`
		}
		if json.Unmarshal(data, &msg) != nil {
			logf("%s %s", p.BoldRed("Unreadable message:"), data)
			continue
		}

		switch msg.Type {
		case "runtime":
			logf("%s %s", p.Bold("Runtime:"), p.Yellow(msg.Language+" "+msg.Version))
		case "stage":
			logf("%s %s", p.Bold("Stage:"), p.Yellow(msg.Stage))
		case "data":
			switch msg.Stream {
			case "stdout":
				io.WriteString(env.Out, msg.Data)
			case "stderr":
				io.WriteString(env.Err, msg.Data)
			default:
				logf("%s %s", p.BoldRed("("+msg.Stream+")"), msg.Data)
			}
		case "exit":
			if msg.Signal == nil {
				code := "null"
				if msg.Code != nil {
					code = strconv.Itoa(*msg.Code)
				}
				logf("%s %s %s %s", p.Bold("Stage"), p.Yellow(msg.Stage), p.Bold("exited with code"), p.Yellow(code))
			} else {
				logf("%s %s %s %s", p.Bold("Stage"), p.Yellow(msg.Stage), p.Bold("exited with signal"), p.Yellow(*msg.Signal))
			}
		case "error":
			fmt.Fprintln(env.Err, msg.Message)
			exit = 1
		default:
			logf("%s %s", p.BoldRed("Unknown message:"), data)
		}
	}
	if aborted.Load() {
		return 130
	}
	return exit
}
