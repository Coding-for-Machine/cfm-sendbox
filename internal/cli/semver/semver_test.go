package semver

import "testing"

func TestSatisfies(t *testing.T) {
	cases := []struct {
		version, rng string
		want         bool
	}{
		{"3.12.0", "*", true},
		{"3.12.0", "", true},
		{"3.12.0", "x", true},
		{"3.12.0", "3.*", true},
		{"3.12.0", "3.x", true},
		{"3.12.0", "3", true},
		{"3.12.0", "3.12", true},
		{"3.12.0", "3.11", false},
		{"2.7.18", "3.*", false},
		{"3.12.0", "3.12.0", true},
		{"3.12.0", "=3.12.0", true},
		{"3.12.0", "3.12.1", false},
		{"3.12.0", "^3.10.0", true},
		{"4.0.0", "^3.10.0", false},
		{"0.2.5", "^0.2.3", true},
		{"0.3.0", "^0.2.3", false},
		{"0.0.4", "^0.0.3", false},
		{"1.2.9", "~1.2.3", true},
		{"1.3.0", "~1.2.3", false},
		{"1.9.0", "~1", true},
		{"2.0.0", "~1", false},
		{"1.5.0", ">=1.0 <2", true},
		{"2.0.0", ">=1.0 <2", false},
		{"1.5.0", ">= 1.0 < 2", true},
		{"1.5.0", ">1", false},
		{"2.0.0", ">1", true},
		{"1.1.0", ">1.0.0", true},
		{"1.2.3", "<=1.2", true},
		{"1.3.0", "<=1.2", false},
		{"3.1.0", "1.x || 3.x", true},
		{"2.1.0", "1.x || 3.x", false},
		{"1.5.0", "1.2.3 - 2.0.0", true},
		{"2.0.1", "1.2.3 - 2.0.0", false},
		{"2.5.0", "1.2.3 - 2", true},
		// pre-releases only match when asked for explicitly
		{"3.10.0-alpha.7", "*", false},
		{"3.10.0-alpha.7", "3.*", false},
		{"3.10.0-alpha.7", "3.10.0-alpha.7", true},
		{"3.10.0-alpha.7", ">=3.10.0-alpha.1", true},
		{"3.10.0-alpha.7", ">=3.9.0", false},
		// garbage never matches
		{"not-a-version", "*", false},
		{"1.0.0", "garbage", false},
	}
	for _, c := range cases {
		if got := Satisfies(c.version, c.rng); got != c.want {
			t.Errorf("Satisfies(%q, %q) = %v, want %v", c.version, c.rng, got, c.want)
		}
	}
}

func TestCompare(t *testing.T) {
	order := []string{"1.0.0-alpha", "1.0.0-alpha.1", "1.0.0-alpha.beta", "1.0.0-beta", "1.0.0-beta.2",
		"1.0.0-beta.11", "1.0.0-rc.1", "1.0.0", "1.0.1", "1.10.0", "2.0.0", "10.0.0"}
	for i := 0; i < len(order)-1; i++ {
		a, err := Parse(order[i])
		if err != nil {
			t.Fatal(err)
		}
		b, err := Parse(order[i+1])
		if err != nil {
			t.Fatal(err)
		}
		if Compare(a, b) != -1 || Compare(b, a) != 1 || Compare(a, a) != 0 {
			t.Errorf("ordering broken between %s and %s", order[i], order[i+1])
		}
	}
}

func TestParse(t *testing.T) {
	good := map[string]string{
		"1.2.3": "1.2.3", "v1.2.3": "1.2.3", "1.2": "1.2.0", "1": "1.0.0",
		"3.10.0-alpha.7": "3.10.0-alpha.7", "1.2.3+build.5": "1.2.3", "6.100.0": "6.100.0",
	}
	for in, want := range good {
		v, err := Parse(in)
		if err != nil || v.String() != want {
			t.Errorf("Parse(%q) = %v, %v; want %s", in, v, err, want)
		}
	}
	for _, in := range []string{"", "a.b.c", "1.2.3.4", "1.2.3-", "-1"} {
		if _, err := Parse(in); err == nil {
			t.Errorf("Parse(%q) should fail", in)
		}
	}
}

func TestSortDesc(t *testing.T) {
	var vs []Version
	for _, s := range []string{"3.9.1", "3.12.0", "3.10.0", "3.10.0-alpha.7", "2.7.18"} {
		v, _ := Parse(s)
		vs = append(vs, v)
	}
	SortDesc(vs)
	want := []string{"3.12.0", "3.10.0", "3.10.0-alpha.7", "3.9.1", "2.7.18"}
	for i, w := range want {
		if vs[i].String() != w {
			t.Fatalf("position %d: got %s want %s", i, vs[i], w)
		}
	}
}
