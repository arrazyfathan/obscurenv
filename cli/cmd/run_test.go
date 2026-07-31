package cmd

import "testing"

func TestParseEnv(t *testing.T) {
	got, err := parseEnv(`
# comment
DATABASE_URL=postgres://localhost
export SECRET="value"
EMPTY=''
`)
	if err != nil {
		t.Fatalf("parseEnv: %v", err)
	}
	want := []string{
		"DATABASE_URL=postgres://localhost",
		"SECRET=value",
		"EMPTY=",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d values, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseEnvRejectsInvalidLine(t *testing.T) {
	if _, err := parseEnv("NOT_VALID\n"); err == nil {
		t.Fatal("expected invalid line error")
	}
}
