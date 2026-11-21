// Package runtime keeps the registry of installed language runtimes.
package runtime

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/Masterminds/semver/v3"

	"github.com/Coding-for-Machine/cfm-sendbox/internal/config"
)

// Stage holds a value for the compile and run stages.
type Stage struct {
	Compile int
	Run     int
}

// Runtime is one installed language/version.
type Runtime struct {
	Language string
	Version  *semver.Version
	Aliases  []string
	PkgDir   string
	// Runtime is the package name when a package provides several languages.
	Runtime string

	Timeouts     Stage // milliseconds, wall time
	CPUTimes     Stage // milliseconds
	MemoryLimits Stage // bytes, -1 = unlimited

	MaxProcessCount int
	MaxOpenFiles    int
	MaxFileSize     int
	OutputMaxSize   int

	once     sync.Once
	compiled bool
	envVars  []string
	envErr   error
}

func (r *Runtime) load() {
	r.once.Do(func() {
		_, err := os.Stat(filepath.Join(r.PkgDir, "compile"))
		r.compiled = err == nil

		data, err := os.ReadFile(filepath.Join(r.PkgDir, ".env"))
		if err != nil {
			r.envErr = err
			return
		}
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			if line != "" {
				r.envVars = append(r.envVars, line)
			}
		}
	})
}

// Compiled reports whether the runtime has a compile stage.
func (r *Runtime) Compiled() bool {
	r.load()
	return r.compiled
}

// EnvVars returns the cached KEY=VALUE environment of the package.
func (r *Runtime) EnvVars() ([]string, error) {
	r.load()
	return r.envVars, r.envErr
}

func (r *Runtime) String() string {
	return r.Language + "-" + r.Version.Original()
}

// Registry is a concurrency-safe list of runtimes.
type Registry struct {
	mu       sync.RWMutex
	runtimes []*Runtime
	cfg      *config.Config
	platform string
}

// NewRegistry creates an empty registry.
func NewRegistry(cfg *config.Config, platform string) *Registry {
	return &Registry{cfg: cfg, platform: platform}
}

type pkgInfo struct {
	Language       string             `json:"language"`
	Version        string             `json:"version"`
	BuildPlatform  string             `json:"build_platform"`
	Aliases        []string           `json:"aliases"`
	LimitOverrides map[string]float64 `json:"limit_overrides"`
	Provides       []struct {
		Language       string             `json:"language"`
		Aliases        []string           `json:"aliases"`
		LimitOverrides map[string]float64 `json:"limit_overrides"`
	} `json:"provides"`
}

func toInts(m map[string]float64) map[string]int {
	out := make(map[string]int, len(m))
	for k, v := range m {
		out[k] = int(v)
	}
	return out
}

func (reg *Registry) newRuntime(language string, aliases []string, version *semver.Version,
	pkgDir, pkgName string, overrides map[string]int) *Runtime {

	lim := func(key string) int { return reg.cfg.Limit(language, key, overrides) }
	if aliases == nil {
		aliases = []string{}
	}
	return &Runtime{
		Language: language,
		Version:  version,
		Aliases:  aliases,
		PkgDir:   pkgDir,
		Runtime:  pkgName,
		Timeouts: Stage{Compile: lim(config.CompileTimeout), Run: lim(config.RunTimeout)},
		CPUTimes: Stage{Compile: lim(config.CompileCPUTime), Run: lim(config.RunCPUTime)},
		MemoryLimits: Stage{
			Compile: lim(config.CompileMemoryLimit),
			Run:     lim(config.RunMemoryLimit),
		},
		MaxProcessCount: lim(config.MaxProcessCount),
		MaxOpenFiles:    lim(config.MaxOpenFiles),
		MaxFileSize:     lim(config.MaxFileSize),
		OutputMaxSize:   lim(config.OutputMaxSize),
	}
}

// LoadPackage reads pkg-info.json from pkgDir and registers its runtimes.
func (reg *Registry) LoadPackage(pkgDir string) error {
	data, err := os.ReadFile(filepath.Join(pkgDir, "pkg-info.json"))
	if err != nil {
		return err
	}
	var info pkgInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return fmt.Errorf("parse %s/pkg-info.json: %w", pkgDir, err)
	}
	version, err := semver.NewVersion(info.Version)
	if err != nil {
		return fmt.Errorf("package %s: bad version %q: %w", info.Language, info.Version, err)
	}

	if info.BuildPlatform != reg.platform {
		slog.Warn("package built for a different platform",
			"package", info.Language+"-"+info.Version,
			"built_for", info.BuildPlatform, "platform", reg.platform)
	}

	var added []*Runtime
	if len(info.Provides) > 0 {
		for _, p := range info.Provides {
			added = append(added, reg.newRuntime(p.Language, p.Aliases, version,
				pkgDir, info.Language, toInts(p.LimitOverrides)))
		}
	} else {
		added = append(added, reg.newRuntime(info.Language, info.Aliases, version,
			pkgDir, "", toInts(info.LimitOverrides)))
	}

	reg.mu.Lock()
	reg.runtimes = append(reg.runtimes, added...)
	reg.mu.Unlock()

	slog.Debug("package loaded", "package", info.Language+"-"+info.Version)
	return nil
}

// All returns a snapshot of the registered runtimes.
func (reg *Registry) All() []*Runtime {
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	return append([]*Runtime(nil), reg.runtimes...)
}

func satisfies(v *semver.Version, selector string) bool {
	c, err := semver.NewConstraint(selector)
	return err == nil && c.Check(v)
}

// MatchingLanguageVersion returns runtimes whose language or alias matches and
// whose version satisfies the selector.
func (reg *Registry) MatchingLanguageVersion(lang, ver string) []*Runtime {
	var out []*Runtime
	for _, rt := range reg.All() {
		if rt.Language != lang && !contains(rt.Aliases, lang) {
			continue
		}
		if satisfies(rt.Version, ver) {
			out = append(out, rt)
		}
	}
	return out
}

// LatestMatchingLanguageVersion returns the highest matching version or nil.
func (reg *Registry) LatestMatchingLanguageVersion(lang, ver string) *Runtime {
	matches := reg.MatchingLanguageVersion(lang, ver)
	if len(matches) == 0 {
		return nil
	}
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Version.GreaterThan(matches[j].Version)
	})
	return matches[0]
}

// ByNameAndVersion finds a runtime by package name (or language for
// single-language packages) and a version selector.
func (reg *Registry) ByNameAndVersion(name, ver string) *Runtime {
	for _, rt := range reg.All() {
		if (rt.Runtime == name || (rt.Runtime == "" && rt.Language == name)) &&
			satisfies(rt.Version, ver) {
			return rt
		}
	}
	return nil
}

// UnregisterPackage removes every runtime that comes from pkgDir.
func (reg *Registry) UnregisterPackage(pkgDir string) {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	kept := reg.runtimes[:0]
	for _, rt := range reg.runtimes {
		if rt.PkgDir != pkgDir {
			kept = append(kept, rt)
		}
	}
	reg.runtimes = kept
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
