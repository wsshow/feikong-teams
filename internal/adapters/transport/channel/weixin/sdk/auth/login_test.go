package auth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validTestCredentials() *Credentials {
	return &Credentials{
		Token:     "token",
		BaseURL:   "https://example.com",
		AccountID: "account",
		UserID:    "user",
	}
}

func TestCredentialsRoundTripAndPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "credentials.json")
	if err := SaveCredentials(validTestCredentials(), path); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadCredentials(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Token != "token" || loaded.UserID != "user" {
		t.Fatalf("loaded credentials = %#v", loaded)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("credentials permissions = %o, want 600", info.Mode().Perm())
	}
	if err := ClearCredentials(path); err != nil {
		t.Fatal(err)
	}
	if err := ClearCredentials(path); err != nil {
		t.Fatalf("second clear failed: %v", err)
	}
}

func TestLoadCredentialsRejectsOversizedAndIncompleteFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", int(maxCredentialsBytes+1))), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCredentials(path); err == nil || !strings.Contains(err.Error(), "exceed") {
		t.Fatalf("oversized credentials error = %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"token":"token"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCredentials(path); err == nil || !strings.Contains(err.Error(), "base URL") {
		t.Fatalf("incomplete credentials error = %v", err)
	}
}

func TestSaveCredentialsRejectsNil(t *testing.T) {
	if err := SaveCredentials(nil, filepath.Join(t.TempDir(), "credentials.json")); err == nil {
		t.Fatal("SaveCredentials accepted nil credentials")
	}
}
