package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ArkGravity/burrow/internal/burrow"
)

func TestKeysRotateAndMigrate(t *testing.T) {
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, "BURROW_") {
			t.Setenv(name, "")
			os.Unsetenv(name)
		}
	}
	t.Chdir(t.TempDir())
	dsn := filepath.Join(t.TempDir(), "cli.db")
	t.Setenv("BURROW_DB_DRIVER", "sqlite")
	t.Setenv("BURROW_DB_DSN", dsn)
	previous := os.Args
	t.Cleanup(func() { os.Args = previous })
	invoke := func(args ...string) error { os.Args = append([]string{"burrow"}, args...); return run() }
	if err := invoke("keys-rotate", "--protocol", "other"); err == nil || err.Error() != "invalid_protocol" {
		t.Fatal("unknown protocol accepted", err)
	}
	if err := invoke("keys-rotate", "--protocol", "saml"); err == nil || err.Error() != "invalid_reason" {
		t.Fatal("missing reason accepted", err)
	}
	if err := invoke("migrate"); err != nil {
		t.Fatal(err)
	}
	config, err := burrow.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	store, err := burrow.Open(config)
	if err != nil {
		t.Fatal(err)
	}
	pool, _ := store.DB.DB()
	defer pool.Close()
	var oidc burrow.SigningKey
	var saml burrow.SAMLSigningKey
	if err := store.DB.Where("active = ?", true).First(&oidc).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.DB.Where("active = ?", true).First(&saml).Error; err != nil {
		t.Fatal(err)
	}
	if err := invoke("migrate"); err != nil {
		t.Fatal(err)
	}
	var count int64
	store.DB.Model(&burrow.SAMLSigningKey{}).Count(&count)
	if count != 1 {
		t.Fatal("migrate replaced SAML key")
	}
	if err := invoke("keys-rotate"); err != nil {
		t.Fatal(err)
	}
	var after burrow.SigningKey
	store.DB.Where("active = ?", true).First(&after)
	if after.ID == oidc.ID {
		t.Fatal("default rotation did not rotate OIDC")
	}
	if err := invoke("keys-rotate", "--protocol", "saml", "--reason", "test CLI"); err != nil {
		t.Fatal(err)
	}
	var samlAfter burrow.SAMLSigningKey
	store.DB.Where("active = ?", true).First(&samlAfter)
	if samlAfter.ID == saml.ID {
		t.Fatal("explicit SAML rotation did not rotate key")
	}
	var oidcAfter burrow.SigningKey
	store.DB.Where("active = ?", true).First(&oidcAfter)
	if oidcAfter.ID != after.ID {
		t.Fatal("SAML rotation changed OIDC key")
	}
}
