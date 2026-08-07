package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestEncryptDecryptWithPassphrase(t *testing.T) {
	plaintext := []byte("DATABASE_URL=postgres://localhost\nSECRET=value\n")
	payload, err := EncryptWithPassphrase(plaintext, "correct horse battery staple")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	got, err := DecryptWithPassphrase(payload, "correct horse battery staple")
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(got) != string(plaintext) {
		t.Fatalf("plaintext mismatch: %q", got)
	}
}

func TestDecryptWithWrongPassphraseFails(t *testing.T) {
	payload, err := EncryptWithPassphrase([]byte("SECRET=value\n"), "right")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if _, err := DecryptWithPassphrase(payload, "wrong"); err == nil {
		t.Fatal("expected decrypt failure")
	}
}

func TestDecryptsLegacyV1Envelope(t *testing.T) {
	plaintext := []byte("LEGACY=value\n")
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		t.Fatalf("salt: %v", err)
	}
	key := DeriveKey("legacy-passphrase", salt)
	ciphertext, err := Encrypt(plaintext, key)
	Wipe(key)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	payload, err := json.Marshal(map[string]any{
		"version":    1,
		"kdf":        kdfName,
		"salt":       base64.StdEncoding.EncodeToString(salt),
		"ciphertext": ciphertext,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := DecryptWithPassphrase(string(payload), "legacy-passphrase")
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(got) != string(plaintext) {
		t.Fatalf("plaintext mismatch: %q", got)
	}
}

func TestV2EnvelopeContainsPassphraseKeySlot(t *testing.T) {
	payload, err := EncryptWithPassphrase([]byte("KEY=value\n"), "passphrase")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	var envelope Envelope
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if envelope.Version != 2 || envelope.KeySlots["passphrase"].WrappedKey == "" {
		t.Fatalf("unexpected v2 envelope: %+v", envelope)
	}
}
