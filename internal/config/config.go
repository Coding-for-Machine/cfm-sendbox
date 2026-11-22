// Package config loads settings from CFM_* environment variables (legacy
// PISTON_* names are still accepted).
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Limit option names that can be overridden per language.
const (
	MaxProcessCount    = "max_process_count"
	MaxOpenFiles       = "max_open_files"
	MaxFileSize        = "max_file_size"
	CompileMemoryLimit = "compile_memory_limit"
	RunMemoryLimit     = "run_memory_limit"
	CompileTimeout     = "compile_timeout"
	RunTimeout         = "run_timeout"
	CompileCPUTime     = "compile_cpu_time"
	RunCPUTime         = "run_cpu_time"
	OutputMaxSize      = "output_max_size"
)

var overridableKeys = map[string]bool{
	MaxProcessCount: true, MaxOpenFiles: true, MaxFileSize: true,
	CompileMemoryLimit: true, RunMemoryLimit: true,
	CompileTimeout: true, RunTimeout: true,
	CompileCPUTime: true, RunCPUTime: true,
	OutputMaxSize: true,
}

// Config holds every runtime setting.
type Config struct {
	LogLevel          string
	BindAddress       string
	DataDirectory     string
	RunnerUIDMin      int
	RunnerUIDMax      int
	RunnerGIDMin      int
	RunnerGIDMax      int
	DisableNetworking bool
	OutputMaxSize     int
	MaxProcessCount   int
	MaxOpenFiles      int
	MaxFileSize       int
	CompileTimeout    int
	RunTimeout        int
	CompileCPUTime    int
	RunCPUTime        int
	// Memory limits in bytes, -1 means unlimited.
	CompileMemoryLimit int
	RunMemoryLimit     int
	RepoURL            string
	RepoURLs           []string
	// APIKeys is the set of accepted API keys; empty disables authentication.
	APIKeys           []string
	MaxConcurrentJobs int
	// LimitOverrides maps language -> option name -> value.
	LimitOverrides map[string]map[string]int
}

// Base returns the global value of a limit option.
func (c *Config) Base(key string) int {
	switch key {
	case MaxProcessCount:
		return c.MaxProcessCount
	case MaxOpenFiles:
		return c.MaxOpenFiles
	case MaxFileSize:
		return c.MaxFileSize
	case CompileMemoryLimit:
		return c.CompileMemoryLimit
	case RunMemoryLimit:
		return c.RunMemoryLimit
	case CompileTimeout:
		return c.CompileTimeout
	case RunTimeout:
		return c.RunTimeout
	case CompileCPUTime:
		return c.CompileCPUTime
	case RunCPUTime:
		return c.RunCPUTime
	case OutputMaxSize:
		return c.OutputMaxSize
	}
	return 0
}

// Limit resolves a limit for a language: config override, then the package's
// own override, then the global value. Zero means "not set" for overrides.
func (c *Config) Limit(language, key string, packageOverrides map[string]int) int {
	if v, ok := c.LimitOverrides[language][key]; ok && v != 0 {
		return v
	}
	if v, ok := packageOverrides[key]; ok && v != 0 {
		return v
	}
	return c.Base(key)
}

// Load reads and validates the configuration from the environment.
func Load() (*Config, error) {
	c := &Config{}
	var err error

	c.LogLevel = strings.ToUpper(envString("log_level", "INFO"))
	switch c.LogLevel {
	case "DEBUG", "INFO", "WARN", "ERROR", "NONE":
	default:
		return nil, fmt.Errorf("log_level: log level %s does not exist", c.LogLevel)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "2000"
	}
	c.BindAddress = envString("bind_address", "0.0.0.0:"+port)
	c.DataDirectory = envString("data_directory", "/cfm")
	if st, serr := os.Stat(c.DataDirectory); serr != nil || !st.IsDir() {
		return nil, fmt.Errorf("data_directory: directory %s does not exist", c.DataDirectory)
	}

	ints := []struct {
		name string
		def  int
		dst  *int
	}{
		{"runner_uid_min", 1001, &c.RunnerUIDMin},
		{"runner_uid_max", 1500, &c.RunnerUIDMax},
		{"runner_gid_min", 1001, &c.RunnerGIDMin},
		{"runner_gid_max", 1500, &c.RunnerGIDMax},
		{"output_max_size", 1024, &c.OutputMaxSize},
		{"max_process_count", 64, &c.MaxProcessCount},
		{"max_open_files", 2048, &c.MaxOpenFiles},
		{"max_file_size", 10000000, &c.MaxFileSize},
		{"compile_timeout", 10000, &c.CompileTimeout},
		{"run_timeout", 3000, &c.RunTimeout},
		{"compile_cpu_time", 10000, &c.CompileCPUTime},
		{"run_cpu_time", 3000, &c.RunCPUTime},
		{"compile_memory_limit", -1, &c.CompileMemoryLimit},
		{"run_memory_limit", -1, &c.RunMemoryLimit},
		{"max_concurrent_jobs", 64, &c.MaxConcurrentJobs},
	}
	for _, o := range ints {
		if *o.dst, err = envInt(o.name, o.def); err != nil {
			return nil, err
		}
	}
	if c.MaxConcurrentJobs <= 0 {
		return nil, fmt.Errorf("max_concurrent_jobs: %d must be positive", c.MaxConcurrentJobs)
	}

	c.DisableNetworking = true
	if v, ok := lookup("disable_networking"); ok {
		c.DisableNetworking = v == "true"
	}

	// Comma separated list of package indexes. The first index wins when the
	// same language/version is listed twice. The upstream Piston index is a
	// fallback so that prebuilt packages are installable right away.
	c.RepoURL = envString("repo_url", DefaultRepoURL)
	if strings.TrimSpace(c.RepoURL) == "" {
		c.RepoURL = DefaultRepoURL
	}
	c.RepoURLs = splitList(c.RepoURL)

	c.APIKeys = splitList(envString("api_keys", ""))

	c.LimitOverrides = map[string]map[string]int{}
	if raw, ok := lookup("limit_overrides"); ok {
		if c.LimitOverrides, err = ParseOverrides(raw); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// ParseOverrides parses the per-language limit overrides JSON.
func ParseOverrides(raw string) (map[string]map[string]int, error) {
	var parsed map[string]map[string]float64
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, fmt.Errorf("limit_overrides: failed to parse the overrides: %w", err)
	}
	out := make(map[string]map[string]int, len(parsed))
	for lang, opts := range parsed {
		out[lang] = make(map[string]int, len(opts))
		for key, v := range opts {
			if !overridableKeys[key] {
				return nil, fmt.Errorf("limit_overrides: option %q for %s cannot be overridden", key, lang)
			}
			out[lang][key] = int(v)
		}
	}
	return out, nil
}

// DefaultRepoURL lists the package indexes used when CFM_REPO_URL is not set.
const DefaultRepoURL = "https://github.com/Coding-for-Machine/cfm-sendbox/releases/download/pkgs/index," +
	"https://github.com/engineer-man/piston/releases/download/pkgs/index"

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// lookup reads CFM_<NAME>, falling back to the legacy PISTON_<NAME>.
func lookup(name string) (string, bool) {
	up := strings.ToUpper(name)
	if v, ok := os.LookupEnv("CFM_" + up); ok {
		return v, true
	}
	return os.LookupEnv("PISTON_" + up)
}

func envString(name, def string) string {
	if v, ok := lookup(name); ok {
		return v
	}
	return def
}

func envInt(name string, def int) (int, error) {
	v, ok := lookup(name)
	if !ok {
		return def, nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return 0, fmt.Errorf("%s: %q is not a number", name, v)
	}
	return n, nil
}
