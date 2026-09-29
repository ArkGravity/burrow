package burrow

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func isolatedConfig(t *testing.T) {
	t.Helper()
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, "BURROW_") {
			t.Setenv(name, "")
			if err := os.Unsetenv(name); err != nil {
				t.Fatal(err)
			}
		}
	}
	t.Chdir(t.TempDir())
}

func TestDefaultConfigWithoutEnvironment(t *testing.T) {
	isolatedConfig(t)
	// Native commands must never read Compose's .env file.
	if err := os.WriteFile(".env", []byte("BURROW_ENV=prod\nBURROW_DB_DRIVER=broken\n"), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if c.Env != "dev" || c.DBDriver != "sqlite" || c.ListenAddr != ":8080" || c.SessionTTL != 8*time.Hour || c.MasterKey == [32]byte{} {
		t.Fatalf("unexpected defaults: env=%s db=%s listen=%s ttl=%s", c.Env, c.DBDriver, c.ListenAddr, c.SessionTTL)
	}
	if base64.StdEncoding.EncodeToString(c.MasterKey[:]) != developmentMasterKey {
		t.Fatal("default development key was not loaded")
	}
	if _, err := os.Stat("data/master.key"); !os.IsNotExist(err) {
		t.Fatal("default inline key should not create a key file")
	}
	// Clearing the inline key explicitly opts into persistent key-file mode.
	t.Setenv("BURROW_MASTER_KEY", "")
	c, err = LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	key, err := os.ReadFile("data/master.key")
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat("data/master.key")
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("master key must be private")
	}
	again, err := LoadConfig()
	if err != nil || again.MasterKey != c.MasterKey {
		t.Fatal("master key changed across config loads")
	}
	if !bytes.Equal(key, []byte(base64.StdEncoding.EncodeToString(c.MasterKey[:])+"\n")) {
		t.Fatal("persisted key mismatch")
	}
}

func TestConfigFileAndEnvironmentPrecedence(t *testing.T) {
	isolatedConfig(t)
	if err := os.Mkdir("configs", 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("configs/config.yaml", []byte("server:\n  listen_addr: 127.0.0.1:9000\nsession:\n  ttl: 3h\n"), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := LoadConfig()
	if err != nil || c.ListenAddr != "127.0.0.1:9000" || c.SessionTTL != 3*time.Hour {
		t.Fatalf("default file was not loaded: %v", err)
	}
	if err := os.WriteFile("custom.yaml", []byte("server:\n  listen_addr: 127.0.0.1:9001\n  trusted_proxies: [127.0.0.1/32]\nsession:\n  ttl: 2h\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BURROW_LISTEN_ADDR", "127.0.0.1:9002")
	t.Setenv("BURROW_TRUSTED_PROXIES", "")
	c, err = LoadConfigFile("custom.yaml")
	if err != nil || c.ListenAddr != "127.0.0.1:9002" || c.SessionTTL != 2*time.Hour || len(c.TrustedProxies) != 0 {
		t.Fatalf("precedence failed: %v", err)
	}
	if _, err = LoadConfigFile("missing.yaml"); err == nil {
		t.Fatal("explicit missing config accepted")
	}
}

func TestConfigRejectsInvalidValues(t *testing.T) {
	for name, document := range map[string]string{
		"unknown":                "server:\n  typo: value\n",
		"duration":               "session:\n  ttl: -1s\n",
		"cidr":                   "server:\n  trusted_proxies: [invalid]\n",
		"driver":                 "database:\n  driver: invalid\n",
		"key":                    "security:\n  master_key: invalid\n",
		"production SQLite":      "env: prod\nserver:\n  issuer: https://id.example\n",
		"production HTTP":        "env: prod\ndatabase:\n  driver: postgres\n",
		"production default key": "env: prod\nserver:\n  issuer: https://id.example\ndatabase:\n  driver: postgres\n",
		"production missing key": "env: prod\nserver:\n  issuer: https://id.example\ndatabase:\n  driver: postgres\nsecurity:\n  master_key: ''\n",
		"multiple documents":     "env: dev\n---\nenv: prod\n",
	} {
		t.Run(name, func(t *testing.T) {
			isolatedConfig(t)
			if err := os.WriteFile("bad.yaml", []byte(document), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadConfigFile("bad.yaml"); err == nil {
				t.Fatal("invalid config accepted")
			}
			if _, err := os.Stat("data/master.key"); !os.IsNotExist(err) {
				t.Fatal("invalid config created a master key")
			}
		})
	}
}

func TestConfigKeyOverrideAndCorruptKey(t *testing.T) {
	isolatedConfig(t)
	want := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))
	t.Setenv("BURROW_MASTER_KEY", want)
	c, err := LoadConfig()
	if err != nil || c.MasterKey[0] != 7 {
		t.Fatalf("environment key not loaded: %v", err)
	}
	if _, err := os.Stat("data/master.key"); !os.IsNotExist(err) {
		t.Fatal("explicit key created a local file")
	}
	t.Setenv("BURROW_MASTER_KEY", "")
	path := filepath.Join(t.TempDir(), "master.key")
	if err := os.WriteFile(path, []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BURROW_MASTER_KEY_FILE", path)
	if _, err := LoadConfig(); err == nil {
		t.Fatal("corrupt key accepted")
	}
	got, _ := os.ReadFile(path)
	if string(got) != "corrupt" {
		t.Fatal("existing key was overwritten")
	}
}

func TestDevelopmentMasterKeyConcurrentInitialization(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "master.key")
	type result struct {
		key string
		err error
	}
	results := make(chan result, 8)
	start := make(chan struct{})
	for i := 0; i < cap(results); i++ {
		go func() {
			<-start
			key, err := readMasterKey(path, true)
			results <- result{key, err}
		}()
	}
	close(start)
	var expected string
	for i := 0; i < cap(results); i++ {
		got := <-results
		if got.err != nil {
			t.Fatal(got.err)
		}
		if expected == "" {
			expected = got.key
		}
		if got.key != expected {
			t.Fatal("concurrent commands loaded different master keys")
		}
	}
	// A production process can read a provisioned file but never generates one.
	key, err := readMasterKey(path, false)
	if err != nil || key != expected {
		t.Fatalf("existing key not readable in production: %v", err)
	}
}

func TestProductionRejectsDevelopmentKeyFromEverySource(t *testing.T) {
	for _, source := range []string{"default", "environment", "file"} {
		t.Run(source, func(t *testing.T) {
			isolatedConfig(t)
			t.Setenv("BURROW_ENV", "prod")
			t.Setenv("BURROW_DB_DRIVER", "postgres")
			t.Setenv("BURROW_ISSUER", "https://id.example")
			if source == "environment" {
				t.Setenv("BURROW_MASTER_KEY", developmentMasterKey)
			}
			if source == "file" {
				t.Setenv("BURROW_MASTER_KEY", "")
				t.Setenv("BURROW_MASTER_KEY_FILE", "example.key")
				if err := os.WriteFile("example.key", []byte(developmentMasterKey), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), "public development master key") {
				t.Fatalf("example key accepted: %v", err)
			}
			t.Setenv("BURROW_MASTER_KEY", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{9}, 32)))
			if _, err := LoadConfig(); err != nil {
				t.Fatalf("custom key rejected: %v", err)
			}
		})
	}
}
