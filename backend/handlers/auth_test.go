package handlers

import "testing"

func TestNormalizeUsername(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "lowercases and trims", in: " Alice-Dev ", want: "alice-dev"},
		{name: "allows underscore", in: "alice_dev", want: "alice_dev"},
		{name: "rejects short", in: "ab", want: ""},
		{name: "rejects invalid character", in: "alice@example", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeUsername(tt.in); got != tt.want {
				t.Fatalf("normalizeUsername(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
