package api

import "testing"

func TestPickLimit(t *testing.T) {
	body := map[string]any{
		"run_timeout":      float64(1000),
		"compile_timeout":  float64(99999),
		"run_cpu_time":     "fast",
		"compile_cpu_time": float64(0),
		"run_memory_limit": float64(-5),
	}

	if v, err := pickLimit(body, "run_timeout", 3000); err != nil || v != 1000 {
		t.Fatalf("got %d, %v", v, err)
	}
	if _, err := pickLimit(body, "compile_timeout", 10000); err == nil {
		t.Fatal("expected error when exceeding configured limit")
	}
	if _, err := pickLimit(body, "run_cpu_time", 3000); err == nil {
		t.Fatal("expected error for non-number")
	}
	// Zero must fall back to the configured default, never "unlimited".
	if v, err := pickLimit(body, "compile_cpu_time", 10000); err != nil || v != 10000 {
		t.Fatalf("got %d, %v", v, err)
	}
	if _, err := pickLimit(body, "run_memory_limit", 1000000); err == nil {
		t.Fatal("expected error for negative value")
	}
	// Absent -> default.
	if v, _ := pickLimit(body, "missing", 42); v != 42 {
		t.Fatalf("got %d", v)
	}
}

func TestParseBody(t *testing.T) {
	if b, err := parseBody(nil); err != nil || len(b) != 0 {
		t.Fatalf("empty body: %v %v", b, err)
	}
	if _, err := parseBody([]byte(`[1]`)); err == nil {
		t.Fatal("array body must be rejected")
	}
	if _, err := parseBody([]byte(`{`)); err == nil {
		t.Fatal("invalid JSON must be rejected")
	}
}
