package commands

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/Coding-for-Machine/cfm-sendbox/internal/cli/client"
	"github.com/Coding-for-Machine/cfm-sendbox/internal/cli/semver"
)

// splitPackage turns "python=3.12.0" into a package reference; the version
// defaults to "*".
func splitPackage(s string) client.PackageRef {
	lang, ver, _ := strings.Cut(s, "=")
	if ver == "" {
		ver = "*"
	}
	return client.PackageRef{Language: lang, Version: ver}
}

func ppmanChange(args []string, env Env, url string, install bool) int {
	name := "uninstall"
	if install {
		name = "install"
	}
	fs := newFlagSet("ppman "+name, env)
	fs.StringVar(&url, "u", url, "")
	fs.StringVar(&url, "cfm-url", url, "")
	pkgs, _, err := parseInterleaved(fs, args, -1)
	if err != nil {
		return parseCode(err)
	}
	if len(pkgs) == 0 {
		fmt.Fprintf(env.Err, "Usage: cfm ppman %s <package[=version]...>\n", name)
		return 2
	}

	p := env.paint()
	c := client.New(url)
	failed := false
	for _, pkg := range pkgs {
		ref := splitPackage(pkg)
		var (
			res client.PackageRef
			err error
		)
		if install {
			res, err = c.Install(ref)
		} else {
			res, err = c.Uninstall(ref)
		}
		if err != nil {
			fmt.Fprintf(env.Err, "%s %s=%s: %v\n", p.BoldRed("✗"), ref.Language, ref.Version, err)
			failed = true
			continue
		}
		verb := "Installation"
		if !install {
			verb = "Uninstallation"
		}
		fmt.Fprintf(env.Out, "%s %s succeeded: %s %s\n", p.BoldGreen("✓"), verb, res.Language, res.Version)
	}
	if failed {
		return 1
	}
	return 0
}

func ppmanList(args []string, env Env, url string) int {
	fs := newFlagSet("ppman list", env)
	fs.StringVar(&url, "u", url, "")
	fs.StringVar(&url, "cfm-url", url, "")
	if err := fs.Parse(args); err != nil {
		return parseCode(err)
	}
	pkgs, err := client.New(url).Packages()
	if err != nil {
		fmt.Fprintln(env.Err, err)
		return 1
	}
	p := env.paint()
	for _, pkg := range pkgs {
		if env.Color {
			bullet := p.Red("•")
			if pkg.Installed {
				bullet = p.Green("•")
			}
			fmt.Fprintf(env.Out, "%s %s %s\n", bullet, pkg.Language, pkg.LanguageVersion)
		} else {
			mark := ""
			if pkg.Installed {
				mark = " (INSTALLED)"
			}
			fmt.Fprintf(env.Out, "%s %s%s\n", pkg.Language, pkg.LanguageVersion, mark)
		}
	}
	return 0
}

// specRule is one non-empty line of a spec file.
type specRule struct {
	comment bool
	negate  bool
	pkg     string // glob matched against the package name
	ver     string // semver range
}

func (r specRule) matches(p client.Package) bool {
	ok, err := path.Match(r.pkg, p.Language)
	return err == nil && ok && semver.Satisfies(p.LanguageVersion, r.ver)
}

func parseSpec(lines []string) []specRule {
	var rules []specRule
	for _, line := range lines {
		l := strings.TrimSpace(line)
		if l == "" {
			continue
		}
		if strings.HasPrefix(l, "#") {
			rules = append(rules, specRule{comment: true})
			continue
		}
		r := specRule{}
		if strings.HasPrefix(l, "!") {
			r.negate = true
			l = strings.TrimSpace(l[1:])
		}
		fields := strings.Fields(l)
		if len(fields) == 0 {
			continue
		}
		r.pkg, r.ver = fields[0], "*"
		if len(fields) > 1 {
			r.ver = fields[1]
		}
		rules = append(rules, r)
	}
	return rules
}

type specOp struct {
	install bool
	pkg     client.Package
}

// planSpec decides which packages have to be installed or uninstalled.
func planSpec(rules []specRule, all []client.Package) []specOp {
	var ensure []client.Package
	has := func(list []client.Package, p client.Package) bool {
		for _, x := range list {
			if x.Language == p.Language && x.LanguageVersion == p.LanguageVersion {
				return true
			}
		}
		return false
	}

	for _, rule := range rules {
		if rule.comment {
			continue
		}
		if rule.negate {
			kept := ensure[:0:0]
			for _, p := range ensure {
				if !rule.matches(p) {
					kept = append(kept, p)
				}
			}
			ensure = kept
			continue
		}

		// Newest matching version of every language.
		best := map[string]semver.Version{}
		for _, p := range all {
			if !rule.matches(p) {
				continue
			}
			v, err := semver.Parse(p.LanguageVersion)
			if err != nil {
				continue
			}
			if cur, ok := best[p.Language]; !ok || semver.Compare(v, cur) > 0 {
				best[p.Language] = v
			}
		}
		for _, p := range all {
			v, err := semver.Parse(p.LanguageVersion)
			if !rule.matches(p) || err != nil {
				continue
			}
			if b := best[p.Language]; semver.Compare(v, b) == 0 && !has(ensure, p) {
				ensure = append(ensure, p)
			}
		}
	}

	var ops []specOp
	for _, p := range ensure {
		if !p.Installed {
			ops = append(ops, specOp{install: true, pkg: p})
		}
	}
	for _, p := range all {
		if p.Installed && !has(ensure, p) {
			ops = append(ops, specOp{install: false, pkg: p})
		}
	}
	return ops
}

func readLines(file string) ([]string, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines, sc.Err()
}

func ppmanSpec(args []string, env Env, url string) int {
	fs := newFlagSet("ppman spec", env)
	fs.StringVar(&url, "u", url, "")
	fs.StringVar(&url, "cfm-url", url, "")
	pos, _, err := parseInterleaved(fs, args, 1)
	if err != nil {
		return parseCode(err)
	}
	if len(pos) != 1 {
		fmt.Fprintln(env.Err, "Usage: cfm ppman spec <specfile>")
		return 2
	}

	lines, err := readLines(pos[0])
	if err != nil {
		fmt.Fprintln(env.Err, err)
		return 1
	}
	c := client.New(url)
	all, err := c.Packages()
	if err != nil {
		fmt.Fprintln(env.Err, err)
		return 1
	}

	ops := planSpec(parseSpec(lines), all)
	p := env.paint()

	fmt.Fprintln(env.Out, p.BoldYellow("Actions"))
	for _, op := range ops {
		verb := p.Red("Uninstall")
		if op.install {
			verb = p.Green("Install")
		}
		fmt.Fprintf(env.Out, "%s %s %s\n", verb, op.pkg.Language, op.pkg.LanguageVersion)
	}
	if len(ops) == 0 {
		fmt.Fprintln(env.Out, p.Gray("None"))
	}

	failed := false
	for _, op := range ops {
		ref := client.PackageRef{Language: op.pkg.Language, Version: op.pkg.LanguageVersion}
		var err error
		if op.install {
			_, err = c.Install(ref)
		} else {
			_, err = c.Uninstall(ref)
		}
		switch {
		case err != nil && op.install:
			fmt.Fprintf(env.Out, "%s %s %s: %v\n", p.BoldRed("Failed to install"), ref.Language, ref.Version, err)
			failed = true
		case err != nil:
			fmt.Fprintf(env.Out, "%s %s %s: %v\n", p.BoldRed("Failed to uninstall"), ref.Language, ref.Version, err)
			failed = true
		case op.install:
			fmt.Fprintf(env.Out, "%s %s %s\n", p.BoldGreen("Installed"), ref.Language, ref.Version)
		default:
			fmt.Fprintf(env.Out, "%s %s %s\n", p.BoldGreen("Uninstalled"), ref.Language, ref.Version)
		}
	}
	if failed {
		return 1
	}
	return 0
}
