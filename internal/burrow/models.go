package burrow

import "time"

type User struct {
	ID                 string     `gorm:"primaryKey" json:"id"`
	Username           string     `gorm:"uniqueIndex;not null" json:"username"`
	Name               string     `json:"name"`
	Email              string     `json:"email"`
	Enabled            bool       `json:"enabled"`
	LocalEnabled       bool       `json:"localEnabled"`
	PasswordHash       string     `json:"-"`
	MustChangePassword bool       `json:"mustChangePassword"`
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
	ID            string `gorm:"primaryKey" json:"id"`
	Name          string `gorm:"uniqueIndex;not null" json:"name"`
	Description   string `json:"description"`
	ApplicationID string `json:"applicationId"`
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
type ApplicationProvider struct {
	ApplicationID string `gorm:"primaryKey"`
	ProviderID    string `gorm:"primaryKey"`
}
type Application struct {
	ID           string   `gorm:"primaryKey" json:"id"`
	Name         string   `json:"name"`
	ClientID     string   `gorm:"uniqueIndex;not null" json:"clientId"`
	ClientType   string   `json:"clientType"`
	SecretHash   string   `json:"-"`
	Enabled      bool     `json:"enabled"`
	Icon         string   `json:"icon"`
	LoginURL     string   `json:"loginUrl"`
	RedirectURLs []string `gorm:"serializer:json" json:"redirectUris"`
	LogoutURLs   []string `gorm:"serializer:json" json:"postLogoutRedirectUris"`
	Origins      []string `gorm:"serializer:json" json:"origins"`
	LocalEnabled bool     `json:"localEnabled"`
	ProviderIDs  []string `gorm:"-" json:"providerIds"`
	RoleIDs      []string `gorm:"-" json:"roleIds"`
}
type Provider struct {
	ID           string `gorm:"primaryKey" json:"id"`
	Name         string `json:"name"`
	Issuer       string `json:"issuer"`
	ClientID     string `json:"clientId"`
	SecretCipher string `json:"-"`
	Enabled      bool   `json:"enabled"`
}
type ExternalIdentity struct {
	ID         string `gorm:"primaryKey" json:"id"`
	ProviderID string `gorm:"uniqueIndex:external_subject" json:"providerId"`
	Issuer     string `gorm:"uniqueIndex:external_subject" json:"issuer"`
	Subject    string `gorm:"uniqueIndex:external_subject" json:"subject"`
	UserID     string `gorm:"index" json:"userId"`
}
type Session struct {
	ID             string `gorm:"primaryKey"`
	UserID         string `gorm:"index"`
	CredentialHash string `gorm:"uniqueIndex"`
	Method         string
	ProviderID     string
	AuthTime       time.Time
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
type UpstreamTransaction struct {
	ID          string `gorm:"primaryKey"`
	ProviderID  string
	RequestID   string
	BrowserHash string
	Nonce       string
	Verifier    string
	ExpiresAt   time.Time `gorm:"index"`
	Consumed    bool
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
	CreatedAt time.Time `gorm:"index"`
}
type SchemaVersion struct {
	ID       int `gorm:"primaryKey"`
	Version  int
	Checksum string
}
