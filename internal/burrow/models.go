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
	ID           string `json:"id"`
	Name         string `json:"name"`
	ClientID     string `json:"clientId"`
	Protocol     string `json:"protocol"`
	SAMLEntityID string `gorm:"column:saml_entity_id" json:"samlEntityId,omitempty"`
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
	ID               string           `gorm:"primaryKey" json:"id"`
	Protocol         string           `gorm:"not null;default:oidc" json:"protocol"`
	Name             string           `json:"name"`
	ClientID         string           `gorm:"uniqueIndex;not null" json:"clientId,omitempty"`
	ClientType       string           `json:"clientType"`
	SecretHash       string           `json:"-"`
	Enabled          bool             `json:"enabled"`
	Icon             string           `json:"icon"`
	LoginURL         string           `json:"loginUrl"`
	RedirectURLs     []string         `gorm:"serializer:json" json:"redirectUris"`
	LogoutURLs       []string         `gorm:"serializer:json" json:"postLogoutRedirectUris"`
	Origins          []string         `gorm:"serializer:json" json:"origins"`
	AllowWithoutPKCE bool             `json:"allowWithoutPkce"`
	RoleIDs          []string         `gorm:"-" json:"roleIds"`
	SAML             *SAMLApplication `gorm:"-" json:"saml,omitempty"`
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
	UTCTimestamps  bool `gorm:"column:utc_timestamps"`
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
	RequestType    string `gorm:"not null;default:oidc"`
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

type SAMLApplication struct {
	ApplicationID      string `gorm:"primaryKey" json:"-"`
	EntityID           string `gorm:"uniqueIndex;not null" json:"entityId"`
	ACSURL             string `gorm:"column:acs_url" json:"acsUrl"`
	SigningCertificate string `json:"signingCertificate"`
	MetadataXML        string `json:"metadataXml"`
	ConfigVersion      int64  `json:"configVersion"`
}

type SAMLTransaction struct {
	ID            string `gorm:"primaryKey"`
	ApplicationID string
	SPRequestID   string `gorm:"column:sp_request_id"`
	BrowserHash   string
	RelayState    string
	ACSURL        string `gorm:"column:acs_url"`
	ConfigVersion int64
	ForceAuthn    bool
	FailureStatus string
	ReceivedAt    time.Time
	ExpiresAt     time.Time
	Consumed      bool
	UserID        string
	SessionID     string
}

// Replay guards deliberately have no application/user foreign key: deletion or
// cancellation must never make an already accepted external request reusable.
type SAMLRequestReplay struct {
	SPEntityID  string `gorm:"primaryKey;column:sp_entity_id"`
	SPRequestID string `gorm:"primaryKey;column:sp_request_id"`
	ReplayUntil time.Time
}

type SAMLSigningKey struct {
	ID             string `gorm:"primaryKey"`
	PrivateCipher  string `json:"-"`
	CertificatePEM string `gorm:"column:certificate_pem"`
	Active         bool
	CreatedAt      time.Time
}

func (SAMLApplication) TableName() string   { return "saml_applications" }
func (SAMLTransaction) TableName() string   { return "saml_transactions" }
func (SAMLRequestReplay) TableName() string { return "saml_request_replays" }
func (SAMLSigningKey) TableName() string    { return "saml_signing_keys" }
