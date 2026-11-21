package config

import "testing"

func TestParseOverrides(t *testing.T) {
	got, err := ParseOverrides(`{"java":{"run_timeout":5000,"max_process_count":128}}`)
	if err != nil {
		t.Fatal(err)
	}
	if got["java"][RunTimeout] != 5000 || got["java"][MaxProcessCount] != 128 {
		t.Fatalf("unexpected overrides: %v", got)
	}
	if _, err := ParseOverrides(`{"java":{"bogus":1}}`); err == nil {
		t.Fatal("expected error for unknown option")
	}
	if _, err := ParseOverrides(`not json`); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLimitPrecedence(t *testing.T) {
	c := &Config{RunTimeout: 3000, LimitOverrides: map[string]map[string]int{
		"java": {RunTimeout: 9000},
	}}
	if v := c.Limit("java", RunTimeout, map[string]int{RunTimeout: 5000}); v != 9000 {
		t.Fatalf("config override should win, got %d", v)
	}
	if v := c.Limit("go", RunTimeout, map[string]int{RunTimeout: 5000}); v != 5000 {
		t.Fatalf("package override should win over default, got %d", v)
	}
	if v := c.Limit("go", RunTimeout, nil); v != 3000 {
		t.Fatalf("default expected, got %d", v)
	}
}
