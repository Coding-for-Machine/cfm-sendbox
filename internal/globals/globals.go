// Package globals holds constants the user shouldn't change in config.
package globals

import (
	"os"
	"strings"
	"syscall"
)

const (
	// Version of the API (Piston v3 API compatible).
	Version = "3.1.1"
	// PackagesDir is the sub-directory of the data directory holding packages.
	PackagesDir = "packages"
	// PkgInstalledFile marks a package as fully installed.
	PkgInstalledFile = ".ppman-installed"
)

// Signals maps Linux signal numbers to their names.
var Signals = map[int]string{
	1: "SIGHUP", 2: "SIGINT", 3: "SIGQUIT", 4: "SIGILL", 5: "SIGTRAP",
	6: "SIGABRT", 7: "SIGBUS", 8: "SIGFPE", 9: "SIGKILL", 10: "SIGUSR1",
	11: "SIGSEGV", 12: "SIGUSR2", 13: "SIGPIPE", 14: "SIGALRM", 15: "SIGTERM",
	16: "SIGSTKFLT", 17: "SIGCHLD", 18: "SIGCONT", 19: "SIGSTOP", 20: "SIGTSTP",
	21: "SIGTTIN", 22: "SIGTTOU", 23: "SIGURG", 24: "SIGXCPU", 25: "SIGXFSZ",
	26: "SIGVTALRM", 27: "SIGPROF", 28: "SIGWINCH", 29: "SIGIO", 30: "SIGPWR",
	31: "SIGSYS", 34: "SIGRTMIN",
	35: "SIGRTMIN+1", 36: "SIGRTMIN+2", 37: "SIGRTMIN+3", 38: "SIGRTMIN+4",
	39: "SIGRTMIN+5", 40: "SIGRTMIN+6", 41: "SIGRTMIN+7", 42: "SIGRTMIN+8",
	43: "SIGRTMIN+9", 44: "SIGRTMIN+10", 45: "SIGRTMIN+11", 46: "SIGRTMIN+12",
	47: "SIGRTMIN+13", 48: "SIGRTMIN+14", 49: "SIGRTMIN+15",
	50: "SIGRTMAX-14", 51: "SIGRTMAX-13", 52: "SIGRTMAX-12", 53: "SIGRTMAX-11",
	54: "SIGRTMAX-10", 55: "SIGRTMAX-9", 56: "SIGRTMAX-8", 57: "SIGRTMAX-7",
	58: "SIGRTMAX-6", 59: "SIGRTMAX-5", 60: "SIGRTMAX-4", 61: "SIGRTMAX-3",
	62: "SIGRTMAX-2", 63: "SIGRTMAX-1", 64: "SIGRTMAX",
}

var signalsByName = func() map[string]syscall.Signal {
	m := make(map[string]syscall.Signal, len(Signals))
	for n, name := range Signals {
		m[name] = syscall.Signal(n)
	}
	return m
}()

// SignalName returns the name for a signal number.
func SignalName(n int) (string, bool) {
	name, ok := Signals[n]
	return name, ok
}

// SignalByName returns the signal for a name such as "SIGKILL".
func SignalByName(name string) (syscall.Signal, bool) {
	s, ok := signalsByName[name]
	return s, ok
}

// Platform returns "docker-<os id>" or "baremetal-<os id>".
func Platform() string {
	kind := "baremetal"
	if isDocker() {
		kind = "docker"
	}
	return kind + "-" + osID()
}

func isDocker() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	data, err := os.ReadFile("/proc/self/cgroup")
	return err == nil && strings.Contains(string(data), "docker")
}

func osID() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "unknown"
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "ID=") {
			return strings.Trim(strings.TrimPrefix(line, "ID="), `"'`)
		}
	}
	return "unknown"
}
