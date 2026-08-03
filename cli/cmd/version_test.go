package cmd

import (
	"strings"
	"testing"
)

func TestNormalizeVersionAcceptsSemVer(t *testing.T) {
	tests := map[string]string{
		"0.1.0":         "0.1.0",
		"v1.2.3":        "1.2.3",
		"1.2.3-beta.1":  "1.2.3-beta.1",
		"1.2.3+build.5": "1.2.3+build.5",
	}

	for input, want := range tests {
		if got := normalizeVersion(input); got != want {
			t.Fatalf("normalizeVersion(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeVersionRejectsInvalidSemVer(t *testing.T) {
	tests := []string{"", "1", "1.2", "01.2.3", "version-1.2.3"}

	for _, input := range tests {
		if got := normalizeVersion(input); got != "0.0.0-dev" {
			t.Fatalf("normalizeVersion(%q) = %q, want 0.0.0-dev", input, got)
		}
	}
}

func TestFormatVersion(t *testing.T) {
	got := FormatVersion(VersionInfo{
		Version: "1.2.3",
		Commit:  "abc123",
		BuiltAt: "2026-07-31T00:00:00Z",
	})

	for _, want := range []string{
		"obe version 1.2.3",
		"commit: abc123",
		"built: 2026-07-31T00:00:00Z",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("FormatVersion output missing %q: %q", want, got)
		}
	}
}
