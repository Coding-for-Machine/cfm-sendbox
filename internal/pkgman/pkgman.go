// Package pkgman downloads, installs and removes language packages (ppman).
package pkgman

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Masterminds/semver/v3"

	"github.com/Coding-for-Machine/cfm-sendbox/internal/config"
	"github.com/Coding-for-Machine/cfm-sendbox/internal/globals"
	"github.com/Coding-for-Machine/cfm-sendbox/internal/runtime"
)

// Manager talks to the package repository and manages installed packages.
type Manager struct {
	cfg *config.Config
	reg *runtime.Registry
	// mu serialises install/uninstall operations.
	mu sync.Mutex
}

// New creates a package manager.
func New(cfg *config.Config, reg *runtime.Registry) *Manager {
	return &Manager{cfg: cfg, reg: reg}
}

// Package is an entry of the repository index.
type Package struct {
	Language string
	Version  *semver.Version
	Checksum string
	Download string
	m        *Manager
}

// Result is returned after installing or uninstalling.
type Result struct {
	Language string `json:"language"`
	Version  string `json:"version"`
}

// InstallPath is where the package lives on disk.
func (p *Package) InstallPath() string {
	return filepath.Join(p.m.cfg.DataDirectory, globals.PackagesDir, p.Language, p.Version.Original())
}

// Installed reports whether the package finished installing.
func (p *Package) Installed() bool {
	_, err := os.Stat(filepath.Join(p.InstallPath(), globals.PkgInstalledFile))
	return err == nil
}

// List fetches and merges the configured repository indexes. An index that
// cannot be fetched is skipped (with a warning); it is an error only when no
// index could be read at all.
func (m *Manager) List() ([]*Package, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	urls := m.cfg.RepoURLs
	if len(urls) == 0 {
		urls = []string{m.cfg.RepoURL}
	}

	var (
		pkgs    []*Package
		seen    = map[string]bool{}
		ok      bool
		lastErr error
	)
	for _, u := range urls {
		body, err := fetchIndex(client, u)
		if err != nil {
			slog.Warn("package index unavailable", "url", u, "error", err)
			lastErr = err
			continue
		}
		ok = true
		for _, p := range m.parseIndex(body) {
			key := p.Language + "\x00" + p.Version.Original()
			if seen[key] {
				continue
			}
			seen[key] = true
			pkgs = append(pkgs, p)
		}
	}
	if !ok {
		return nil, lastErr
	}
	return pkgs, nil
}

func fetchIndex(client *http.Client, url string) ([]byte, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch package index: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch package index: unexpected status %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read package index: %w", err)
	}
	return body, nil
}

func (m *Manager) parseIndex(body []byte) []*Package {
	var pkgs []*Package
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ",", 4)
		if len(parts) < 4 {
			slog.Warn("skipping malformed package index line", "line", line)
			continue
		}
		v, err := semver.NewVersion(parts[1])
		if err != nil {
			slog.Warn("skipping package with bad version", "line", line)
			continue
		}
		pkgs = append(pkgs, &Package{
			Language: parts[0], Version: v, Checksum: parts[2], Download: parts[3], m: m,
		})
	}
	return pkgs
}

// Get returns the newest package matching language and version selector, or
// nil when there is none.
func (m *Manager) Get(language, version string) (*Package, error) {
	constraint, err := semver.NewConstraint(version)
	if err != nil {
		return nil, nil // an invalid selector matches nothing
	}
	pkgs, err := m.List()
	if err != nil {
		return nil, err
	}
	var candidates []*Package
	for _, p := range pkgs {
		if p.Language == language && constraint.Check(p.Version) {
			candidates = append(candidates, p)
		}
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Version.GreaterThan(candidates[j].Version)
	})
	return candidates[0], nil
}

// LoadInstalled registers every installed package found in the data directory.
func (m *Manager) LoadInstalled() error {
	root := filepath.Join(m.cfg.DataDirectory, globals.PackagesDir)
	langs, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, lang := range langs {
		if !lang.IsDir() {
			continue
		}
		versions, err := os.ReadDir(filepath.Join(root, lang.Name()))
		if err != nil {
			slog.Error("cannot read package directory", "language", lang.Name(), "error", err)
			continue
		}
		for _, ver := range versions {
			dir := filepath.Join(root, lang.Name(), ver.Name())
			if _, err := os.Stat(filepath.Join(dir, globals.PkgInstalledFile)); err != nil {
				continue
			}
			if err := m.reg.LoadPackage(dir); err != nil {
				slog.Error("failed to load package", "dir", dir, "error", err)
			}
		}
	}
	return nil
}

// Install downloads, verifies and registers the package.
func (p *Package) Install() (*Result, error) {
	p.m.mu.Lock()
	defer p.m.mu.Unlock()

	if p.Installed() {
		return nil, errors.New("Already installed")
	}
	name := p.Language + "-" + p.Version.Original()
	slog.Info("installing package", "package", name)

	path := p.InstallPath()
	if _, err := os.Stat(path); err == nil {
		slog.Warn("package has residual files, removing them", "package", name)
		if err := os.RemoveAll(path); err != nil {
			return nil, err
		}
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return nil, err
	}

	// Download while hashing.
	archive := filepath.Join(path, "pkg.tar.gz")
	sum, err := download(p.Download, archive)
	if err != nil {
		return nil, err
	}
	if sum != p.Checksum {
		return nil, fmt.Errorf("Checksum miss-match want: %s got: %s", p.Checksum, sum)
	}

	// Extract.
	tar := exec.Command("tar", "xzf", archive)
	tar.Dir = path
	if out, err := tar.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("extract %s: %v: %s", archive, err, bytes.TrimSpace(out))
	}

	// Cache the package environment: source `environment` in a clean shell.
	envCmd := exec.Command("env", "-i", "bash", "-c",
		`cd "$1" && source environment && env`, "bash", path)
	envOut, err := envCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("read package environment: %w", err)
	}
	var kept []string
	for _, line := range strings.Split(string(envOut), "\n") {
		switch strings.SplitN(line, "=", 2)[0] {
		case "PWD", "OLDPWD", "_", "SHLVL":
			continue
		}
		kept = append(kept, line)
	}
	if err := os.WriteFile(filepath.Join(path, ".env"), []byte(strings.Join(kept, "\n")), 0o644); err != nil {
		return nil, err
	}

	// Make sure the files belong to the user running the API.
	uid, gid := os.Getuid(), os.Getgid()
	_ = filepath.WalkDir(path, func(p string, _ fs.DirEntry, err error) error {
		if err == nil {
			_ = os.Lchown(p, uid, gid)
		}
		return nil
	})

	// Register only once everything the runtime needs exists on disk.
	if err := p.m.reg.LoadPackage(path); err != nil {
		return nil, err
	}

	if err := os.WriteFile(filepath.Join(path, globals.PkgInstalledFile),
		[]byte(fmt.Sprint(time.Now().UnixMilli())), 0o644); err != nil {
		return nil, err
	}

	slog.Info("package installed", "package", name)
	return &Result{Language: p.Language, Version: p.Version.Original()}, nil
}

// Uninstall unregisters the package and removes its files.
func (p *Package) Uninstall() (*Result, error) {
	p.m.mu.Lock()
	defer p.m.mu.Unlock()

	name := p.Language + "-" + p.Version.Original()
	slog.Info("uninstalling package", "package", name)

	if p.m.reg.ByNameAndVersion(p.Language, p.Version.Original()) == nil {
		return nil, fmt.Errorf("%s is not installed", name)
	}
	p.m.reg.UnregisterPackage(p.InstallPath())
	if err := os.RemoveAll(p.InstallPath()); err != nil {
		return nil, err
	}

	slog.Info("package uninstalled", "package", name)
	return &Result{Language: p.Language, Version: p.Version.Original()}, nil
}

func download(url, dest string) (string, error) {
	client := &http.Client{Timeout: 15 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: unexpected status %s", url, resp.Status)
	}

	f, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(f, h), resp.Body); err != nil {
		return "", fmt.Errorf("download %s: %w", url, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
