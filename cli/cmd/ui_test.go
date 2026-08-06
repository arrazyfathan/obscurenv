package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestColorEnabledFalseForBuffers(t *testing.T) {
	noColorFlag = false
	t.Setenv("NO_COLOR", "")
	t.Setenv("OBE_NO_COLOR", "")
	var out bytes.Buffer
	if colorEnabled(&out) {
		t.Fatal("buffers must never be colored")
	}
}

func TestColorEnabledRespectsNoColorEnv(t *testing.T) {
	noColorFlag = false
	t.Setenv("NO_COLOR", "1")
	t.Setenv("OBE_NO_COLOR", "")
	var out bytes.Buffer
	if colorEnabled(&out) {
		t.Fatal("NO_COLOR must disable color")
	}
}

func TestColorEnabledRespectsNoColorFlag(t *testing.T) {
	noColorFlag = true
	t.Setenv("NO_COLOR", "")
	t.Setenv("OBE_NO_COLOR", "")
	defer func() { noColorFlag = false }()
	var out bytes.Buffer
	if colorEnabled(&out) {
		t.Fatal("--no-color must disable color")
	}
}

func TestStyledHelpersStayPlainForBuffers(t *testing.T) {
	noColorFlag = false
	t.Setenv("NO_COLOR", "")
	t.Setenv("OBE_NO_COLOR", "")
	var out bytes.Buffer
	success(&out, "Done.")
	if strings.Contains(out.String(), "\x1b[") {
		t.Fatalf("expected plain output, got %q", out.String())
	}
	if !strings.Contains(out.String(), "Done.") {
		t.Fatalf("output = %q, want message", out.String())
	}
}

func TestStyledHelpersPrefixOnlyWhenColored(t *testing.T) {
	// Simulate a terminal writer by directly checking the rendered prefix logic
	// via colorEnabled gating: buffers keep the message intact.
	noColorFlag = false
	t.Setenv("NO_COLOR", "")
	t.Setenv("OBE_NO_COLOR", "")
	var out bytes.Buffer
	info(&out, "Heads up.")
	if out.String() != "Heads up.\n" {
		t.Fatalf("output = %q, want plain line", out.String())
	}
}

func TestPrintTableAlignsColumns(t *testing.T) {
	var out bytes.Buffer
	printTable(&out, []string{"ID", "Name", "Expires"}, [][]string{
		{"abc", "ci", "never"},
		{"long-token-id", "deploy", "2026-12-31T00:00:00Z"},
	})
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines: %q", len(lines), out.String())
	}
	headerID := strings.Index(lines[0], "ID")
	headerName := strings.Index(lines[0], "Name")
	rowName := strings.Index(lines[1], "ci")
	rowName2 := strings.Index(lines[2], "deploy")
	if headerName != rowName || headerName != rowName2 {
		t.Fatalf("columns misaligned: header Name at %d, rows at %d and %d\n%s", headerName, rowName, rowName2, out.String())
	}
	if headerID != 0 {
		t.Fatalf("header ID should start at column 0, got %d", headerID)
	}
	if !strings.Contains(lines[2], "long-token-id") || !strings.Contains(lines[2], "2026-12-31T00:00:00Z") {
		t.Fatalf("row 2 = %q", lines[2])
	}
}

func TestPrintTableEmptyRows(t *testing.T) {
	var out bytes.Buffer
	printTable(&out, []string{"ID", "Name"}, nil)
	if !strings.Contains(out.String(), "ID") || !strings.Contains(out.String(), "Name") {
		t.Fatalf("header missing: %q", out.String())
	}
}
