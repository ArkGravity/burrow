package burrow

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/logic3579/burrow/configs"
	"gopkg.in/yaml.v3"
)

type fileConfig struct {
	Bootstrap BootstrapConfig `yaml:"bootstrap"`
	Env       string          `yaml:"env"`
	Server    struct {
		ListenAddr     string   `yaml:"listen_addr"`
		Issuer         string   `yaml:"issuer"`
		StaticDir      string   `yaml:"static_dir"`
		TrustedProxies []string `yaml:"trusted_proxies"`
	} `yaml:"server"`
	Database struct {
		Driver string `yaml:"driver"`
		DSN    string `yaml:"dsn"`
	} `yaml:"database"`
	Security struct {
		MasterKey     string `yaml:"master_key"`
		MasterKeyFile string `yaml:"master_key_file"`
	} `yaml:"security"`
	Session struct {
		TTL time.Duration `yaml:"ttl"`
	} `yaml:"session"`
	OIDC struct {
		TokenTTL    time.Duration `yaml:"token_ttl"`
		AuthCodeTTL time.Duration `yaml:"auth_code_ttl"`
		LoginTTL    time.Duration `yaml:"login_ttl"`
	} `yaml:"oidc"`
	Providers struct {
		AllowedCIDRs []string `yaml:"allowed_cidrs"`
		AllowPrivate bool     `yaml:"allow_private"`
	} `yaml:"providers"`
	Audit struct {
		Retention time.Duration `yaml:"retention"`
	} `yaml:"audit"`
}

// This public example is intentionally usable only in development.
const developmentMasterKey = "MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIzNDU2Nzg5MDE="

func decodeConfig(raw []byte, c *fileConfig) error {
	d := yaml.NewDecoder(bytes.NewReader(raw))
	d.KnownFields(true)
	if err := d.Decode(c); err != nil {
		return errors.New("invalid YAML configuration: check field names and value types")
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return errors.New("configuration must contain one YAML document")
	}
	return nil
}

func configDefaults() fileConfig {
	var c fileConfig
	if err := decodeConfig(configs.Default, &c); err != nil {
		panic(err)
	}
	return c
}

func LoadConfig() (Config, error) { return LoadConfigFile("") }

// LoadConfigFile overlays a file and environment variables onto the embedded
// defaults. Relative paths are always relative to the process working directory.
// An omitted path uses configs/config.yaml when present; explicit paths must exist.
func LoadConfigFile(path string) (Config, error) {
	f := configDefaults()
	explicit := path != ""
	if !explicit {
		path = "configs/config.yaml"
	}
	raw, err := os.ReadFile(path)
	if err == nil {
		if err = decodeConfig(raw, &f); err != nil {
			return Config{}, fmt.Errorf("%s: %w", path, err)
		}
	} else if explicit || !errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	for name, target := range map[string]*string{
		"ENV": &f.Env, "LISTEN_ADDR": &f.Server.ListenAddr, "ISSUER": &f.Server.Issuer,
		"STATIC_DIR": &f.Server.StaticDir, "DB_DRIVER": &f.Database.Driver, "DB_DSN": &f.Database.DSN,
		"MASTER_KEY": &f.Security.MasterKey, "MASTER_KEY_FILE": &f.Security.MasterKeyFile,
		"BOOTSTRAP_ADMIN_USERNAME": &f.Bootstrap.Username, "BOOTSTRAP_ADMIN_NAME": &f.Bootstrap.Name,
		"BOOTSTRAP_ADMIN_EMAIL": &f.Bootstrap.Email, "BOOTSTRAP_ADMIN_PASSWORD": &f.Bootstrap.Password,
	} {
		if value, ok := os.LookupEnv("BURROW_" + name); ok {
			*target = value
		}
	}
	for name, target := range map[string]*time.Duration{
		"SESSION_TTL": &f.Session.TTL, "TOKEN_TTL": &f.OIDC.TokenTTL,
		"AUTH_CODE_TTL": &f.OIDC.AuthCodeTTL, "LOGIN_TTL": &f.OIDC.LoginTTL, "EVENT_RETENTION": &f.Audit.Retention,
	} {
		if value, ok := os.LookupEnv("BURROW_" + name); ok {
			*target, err = time.ParseDuration(value)
			if err != nil {
				return Config{}, fmt.Errorf("invalid BURROW_%s duration", name)
			}
		}
		if *target <= 0 {
			return Config{}, fmt.Errorf("%s must be a positive duration", name)
		}
	}
	for name, target := range map[string]*[]string{"TRUSTED_PROXIES": &f.Server.TrustedProxies, "PROVIDER_ALLOWED_CIDRS": &f.Providers.AllowedCIDRs} {
		if value, ok := os.LookupEnv("BURROW_" + name); ok {
			*target = strings.Split(value, ",")
		}
	}
	if value, ok := os.LookupEnv("BURROW_ALLOW_PRIVATE_PROVIDERS"); ok {
		f.Providers.AllowPrivate, err = strconv.ParseBool(value)
		if err != nil {
			return Config{}, errors.New("invalid BURROW_ALLOW_PRIVATE_PROVIDERS boolean")
		}
	}
	c := Config{Env: f.Env, ListenAddr: f.Server.ListenAddr, Issuer: strings.TrimRight(f.Server.Issuer, "/"), StaticDir: f.Server.StaticDir,
		Bootstrap: f.Bootstrap,
		DBDriver:  f.Database.Driver, DBDSN: f.Database.DSN, AllowPrivateProviders: f.Providers.AllowPrivate,
		SessionTTL: f.Session.TTL, TokenTTL: f.OIDC.TokenTTL, AuthCodeTTL: f.OIDC.AuthCodeTTL, LoginTTL: f.OIDC.LoginTTL, EventRetention: f.Audit.Retention}
	if c.Env == "development" {
		c.Env = "dev"
	}
	if c.Env == "production" {
		c.Env = "prod"
	}
	if c.Env != "dev" && c.Env != "prod" {
		return c, errors.New("env must be dev or prod")
	}
	if c.DBDriver != "postgres" && c.DBDriver != "sqlite" {
		return c, errors.New("database.driver must be postgres or sqlite")
	}
	if c.DBDSN == "" || c.ListenAddr == "" {
		return c, errors.New("database.dsn and server.listen_addr must not be empty")
	}
	if c.Env == "prod" && c.DBDriver != "postgres" {
		return c, errors.New("production requires PostgreSQL")
	}
	u, err := url.Parse(c.Issuer)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || (u.Scheme != "https" && !(c.Env == "dev" && u.Scheme == "http")) {
		return c, errors.New("invalid issuer (production requires HTTPS, no path/query)")
	}
	for _, entry := range []struct {
		values []string
		target *[]netip.Prefix
		name   string
	}{
		{f.Server.TrustedProxies, &c.TrustedProxies, "server.trusted_proxies"},
		{f.Providers.AllowedCIDRs, &c.ProviderAllowedCIDRs, "providers.allowed_cidrs"},
	} {
		for _, value := range entry.values {
			if strings.TrimSpace(value) == "" {
				continue
			}
			prefix, err := netip.ParsePrefix(strings.TrimSpace(value))
			if err != nil {
				return c, fmt.Errorf("invalid %s CIDR", entry.name)
			}
			*entry.target = append(*entry.target, prefix)
		}
	}
	encoded := f.Security.MasterKey
	if encoded == "" {
		encoded, err = readMasterKey(f.Security.MasterKeyFile, c.Env == "dev")
		if err != nil {
			return c, err
		}
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil || len(key) != 32 {
		return c, errors.New("master key must be base64 of 32 random bytes")
	}
	copy(c.MasterKey[:], key)
	if c.Env == "prod" && base64.StdEncoding.EncodeToString(key) == developmentMasterKey {
		return c, errors.New("production requires replacing the public development master key")
	}
	return c, nil
}

func readMasterKey(path string, development bool) (string, error) {
	if path == "" {
		return "", errors.New("security.master_key or security.master_key_file is required")
	}
	raw, err := os.ReadFile(path)
	if err == nil {
		return string(raw), nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("read master key: %w", err)
	}
	if !development {
		return "", errors.New("production requires an explicit master key or an existing key file")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", err
	}
	// Publish a complete key atomically without ever replacing an existing file,
	// including when two local commands start for the first time concurrently.
	temp, err := os.CreateTemp(filepath.Dir(path), ".master-key-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(temp.Name())
	_, writeErr := temp.WriteString(base64.StdEncoding.EncodeToString(key) + "\n")
	closeErr := temp.Close()
	if writeErr != nil {
		return "", writeErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	if err := os.Link(temp.Name(), path); err != nil && !errors.Is(err, os.ErrExist) {
		return "", err
	}
	raw, err = os.ReadFile(path)
	return string(raw), err
}
