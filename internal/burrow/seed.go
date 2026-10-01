package burrow

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"
)

// BootstrapConfig is used only by the explicit seed command. Passwords are
// never returned by the API or printed by the command.
type BootstrapConfig struct {
	Username string `yaml:"admin_username"`
	Name     string `yaml:"admin_name"`
	Email    string `yaml:"admin_email"`
	Password string `yaml:"admin_password" json:"-"`
}

const developmentAdminPassword = "Burrow-development-admin-2026"

// Seed supplements the built-in roles and bootstraps an empty database. It does
// not reset passwords or change the status/assignments of existing users.
func (s *Store) Seed(options BootstrapConfig) error {
	if err := s.Health(context.Background()); err != nil {
		return err
	}
	options.Username = strings.TrimSpace(options.Username)
	if options.Username == "" || len(options.Username) > 128 {
		return errors.New("invalid bootstrap admin username")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.DB.Transaction(func(tx *gorm.DB) error {
		if s.Config.DBDriver == "postgres" {
			if err := tx.Exec("SELECT pg_advisory_xact_lock(734285622)").Error; err != nil {
				return err
			}
		}
		for _, role := range []Role{
			{ID: "admin", Name: "Administrator", Description: "Built-in administrator", Builtin: true},
		} {
			var existing Role
			err := tx.First(&existing, "id = ? OR name = ?", role.ID, role.Name).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				if err := tx.Create(&role).Error; err != nil {
					return err
				}
			} else if err != nil {
				return err
			} else if existing.ID != role.ID || !existing.Builtin {
				return errors.New("bootstrap role conflicts with an existing role")
			}
		}
		var user User
		err := tx.First(&user, "username = ?", options.Username).Error
		if err == nil {
			var count int64
			if err := tx.Model(&UserRole{}).Where("user_id = ? AND role_id = ?", user.ID, "admin").Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return errors.New("bootstrap username belongs to an existing non-administrator account")
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var count int64
		if err := tx.Model(&User{}).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return errors.New("administrator bootstrap requires an empty user database; preserve the existing bootstrap username")
		}
		if options.Password == "" && s.Config.Env == "dev" {
			options.Password = developmentAdminPassword
		}
		if s.Config.Env == "prod" && (options.Password == "" || options.Password == developmentAdminPassword) {
			return errors.New("production seed requires an independent bootstrap admin password")
		}
		h, err := passwordHash(options.Password)
		if err != nil {
			return errors.New("bootstrap admin password must be 12 to 256 characters")
		}
		if options.Name == "" {
			options.Name = "Administrator"
		}
		user = User{ID: random(18), Username: options.Username, Name: options.Name, Email: options.Email, Enabled: true, LocalEnabled: true,
			PasswordHash: h, MustChangePassword: true, Language: "en", Theme: "system"}
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		return tx.Create(&UserRole{UserID: user.ID, RoleID: "admin"}).Error
	})
}
