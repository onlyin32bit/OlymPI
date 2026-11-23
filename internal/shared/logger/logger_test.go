package logger

import "testing"

func TestNormalizeLevel(t *testing.T) {
	tests := map[string]string{
		"":        "info",
		" debug ": "debug",
		"INFO":    "info",
		"warning": "warn",
		"error":   "error",
	}
	for input, expected := range tests {
		t.Run(input, func(t *testing.T) {
			got, err := NormalizeLevel(input)
			if err != nil {
				t.Fatal(err)
			}
			if got != expected {
				t.Fatalf("NormalizeLevel(%q) = %q, want %q", input, got, expected)
			}
		})
	}
}

func TestNormalizeLevelRejectsUnknownValues(t *testing.T) {
	if _, err := NormalizeLevel("verbose"); err == nil {
		t.Fatal("expected unknown log level to fail")
	}
}
