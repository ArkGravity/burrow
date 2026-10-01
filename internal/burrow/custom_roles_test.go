package burrow

import (
	"context"
	"testing"
)

func TestCustomRolesMigration(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		for _, scenario := range []string{"empty-viewer", "assigned-editor", "unused-editor", "assigned-viewer-permission", "custom-role", "audit-rollback"} {
			t.Run(driver+"/"+scenario, func(t *testing.T) {
				s := testStore(t, driver)
				role := Role{ID: "viewer", Name: "Viewer", Builtin: true}
				if scenario == "assigned-editor" || scenario == "unused-editor" {
					role.ID = "editor"
					role.Name = "Editor"
				}
				if scenario == "custom-role" {
					role.Builtin = false
				}
				if err := s.DB.Create(&role).Error; err != nil {
					t.Fatal(err)
				}
				user := User{ID: random(18), Username: "legacy", Enabled: true}
				group := Group{ID: random(18), Name: "legacy-group"}
				s.DB.Create(&user)
				s.DB.Create(&group)
				s.DB.Create(&GroupMember{UserID: user.ID, GroupID: group.ID})
				if scenario != "unused-editor" {
					s.DB.Create(&UserRole{UserID: user.ID, RoleID: role.ID})
					s.DB.Create(&GroupRole{GroupID: group.ID, RoleID: role.ID})
				}
				if scenario != "empty-viewer" && scenario != "audit-rollback" {
					s.DB.Create(&RolePermission{RoleID: role.ID, PermissionID: "applications:write"})
				}
				before, _, err := permissions(s.DB, user)
				if err != nil {
					t.Fatal(err)
				}
				s.DB.Delete(&Event{}, "id = ?", "migration:003:custom_roles")
				if err := s.DB.Model(&SchemaVersion{}).Where("id = 1").Updates(map[string]any{"version": 2, "checksum": migrationChecksum(2)}).Error; err != nil {
					t.Fatal(err)
				}
				if s.Health(context.Background()) == nil {
					t.Fatal("v2 schema ready before upgrade")
				}
				s.DB.Model(&SchemaVersion{}).Where("id = 1").Update("checksum", "tampered")
				if s.Migrate() == nil {
					t.Fatal("tampered v2 checksum accepted")
				}
				s.DB.Model(&SchemaVersion{}).Where("id = 1").Update("checksum", migrationChecksum(2))
				if scenario == "audit-rollback" {
					if err := s.DB.Migrator().DropTable(&Event{}); err != nil {
						t.Fatal(err)
					}
					if s.Migrate() == nil {
						t.Fatal("migration accepted missing audit table")
					}
					var current Role
					if err := s.DB.First(&current, "id = ?", role.ID).Error; err != nil || !current.Builtin {
						t.Fatal("role changes committed without audit")
					}
					var version SchemaVersion
					s.DB.First(&version, 1)
					if version.Version != 2 {
						t.Fatal("failed migration advanced schema")
					}
					var count int64
					s.DB.Model(&UserRole{}).Where("role_id = ?", role.ID).Count(&count)
					if count != 1 {
						t.Fatal("failed migration removed membership")
					}
					return
				}
				if err := s.Migrate(); err != nil {
					t.Fatal(err)
				}
				if err := s.Migrate(); err != nil {
					t.Fatal(err)
				}
				if err := s.Health(context.Background()); err != nil {
					t.Fatal(err)
				}
				var current Role
				removed := scenario == "empty-viewer" || scenario == "unused-editor"
				err = s.DB.First(&current, "id = ?", role.ID).Error
				if removed {
					if err == nil {
						t.Fatal("unused legacy role retained")
					}
				} else if err != nil || current.Builtin {
					t.Fatal("assigned legacy/custom role not preserved as ordinary")
				}
				after, admin, err := permissions(s.DB, user)
				if err != nil || admin || len(after) != len(before) {
					t.Fatal("migration changed effective privileges")
				}
				for _, permission := range before {
					if !contains(after, permission) {
						t.Fatal("migration revoked a grant")
					}
				}
				var count int64
				s.DB.Model(&Event{}).Where("id = ?", "migration:003:custom_roles").Count(&count)
				if count != 1 {
					t.Fatal("migration audit absent or duplicated")
				}
				if removed {
					s.DB.Model(&UserRole{}).Where("role_id = ?", role.ID).Count(&count)
					if count != 0 {
						t.Fatal("legacy user link retained")
					}
					s.DB.Model(&GroupRole{}).Where("role_id = ?", role.ID).Count(&count)
					if count != 0 {
						t.Fatal("legacy group link retained")
					}
				}
			})
		}
	}
}
