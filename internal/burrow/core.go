package burrow

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Config struct {
	Bootstrap                                                   BootstrapConfig
	TrustedProxies                                              []netip.Prefix
	Env, ListenAddr, Issuer, DBDriver, DBDSN, StaticDir         string
	MasterKey                                                   [32]byte
	SessionTTL, TokenTTL, AuthCodeTTL, LoginTTL, EventRetention time.Duration
}

type Store struct {
	DB     *gorm.DB
	Config Config
	mu     sync.Mutex
}

func Open(c Config) (*Store, error) {
	c.defaults()
	var d gorm.Dialector
	switch c.DBDriver {
	case "sqlite":
		sep := "?"
		if strings.Contains(c.DBDSN, "?") {
			sep = "&"
		}
		d = sqlite.Open(c.DBDSN + sep + "_foreign_keys=on&_busy_timeout=10000&_journal_mode=WAL&_txlock=immediate")
	case "postgres":
		d = postgres.Open(c.DBDSN)
	default:
		return nil, errors.New("invalid database driver")
	}
	db, err := gorm.Open(d, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), TranslateError: true})
	if err != nil {
		return nil, err
	}
	sql, err := db.DB()
	if err != nil {
		return nil, err
	}
	if c.DBDriver == "sqlite" {
		sql.SetMaxOpenConns(1)
	} else {
		sql.SetMaxOpenConns(10)
	}
	return &Store{DB: db, Config: c}, nil
}

func (s *Store) Migrate() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.DB.Transaction(func(tx *gorm.DB) error {
		if s.Config.DBDriver == "postgres" {
			if err := tx.Exec("SELECT pg_advisory_xact_lock(734285621)").Error; err != nil {
				return err
			}
		}
		if !tx.Migrator().HasTable(&SchemaVersion{}) {
			if err := tx.Migrator().CreateTable(&SchemaVersion{}); err != nil {
				return err
			}
			if err := tx.Create(&SchemaVersion{ID: 1}).Error; err != nil {
				return err
			}
		}
		var v SchemaVersion
		if err := tx.First(&v, 1).Error; err != nil {
			return err
		}
		migrations := []string{initialMigration, pkceCompatibilityMigration, customRolesMigration, removeUpstreamMigration}
		if v.Version > len(migrations) {
			return errors.New("database schema is newer than binary")
		}
		if v.Version > 0 {
			if v.Checksum != migrationChecksum(v.Version) {
				return errors.New("migration checksum mismatch")
			}
		}
		for version := v.Version; version < len(migrations); version++ {
			if version == 3 {
				if err := checkPasswordLoginMigration(tx); err != nil {
					return err
				}
			}
			for _, statement := range strings.Split(migrations[version], ";") {
				if strings.TrimSpace(statement) != "" {
					if err := tx.Exec(statement).Error; err != nil {
						return err
					}
				}
			}
			if err := tx.Model(&v).Updates(map[string]any{"version": version + 1, "checksum": migrationChecksum(version + 1)}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
func (s *Store) Health(ctx context.Context) error {
	var v SchemaVersion
	if err := s.DB.WithContext(ctx).First(&v, 1).Error; err != nil {
		return err
	}
	if v.Version != 4 || v.Checksum != migrationChecksum(4) {
		return errors.New("migration required")
	}
	return nil
}
func random(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func hash(v string) string {
	h := sha256.Sum256([]byte(v))
	return base64.RawURLEncoding.EncodeToString(h[:])
}
func passwordHash(p string) (string, error) {
	if len(p) < 12 || len(p) > 256 {
		return "", errors.New("password_policy")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(p), salt, 2, 64*1024, 2, 32)
	return "$argon2id$v=19$m=65536,t=2,p=2$" + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(key), nil
}
func passwordOK(encoded, p string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v=19" || parts[3] != "m=65536,t=2,p=2" || len(p) > 256 {
		return false
	}
	salt, e := base64.RawStdEncoding.DecodeString(parts[4])
	want, e2 := base64.RawStdEncoding.DecodeString(parts[5])
	if e != nil || e2 != nil || len(salt) != 16 || len(want) != 32 {
		return false
	}
	got := argon2.IDKey([]byte(p), salt, 2, 64*1024, 2, 32)
	return subtle.ConstantTimeCompare(got, want) == 1
}
func (s *Store) seal(value string) (string, error) {
	block, err := aes.NewCipher(s.Config.MasterKey[:])
	if err != nil {
		return "", err
	}
	a, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	n := make([]byte, a.NonceSize())
	if _, err = rand.Read(n); err != nil {
		return "", err
	}
	return base64.RawStdEncoding.EncodeToString(a.Seal(n, n, []byte(value), []byte("burrow:v1"))), nil
}
func (s *Store) unseal(value string) (string, error) {
	b, err := base64.RawStdEncoding.DecodeString(value)
	if err != nil {
		return "", err
	}
	block, _ := aes.NewCipher(s.Config.MasterKey[:])
	a, _ := cipher.NewGCM(block)
	if len(b) < a.NonceSize() {
		return "", errors.New("invalid ciphertext")
	}
	p, err := a.Open(nil, b[:a.NonceSize()], b[a.NonceSize():], []byte("burrow:v1"))
	return string(p), err
}
func roleIDs(db *gorm.DB, userID string) ([]string, error) {
	ids := []string{}
	err := db.Raw("SELECT role_id FROM user_roles WHERE user_id = ? UNION SELECT gr.role_id FROM group_roles gr JOIN group_members gm ON gm.group_id = gr.group_id WHERE gm.user_id = ?", userID, userID).Scan(&ids).Error
	return ids, err
}
func permissions(db *gorm.DB, u User) ([]string, bool, error) {
	ids, err := roleIDs(db, u.ID)
	if err != nil {
		return nil, false, err
	}
	admin := contains(ids, "admin")
	p := []string{}
	if admin {
		err = db.Model(&Permission{}).Pluck("id", &p).Error
	} else if len(ids) > 0 {
		err = db.Model(&RolePermission{}).Distinct("permission_id").Where("role_id IN ?", ids).Pluck("permission_id", &p).Error
	}
	return p, admin, err
}
func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
func enabledAdminExists(tx *gorm.DB) error {
	var users []User
	if err := tx.Where("enabled = ? AND password_hash <> ?", true, "").Find(&users).Error; err != nil {
		return err
	}
	for _, u := range users {
		ids, err := roleIDs(tx, u.ID)
		if err != nil {
			return err
		}
		if contains(ids, "admin") {
			return nil
		}
	}
	return errors.New("last_admin")
}
func (s *Store) event(actor, object, kind, request string, success bool) {
	s.DB.Create(&Event{ID: random(18), ActorID: actor, ObjectID: object, Kind: kind, Success: success, RequestID: request, CreatedAt: time.Now()})
}
func (s *Store) Cleanup(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			for _, m := range []any{&Session{}, &AuthTransaction{}, &TokenRecord{}} {
				s.DB.Where("expires_at < ?", now).Delete(m)
			}
			s.DB.Where("created_at < ?", now.Add(-s.Config.EventRetention)).Delete(&Event{})
		}
	}
}
func invalid(field string) error { return fmt.Errorf("invalid_%s", field) }

//go:embed migrations/001_initial.sql
var initialMigration string

//go:embed migrations/002_pkce_compatibility.sql
var pkceCompatibilityMigration string

//go:embed migrations/003_custom_roles.sql
var customRolesMigration string

//go:embed migrations/004_remove_upstream.sql
var removeUpstreamMigration string

// Do not silently enable password login or lock out active upstream-only accounts.
// Operators must prepare these accounts and applications using the old version.
func checkPasswordLoginMigration(tx *gorm.DB) error {
	var users, applications int64
	if err := tx.Table("users").Where("enabled = ? AND (local_enabled = ? OR password_hash IS NULL OR password_hash = ?)", true, false, "").Count(&users).Error; err != nil {
		return err
	}
	if err := tx.Table("applications").Where("enabled = ? AND local_enabled = ?", true, false).Count(&applications).Error; err != nil {
		return err
	}
	if users > 0 || applications > 0 {
		return fmt.Errorf("migration 004 requires password login for enabled users and applications (%d users, %d applications): use the previous version to configure passwords and enable local login, or disable unused records, before retrying", users, applications)
	}
	return nil
}

func migrationChecksum(version int) string {
	if version == 1 {
		return hash(initialMigration)
	}
	if version == 2 {
		return hash(initialMigration + pkceCompatibilityMigration)
	}
	if version == 3 {
		return hash(initialMigration + pkceCompatibilityMigration + customRolesMigration)
	}
	return hash(initialMigration + pkceCompatibilityMigration + customRolesMigration + removeUpstreamMigration)
}

func (c *Config) defaults() {
	f := configDefaults()
	if c.SessionTTL == 0 {
		c.SessionTTL = f.Session.TTL
	}
	if c.TokenTTL == 0 {
		c.TokenTTL = f.OIDC.TokenTTL
	}
	if c.AuthCodeTTL == 0 {
		c.AuthCodeTTL = f.OIDC.AuthCodeTTL
	}
	if c.LoginTTL == 0 {
		c.LoginTTL = f.OIDC.LoginTTL
	}
	if c.EventRetention == 0 {
		c.EventRetention = f.Audit.Retention
	}
}
