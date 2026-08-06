package cmd

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
)

func TestPasswdChangesPassword(t *testing.T) {
	setupAuthCommandTest(t)

	oldCurrent := passwdCurrent
	oldNew := passwdNew
	t.Cleanup(func() {
		passwdCurrent = oldCurrent
		passwdNew = oldNew
		passwdCmd.SetOut(nil)
	})
	passwdCurrent = "old-pass"
	passwdNew = "new-pass-123"

	var sawChange bool
	restore := stubAPIClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/user/password" {
			http.NotFound(w, r)
			return
		}
		sawChange = true
		_, _ = w.Write([]byte(`{"message":"password updated"}`))
	}))
	t.Cleanup(restore)

	var out bytes.Buffer
	passwdCmd.SetOut(&out)

	if err := passwdCmd.RunE(passwdCmd, nil); err != nil {
		t.Fatalf("passwd: %v", err)
	}
	if !sawChange {
		t.Fatal("expected passwd to call change API")
	}
	if !strings.Contains(out.String(), "Password updated.") {
		t.Fatalf("output = %q, want confirmation", out.String())
	}
}

func TestPasswdRejectsShortNewPassword(t *testing.T) {
	setupAuthCommandTest(t)

	oldCurrent := passwdCurrent
	oldNew := passwdNew
	t.Cleanup(func() {
		passwdCurrent = oldCurrent
		passwdNew = oldNew
	})
	passwdCurrent = "old-pass"
	passwdNew = "short"

	var out bytes.Buffer
	passwdCmd.SetOut(&out)

	err := passwdCmd.RunE(passwdCmd, nil)
	if err == nil {
		t.Fatal("expected passwd to reject short new password")
	}
	if !strings.Contains(err.Error(), "at least 8 characters") {
		t.Fatalf("error = %q, want length hint", err)
	}
}

func TestAccountRmDeletesAccount(t *testing.T) {
	setupAuthCommandTest(t)

	oldYes := accountYes
	t.Cleanup(func() {
		accountYes = oldYes
		accountRmCmd.SetOut(nil)
	})
	accountYes = true

	var sawDelete bool
	restore := stubAPIClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/v1/user" {
			http.NotFound(w, r)
			return
		}
		sawDelete = true
		_, _ = w.Write([]byte(`{"message":"account deleted"}`))
	}))
	t.Cleanup(restore)

	var out bytes.Buffer
	accountRmCmd.SetOut(&out)

	if err := accountRmCmd.RunE(accountRmCmd, nil); err != nil {
		t.Fatalf("account rm: %v", err)
	}
	if !sawDelete {
		t.Fatal("expected account rm to call delete API")
	}
	if !strings.Contains(out.String(), "Account deleted.") {
		t.Fatalf("output = %q, want confirmation", out.String())
	}
}
