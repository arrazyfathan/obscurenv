package cmd

import "testing"

func TestSplitLoginIdentifierTreatsEmailAsEmail(t *testing.T) {
	email, username := splitLoginIdentifier(" User@Example.COM ")
	if email != "User@Example.COM" {
		t.Fatalf("email = %q, want %q", email, "User@Example.COM")
	}
	if username != "" {
		t.Fatalf("username = %q, want empty", username)
	}
}

func TestSplitLoginIdentifierTreatsUsernameAsUsername(t *testing.T) {
	email, username := splitLoginIdentifier(" alice_dev ")
	if email != "" {
		t.Fatalf("email = %q, want empty", email)
	}
	if username != "alice_dev" {
		t.Fatalf("username = %q, want %q", username, "alice_dev")
	}
}
