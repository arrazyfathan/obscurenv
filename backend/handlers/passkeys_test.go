package handlers

import (
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

func TestPasskeySessionExpiresUsesExistingExpiry(t *testing.T) {
	want := time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)

	if got := passkeySessionExpires(webauthn.SessionData{Expires: want}); !got.Equal(want) {
		t.Fatalf("passkeySessionExpires() = %s, want %s", got, want)
	}
}

func TestPasskeySessionExpiresAddsFallbackForZeroExpiry(t *testing.T) {
	before := time.Now().UTC().Add(passkeySessionTTL - time.Second)

	got := passkeySessionExpires(webauthn.SessionData{})

	after := time.Now().UTC().Add(passkeySessionTTL + time.Second)
	if got.Before(before) || got.After(after) {
		t.Fatalf("passkeySessionExpires() = %s, want between %s and %s", got, before, after)
	}
}
