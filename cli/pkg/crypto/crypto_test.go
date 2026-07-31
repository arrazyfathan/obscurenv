package crypto

import "testing"

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
