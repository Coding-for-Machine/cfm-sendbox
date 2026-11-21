// Package semver is a small npm-style semantic version and range matcher.
//
// It supports the range syntax used in Piston spec files and package
// selectors: "*", "3.x", "3", "3.10", "^1.2.3", "~1.2", ">=1.0 <2", "1.x || 3.x"
// and hyphen ranges. As in npm, pre-release versions only satisfy a range when
// one of its comparators names a pre-release of the same major.minor.patch.
package semver

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Version is a parsed semantic version.
type Version struct {
	Major, Minor, Patch uint64
	Pre                 []string
}

// Parse parses "1.2.3", "v1.2", "1.2.3-alpha.7" or "1.2.3+build". Missing minor
// and patch numbers default to zero.
func Parse(s string) (Version, error) {
	orig := s
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(strings.TrimPrefix(s, "="), "v")
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	var v Version
	if i := strings.IndexByte(s, '-'); i >= 0 {
		v.Pre = strings.Split(s[i+1:], ".")
		s = s[:i]
		for _, id := range v.Pre {
			if id == "" {
				return Version{}, fmt.Errorf("invalid version %q", orig)
			}
		}
	}
	parts := strings.Split(s, ".")
	if len(parts) == 0 || len(parts) > 3 {
		return Version{}, fmt.Errorf("invalid version %q", orig)
	}
	nums := [3]uint64{}
	for i, p := range parts {
		n, err := strconv.ParseUint(p, 10, 64)
		if err != nil {
			return Version{}, fmt.Errorf("invalid version %q", orig)
		}
		nums[i] = n
	}
	v.Major, v.Minor, v.Patch = nums[0], nums[1], nums[2]
	return v, nil
}

func (v Version) String() string {
	s := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if len(v.Pre) > 0 {
		s += "-" + strings.Join(v.Pre, ".")
	}
	return s
}

func cmpUint(a, b uint64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func isNumeric(s string) bool {
	_, err := strconv.ParseUint(s, 10, 64)
	return err == nil
}

// Compare returns -1, 0 or 1.
func Compare(a, b Version) int {
	if c := cmpUint(a.Major, b.Major); c != 0 {
		return c
	}
	if c := cmpUint(a.Minor, b.Minor); c != 0 {
		return c
	}
	if c := cmpUint(a.Patch, b.Patch); c != 0 {
		return c
	}
	switch {
	case len(a.Pre) == 0 && len(b.Pre) == 0:
		return 0
	case len(a.Pre) == 0:
		return 1
	case len(b.Pre) == 0:
		return -1
	}
	for i := 0; i < len(a.Pre) && i < len(b.Pre); i++ {
		x, y := a.Pre[i], b.Pre[i]
		xn, yn := isNumeric(x), isNumeric(y)
		switch {
		case xn && yn:
			xv, _ := strconv.ParseUint(x, 10, 64)
			yv, _ := strconv.ParseUint(y, 10, 64)
			if c := cmpUint(xv, yv); c != 0 {
				return c
			}
		case xn:
			return -1
		case yn:
			return 1
		default:
			if c := strings.Compare(x, y); c != 0 {
				return c
			}
		}
	}
	return cmpUint(uint64(len(a.Pre)), uint64(len(b.Pre)))
}

// SortDesc sorts versions, newest first.
func SortDesc(vs []Version) {
	sort.SliceStable(vs, func(i, j int) bool { return Compare(vs[i], vs[j]) > 0 })
}

type comparator struct {
	op string // ">=", ">", "<", "<=", "="
	v  Version
}

func (c comparator) test(v Version) bool {
	r := Compare(v, c.v)
	switch c.op {
	case ">=":
		return r >= 0
	case ">":
		return r > 0
	case "<":
		return r < 0
	case "<=":
		return r <= 0
	default:
		return r == 0
	}
}

// Range is a parsed version range.
type Range struct {
	sets [][]comparator
}

// ParseRange parses an npm-style range. An empty string means "*".
func ParseRange(s string) (*Range, error) {
	s = strings.TrimSpace(s)
	r := &Range{}
	for _, part := range strings.Split(s, "||") {
		set, err := parseSet(strings.TrimSpace(part))
		if err != nil {
			return nil, err
		}
		r.sets = append(r.sets, set)
	}
	return r, nil
}

// Satisfies reports whether v is inside the range.
func (r *Range) Satisfies(v Version) bool {
	for _, set := range r.sets {
		if setSatisfies(set, v) {
			return true
		}
	}
	return false
}

// Satisfies parses version and constraint strings and matches them. Strings
// that fail to parse never match.
func Satisfies(version, constraint string) bool {
	v, err := Parse(version)
	if err != nil {
		return false
	}
	r, err := ParseRange(constraint)
	if err != nil {
		return false
	}
	return r.Satisfies(v)
}

func setSatisfies(set []comparator, v Version) bool {
	for _, c := range set {
		if !c.test(v) {
			return false
		}
	}
	if len(v.Pre) == 0 {
		return true
	}
	// Pre-releases only match when a comparator targets the same tuple.
	for _, c := range set {
		if len(c.v.Pre) > 0 && c.v.Major == v.Major && c.v.Minor == v.Minor && c.v.Patch == v.Patch {
			return true
		}
	}
	return false
}

func isOperator(tok string) bool {
	switch tok {
	case ">", ">=", "<", "<=", "=", "^", "~", "~>":
		return true
	}
	return false
}

func parseSet(s string) ([]comparator, error) {
	toks := strings.Fields(s)
	if len(toks) == 0 {
		return nil, nil // matches everything
	}

	// Hyphen range: "1.2.3 - 2.3.4".
	if len(toks) == 3 && toks[1] == "-" {
		lo, err := parsePartial(toks[0])
		if err != nil {
			return nil, err
		}
		hi, err := parsePartial(toks[2])
		if err != nil {
			return nil, err
		}
		set := []comparator{{">=", lo.floor()}}
		if hi.n == 3 {
			set = append(set, comparator{"<=", hi.floor()})
		} else if up, ok := hi.nextUp(); ok {
			set = append(set, comparator{"<", up})
		}
		return set, nil
	}

	// Join detached operators: ">= 1.2.3" -> ">=1.2.3".
	var merged []string
	for i := 0; i < len(toks); i++ {
		if isOperator(toks[i]) && i+1 < len(toks) {
			merged = append(merged, toks[i]+toks[i+1])
			i++
			continue
		}
		merged = append(merged, toks[i])
	}

	var set []comparator
	for _, tok := range merged {
		cs, err := parseComparator(tok)
		if err != nil {
			return nil, err
		}
		set = append(set, cs...)
	}
	return set, nil
}

// partial is a possibly incomplete version such as "3", "3.10" or "3.x".
type partial struct {
	v Version
	n int // number of specified numeric components (0 = wildcard)
}

func (p partial) floor() Version { return p.v }

// nextUp returns the exclusive upper bound that covers every version matching
// the partial, e.g. 3 -> 4.0.0, 3.10 -> 3.11.0. ok is false for "*".
func (p partial) nextUp() (Version, bool) {
	switch p.n {
	case 0:
		return Version{}, false
	case 1:
		return Version{Major: p.v.Major + 1}, true
	case 2:
		return Version{Major: p.v.Major, Minor: p.v.Minor + 1}, true
	}
	return Version{Major: p.v.Major, Minor: p.v.Minor, Patch: p.v.Patch + 1}, true
}

func isWildcard(s string) bool { return s == "x" || s == "X" || s == "*" || s == "" }

func parsePartial(s string) (partial, error) {
	s = strings.TrimPrefix(strings.TrimPrefix(s, "="), "v")
	var p partial
	if isWildcard(s) {
		return p, nil
	}
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	pre := ""
	if i := strings.IndexByte(s, '-'); i >= 0 {
		pre = s[i+1:]
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) > 3 {
		return p, fmt.Errorf("invalid version range %q", s)
	}
	nums := [3]uint64{}
	for i, part := range parts {
		if isWildcard(part) {
			break
		}
		n, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return p, fmt.Errorf("invalid version range %q", s)
		}
		nums[i] = n
		p.n = i + 1
	}
	p.v = Version{Major: nums[0], Minor: nums[1], Patch: nums[2]}
	if pre != "" && p.n == 3 {
		p.v.Pre = strings.Split(pre, ".")
	}
	return p, nil
}

func parseComparator(tok string) ([]comparator, error) {
	op := ""
	for _, o := range []string{">=", "<=", "~>", ">", "<", "=", "^", "~"} {
		if strings.HasPrefix(tok, o) {
			op = o
			tok = tok[len(o):]
			break
		}
	}
	p, err := parsePartial(tok)
	if err != nil {
		return nil, err
	}
	lo := comparator{">=", p.v}
	up, hasUp := p.nextUp()

	switch op {
	case "", "=":
		if p.n == 0 {
			return nil, nil
		}
		if p.n == 3 {
			return []comparator{{"=", p.v}}, nil
		}
		return []comparator{lo, {"<", up}}, nil
	case "^":
		if p.n == 0 {
			return nil, nil
		}
		var hi Version
		switch {
		case p.v.Major > 0 || p.n == 1:
			hi = Version{Major: p.v.Major + 1}
		case p.v.Minor > 0 || p.n == 2:
			hi = Version{Minor: p.v.Minor + 1}
		default:
			hi = Version{Patch: p.v.Patch + 1}
		}
		return []comparator{lo, {"<", hi}}, nil
	case "~", "~>":
		if p.n == 0 {
			return nil, nil
		}
		if p.n == 1 {
			return []comparator{lo, {"<", Version{Major: p.v.Major + 1}}}, nil
		}
		return []comparator{lo, {"<", Version{Major: p.v.Major, Minor: p.v.Minor + 1}}}, nil
	case ">":
		if p.n == 0 {
			return []comparator{{"<", Version{}}}, nil // nothing is greater than *
		}
		if p.n == 3 {
			return []comparator{{">", p.v}}, nil
		}
		return []comparator{{">=", up}}, nil
	case ">=":
		return []comparator{lo}, nil
	case "<":
		if p.n == 0 {
			return []comparator{{"<", Version{}}}, nil
		}
		return []comparator{{"<", p.v}}, nil
	case "<=":
		if p.n == 0 || !hasUp {
			return nil, nil
		}
		if p.n == 3 {
			return []comparator{{"<=", p.v}}, nil
		}
		return []comparator{{"<", up}}, nil
	}
	return nil, fmt.Errorf("invalid comparator %q", tok)
}
