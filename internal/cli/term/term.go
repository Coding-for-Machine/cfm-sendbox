// Package term has tiny helpers for ANSI colors and TTY detection.
package term

import (
	"os"
	"strings"
)

// IsTTY reports whether f is an interactive terminal.
func IsTTY(f *os.File) bool {
	if f == nil {
		return false
	}
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

// ColorEnabled follows the NO_COLOR / FORCE_COLOR conventions.
func ColorEnabled(f *os.File) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("FORCE_COLOR") != "" {
		return true
	}
	return IsTTY(f)
}

// Painter wraps text in ANSI styles when enabled.
type Painter struct{ On bool }

func (p Painter) wrap(code, s string) string {
	if !p.On {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func (p Painter) Bold(s string) string       { return p.wrap("1", s) }
func (p Painter) Red(s string) string        { return p.wrap("31", s) }
func (p Painter) Green(s string) string      { return p.wrap("32", s) }
func (p Painter) Yellow(s string) string     { return p.wrap("33", s) }
func (p Painter) Gray(s string) string       { return p.wrap("90", s) }
func (p Painter) BoldRed(s string) string    { return p.wrap("1;31", s) }
func (p Painter) BoldGreen(s string) string  { return p.wrap("1;32", s) }
func (p Painter) BoldYellow(s string) string { return p.wrap("1;33", s) }

// Indent prefixes every line after the first with four spaces, matching the
// output layout of the original Piston CLI.
func Indent(s string) string { return strings.ReplaceAll(s, "\n", "\n    ") }
