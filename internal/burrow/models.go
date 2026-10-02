package burrow

import "time"

type User struct {
	ID                 string     `gorm:"primaryKey" json:"id"`
	Username           string     `gorm:"uniqueIndex;not null" json:"username"`
	Name               string     `json:"name"`
	Email              string     `json:"email"`
	Enabled            bool       `json:"enabled"`
	PasswordHash       string     `json:"-"`
	MustChangePassword bool       `json:"mustChangePassword"`
	MFAEnabled         bool       `json:"mfaEnabled"`
	MFACipher          string     `json:"-"`
	MFALastStep        int64      `json:"-"`
	AuthVersion        int64      `json:"-"`
	Language           string     `json:"language"`
	Theme              string     `json:"theme"`
	RoleIDs            []string   `gorm:"-" json:"roleIds"`
	GroupIDs           []string   `gorm:"-" json:"groupIds"`
	Groups             []GroupRef `gorm:"-" json:"groups"`
}
type GroupRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type Group struct {
	ID          string   `gorm:"primaryKey" json:"id"`
	Name        string   `gorm:"uniqueIndex;not null" json:"name"`
	Description string   `json:"description"`
	UserIDs     []string `gorm:"-" json:"userIds"`
	RoleIDs     []string `gorm:"-" json:"roleIds"`
}
type Role struct {
	ID            string   `gorm:"primaryKey" json:"id"`
	Name          string   `gorm:"uniqueIndex;not null" json:"name"`
	Description   string   `json:"description"`
	Builtin       bool     `json:"builtin"`
	PermissionIDs []string `gorm:"-" json:"permissionIds"`
}
type Permission struct {
	ID            string          `gorm:"primaryKey" json:"id"`
	Name          string          `gorm:"uniqueIndex;not null" json:"name"`
	Description   string          `json:"description"`
	ApplicationID string          `json:"applicationId"`
	Application   *ApplicationRef `gorm:"-" json:"application,omitempty"`
}
type ApplicationRef struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ClientID string `json:"clientId"`
}
type UserRole struct {
	UserID string `gorm:"primaryKey"`
	RoleID string `gorm:"primaryKey"`
}
type GroupRole struct {
	GroupID string `gorm:"primaryKey"`
	RoleID  string `gorm:"primaryKey"`
}
type GroupMember struct {
	GroupID string `gorm:"primaryKey"`
	UserID  string `gorm:"primaryKey"`
}
type RolePermission struct {
	RoleID       string `gorm:"primaryKey"`
	PermissionID string `gorm:"primaryKey"`
}
type Application struct {
	ID               string   `gorm:"primaryKey" json:"id"`
	Name             string   `json:"name"`
	ClientID         string   `gorm:"uniqueIndex;not null" json:"clientId"`
	ClientType       string   `json:"clientType"`
	SecretHash       string   `json:"-"`
	Enabled          bool     `json:"enabled"`
	Icon             string   `json:"icon"`
	LoginURL         string   `json:"loginUrl"`
	RedirectURLs     []string `gorm:"serializer:json" json:"redirectUris"`
	LogoutURLs       []string `gorm:"serializer:json" json:"postLogoutRedirectUris"`
	Origins          []string `gorm:"serializer:json" json:"origins"`
	AllowWithoutPKCE bool     `json:"allowWithoutPkce"`
	RoleIDs          []string `gorm:"-" json:"roleIds"`
}
type Session struct {
	ID             string `gorm:"primaryKey"`
	UserID         string `gorm:"index"`
	CredentialHash string `gorm:"uniqueIndex"`
	Method         string
	AuthTime       time.Time
	MFAAt          time.Time
	AuthVersion    int64
	ExpiresAt      time.Time `gorm:"index"`
	Revoked        bool
}
type AuthTransaction struct {
	ID            string `gorm:"primaryKey"`
	Payload       string
	BrowserHash   string
	SessionID     string
	UserID        string
	ClientID      string
	CodeHash      *string `gorm:"uniqueIndex"`
	CodeExpiresAt time.Time
	ExpiresAt     time.Time `gorm:"index"`
	Consumed      bool
	AuthTime      time.Time
	Method        string
}

// LoginTransaction grants access only to the remaining authentication steps.
// Neither its credential nor its encrypted pending secret is a shared session.
type LoginTransaction struct {
	ID             string `gorm:"primaryKey"`
	CredentialHash string `gorm:"uniqueIndex"`
	BrowserHash    string
	UserID         string `gorm:"index"`
	AuthVersion    int64
	RequestID      string
	PendingCipher  string
	MFAVerified    bool
	MFAAt          time.Time
	Attempts       int
	ExpiresAt      time.Time `gorm:"index"`
}
type TokenRecord struct {
	ID        string `gorm:"primaryKey"`
	UserID    string
	ClientID  string
	SessionID string
	Scopes    []string  `gorm:"serializer:json"`
	ExpiresAt time.Time `gorm:"index"`
	Revoked   bool
}
type SigningKey struct {
	ID            string `gorm:"primaryKey"`
	PrivateCipher string
	PublicJSON    string
	Active        bool
	CreatedAt     time.Time
}
type Event struct {
	ID        string `gorm:"primaryKey"`
	ActorID   string
	ObjectID  string
	Kind      string `gorm:"index"`
	Success   bool
	RequestID string
	Details   string
	CreatedAt time.Time `gorm:"index"`
}
type SchemaVersion struct {
	ID       int `gorm:"primaryKey"`
	Version  int
	Checksum string
}
