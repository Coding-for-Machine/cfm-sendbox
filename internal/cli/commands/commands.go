// Package commands implements the cfm CLI sub-commands.
package commands

import (
	"errors"
	"flag"
	"fmt"
	"io"
	neturl "net/url"
	"os"
	"strings"

	"github.com/Coding-for-Machine/cfm-sendbox/internal/cli/client"
	"github.com/Coding-for-Machine/cfm-sendbox/internal/cli/term"
)

// Version of the CLI.
const Version = "2.0.0"

// DefaultURL is used when neither -u nor CFM_URL is given.
const DefaultURL = "http://127.0.0.1:2000"

// Env bundles everything a command needs from the outside world, so tests can
// run commands against fake servers and in-memory streams.
type Env struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer

	// Color enables ANSI colors on Out; StatusTTY controls the status lines
	// of interactive mode (which are only shown on a terminal).
	Color bool

	// APIKey overrides $CFM_API_KEY.
	APIKey    string
	StatusTTY bool

	// Signals delivers OS signals to forward in interactive mode. When nil the
	// execute command subscribes to the real ones.
	Signals <-chan os.Signal

	Getenv func(string) string
}

// NewEnv returns an Env wired to the process' standard streams.
func NewEnv() Env {
	return Env{
		In:        os.Stdin,
		Out:       os.Stdout,
		Err:       os.Stderr,
		Color:     term.ColorEnabled(os.Stdout),
		StatusTTY: term.IsTTY(os.Stderr),
		Getenv:    os.Getenv,
	}
}

// newClient creates an API client carrying the API key, if any.
func (e Env) newClient(url string) *client.Client {
	key := e.APIKey
	if key == "" {
		key = e.getenv("CFM_API_KEY")
	}
	return client.New(url).WithKey(key)
}

// wsURL adds the API key to a WebSocket URL (headers cannot be set by every
// WebSocket client, so the server also accepts the api_key query parameter).
func (e Env) wsURL(u string) string {
	key := e.APIKey
	if key == "" {
		key = e.getenv("CFM_API_KEY")
	}
	if key == "" {
		return u
	}
	return u + "?api_key=" + neturl.QueryEscape(key)
}

func (e Env) paint() term.Painter { return term.Painter{On: e.Color} }

func (e Env) getenv(k string) string {
	if e.Getenv == nil {
		return ""
	}
	return e.Getenv(k)
}

const usage = `cfm - cfm-sendbox CLI

Usage:
  cfm [-u URL] <command> [options]

Commands:
  execute, run <language> <file> [args...]   Execute a file with the specified runtime
  ppman, pkg install|i <package[=version]...> Install packages
  ppman, pkg uninstall|u <package[=version]...> Uninstall packages
  ppman, pkg list|l                           List the available packages
  ppman, pkg spec|s <specfile>                Install the packages in a spec file and
                                              uninstall those which aren't listed
  runtimes                                    List the installed runtimes
  version                                     Print the CLI version

Global options:
  -u, --cfm-url URL   API URL (default $CFM_URL or ` + DefaultURL + `)
  -k, --api-key KEY      API key (default $CFM_API_KEY)
  -h, --help             Show help
`

const executeUsage = `Usage: cfm run [options] <language> <file> [args...]

Executes <file> with the specified runner. Options must come before the file;
everything after the file name is passed to the program as arguments.

Options:
  -l, --language_version VER  Version (or semver range) of the language (default "*")
  -i, --stdin                 Read stdin until EOF and pass it to the program
  -r, --run_timeout MS        Milliseconds before killing the run process
  -c, --compile_timeout MS    Milliseconds before killing the compile process
  -f, --files FILE            Additional file to add (repeatable)
  -t, --interactive           Run interactively using WebSocket transport
  -s, --status                Print additional status to stderr (interactive, TTY only)
  -u, --cfm-url URL        API URL
`

const ppmanUsage = `Usage: cfm ppman <command>

Commands:
  install, i <package[=version]...>   Installs the named packages (version defaults to "*")
  uninstall, u <package[=version]...> Uninstalls the named packages
  list, l                             Lists all available packages
  spec, s <specfile>                  Installs the packages described in the spec file,
                                      uninstalling packages which aren't in the list
`

// Run executes the CLI with the given arguments (without the program name) and
// returns the process exit code.
func Run(args []string, env Env) int {
	url := env.getenv("CFM_URL")
	if url == "" {
		url = env.getenv("PISTON_URL") // legacy name
	}
	if url == "" {
		url = DefaultURL
	}

	// Global flags before the command.
	i := 0
loop:
	for i < len(args) {
		a := args[i]
		switch {
		case a == "-h" || a == "--help" || a == "-help":
			fmt.Fprint(env.Out, usage)
			return 0
		case a == "-u" || a == "--cfm-url" || a == "-cfm-url":
			if i+1 >= len(args) {
				fmt.Fprintf(env.Err, "flag needs an argument: %s\n", a)
				return 2
			}
			url = args[i+1]
			i += 2
		case a == "-k" || a == "--api-key":
			if i+1 >= len(args) {
				fmt.Fprintf(env.Err, "flag needs an argument: %s\n", a)
				return 2
			}
			env.APIKey = args[i+1]
			i += 2
		case strings.HasPrefix(a, "--api-key="):
			env.APIKey = strings.TrimPrefix(a, "--api-key=")
			i++
		case strings.HasPrefix(a, "--cfm-url="):
			url = strings.TrimPrefix(a, "--cfm-url=")
			i++
		case strings.HasPrefix(a, "-u="):
			url = strings.TrimPrefix(a, "-u=")
			i++
		default:
			break loop
		}
	}
	args = args[i:]
	if len(args) == 0 {
		fmt.Fprint(env.Err, usage)
		return 2
	}

	cmd, rest := args[0], args[1:]
	switch cmd {
	case "execute", "run":
		return runExecute(rest, env, url)
	case "ppman", "pkg":
		return runPpman(rest, env, url)
	case "runtimes":
		return runRuntimes(rest, env, url)
	case "version", "--version", "-v":
		fmt.Fprintln(env.Out, "cfm-cli", Version)
		return 0
	case "help":
		fmt.Fprint(env.Out, usage)
		return 0
	}
	fmt.Fprintf(env.Err, "unknown command %q\n\n%s", cmd, usage)
	return 2
}

func runPpman(args []string, env Env, url string) int {
	// Allow global flags after "ppman" too.
	for len(args) >= 2 && (args[0] == "-u" || args[0] == "--cfm-url") {
		url = args[1]
		args = args[2:]
	}
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(env.Out, ppmanUsage)
		if len(args) == 0 {
			return 2
		}
		return 0
	}

	sub, rest := args[0], args[1:]
	switch sub {
	case "install", "i":
		return ppmanChange(rest, env, url, true)
	case "uninstall", "u":
		return ppmanChange(rest, env, url, false)
	case "list", "l":
		return ppmanList(rest, env, url)
	case "spec", "s":
		return ppmanSpec(rest, env, url)
	}
	fmt.Fprintf(env.Err, "unknown ppman command %q\n\n%s", sub, ppmanUsage)
	return 2
}

func runRuntimes(args []string, env Env, url string) int {
	fs := newFlagSet("runtimes", env)
	fs.StringVar(&url, "u", url, "")
	fs.StringVar(&url, "cfm-url", url, "")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rts, err := env.newClient(url).Runtimes()
	if err != nil {
		fmt.Fprintln(env.Err, err)
		return 1
	}
	for _, rt := range rts {
		line := rt.Language + " " + rt.Version
		if len(rt.Aliases) > 0 {
			line += " (" + strings.Join(rt.Aliases, ", ") + ")"
		}
		fmt.Fprintln(env.Out, line)
	}
	return 0
}

func newFlagSet(name string, env Env) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(env.Err)
	return fs
}

// splitDashDash splits args at the first literal "--".
func splitDashDash(args []string) (before, after []string, found bool) {
	for i, a := range args {
		if a == "--" {
			return args[:i], args[i+1:], true
		}
	}
	return args, nil, false
}

// parseInterleaved parses flags and positional arguments in any order. It
// collects at most maxPos positionals (all of them when maxPos < 0); once that
// many are found the remaining arguments are returned untouched in rest.
func parseInterleaved(fs *flag.FlagSet, args []string, maxPos int) (pos, rest []string, err error) {
	before, after, found := splitDashDash(args)

	for {
		// Once all positionals are known, the rest belongs to the program.
		if maxPos >= 0 && len(pos) >= maxPos {
			rest = before
			break
		}
		if err = fs.Parse(before); err != nil {
			return nil, nil, err
		}
		remaining := fs.Args()
		if len(remaining) == 0 {
			break
		}
		pos = append(pos, remaining[0])
		before = remaining[1:]
	}

	if len(rest) == 0 {
		rest = nil
	}
	if found {
		if maxPos < 0 {
			pos = append(pos, after...)
		} else {
			for len(after) > 0 && len(pos) < maxPos {
				pos = append(pos, after[0])
				after = after[1:]
			}
			rest = append(rest, after...)
		}
	}
	return pos, rest, nil
}

// parseCode maps a flag parsing error to the process exit code (-h is not an error).
func parseCode(err error) int {
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	return 2
}

// stringList is a repeatable string flag.
type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }
