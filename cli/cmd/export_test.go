package cmd

import (
	"bytes"
	"net/http"
	"os"
	"strings"
	"testing"

	obecrypto "github.com/obscurenv/obscurenv/cli/pkg/crypto"
)

func TestExportWritesDecryptedEnvironmentFiles(t *testing.T) {
	withTempWorkingDir(t)
	setupExportCommandTest(t)

	const passphrase = "test-key"
	productionPayload, err := obecrypto.EncryptWithPassphrase([]byte("FOO=bar\n"), passphrase)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	stagingPayload, err := obecrypto.EncryptWithPassphrase([]byte("BAZ=qux\n"), passphrase)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	restore := stubAPIClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/env/export" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"project_slug":"obsecurenv","environments":[{"environment":"production","version":2,"checksum":"c1","encrypted_payload":` +
			strconvQuote(productionPayload) + `,"created_at":"2026-08-06T00:00:00Z"},{"environment":"staging","version":1,"checksum":"c2","encrypted_payload":` +
			strconvQuote(stagingPayload) + `,"created_at":"2026-08-06T00:00:00Z"}]}`))
	}))
	t.Cleanup(restore)

	oldKey := exportKey
	t.Cleanup(func() {
		exportKey = oldKey
		exportCmd.SetOut(nil)
	})
	exportKey = passphrase

	var out bytes.Buffer
	exportCmd.SetOut(&out)

	if err := exportCmd.RunE(exportCmd, nil); err != nil {
		t.Fatalf("export: %v", err)
	}

	production, err := os.ReadFile("production.env")
	if err != nil {
		t.Fatalf("read production.env: %v", err)
	}
	if string(production) != "FOO=bar\n" {
		t.Fatalf("production.env = %q, want decrypted content", production)
	}
	staging, err := os.ReadFile("staging.env")
	if err != nil {
		t.Fatalf("read staging.env: %v", err)
	}
	if string(staging) != "BAZ=qux\n" {
		t.Fatalf("staging.env = %q, want decrypted content", staging)
	}
	if !strings.Contains(out.String(), `Exported "production" into production.env.`) {
		t.Fatalf("output = %q, want export confirmation", out.String())
	}
}

func TestExportFailsWithoutWritingWhenDecryptFails(t *testing.T) {
	withTempWorkingDir(t)
	setupExportCommandTest(t)

	restore := stubAPIClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/env/export" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"project_slug":"obsecurenv","environments":[{"environment":"production","version":1,"checksum":"c1","encrypted_payload":"not-a-valid-envelope","created_at":"2026-08-06T00:00:00Z"}]}`))
	}))
	t.Cleanup(restore)

	oldKey := exportKey
	t.Cleanup(func() {
		exportKey = oldKey
		exportCmd.SetOut(nil)
	})
	exportKey = "wrong-key"

	var out bytes.Buffer
	exportCmd.SetOut(&out)

	err := exportCmd.RunE(exportCmd, nil)
	if err == nil {
		t.Fatal("expected export to fail on decrypt error")
	}
	if _, statErr := os.Stat("production.env"); !os.IsNotExist(statErr) {
		t.Fatal("expected production.env not to be written on decrypt failure")
	}
}

func strconvQuote(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}

func setupExportCommandTest(t *testing.T) {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("OBE_API_URL", "")
	if err := saveCredentials(Credentials{Token: "test-token", APIURL: "http://obe.test"}); err != nil {
		t.Fatalf("saveCredentials: %v", err)
	}
	if err := saveProjectConfig(ProjectConfig{
		ProjectSlug:       "obsecurenv",
		ActiveEnvironment: "development",
	}); err != nil {
		t.Fatalf("saveProjectConfig: %v", err)
	}
}
