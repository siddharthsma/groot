package edition

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"groot/internal/tenant"
)

const CommunityBootstrapTenantSettingKey = "community_bootstrap_tenant_id"

type Edition string
type TenancyMode string

const (
	EditionCloud     Edition = "cloud"
	EditionCommunity Edition = "community"
	EditionInternal  Edition = "internal"
)

const (
	TenancySingle TenancyMode = "single"
	TenancyMulti  TenancyMode = "multi"
)

type Capabilities struct {
	MultiTenant               bool `json:"multi_tenant"`
	CrossTenantAdmin          bool `json:"cross_tenant_admin"`
	TenantCreationAllowed     bool `json:"tenant_creation_allowed"`
	HostedBillingEnabled      bool `json:"hosted_billing_enabled"`
	InternalRuntimeToolAccess bool `json:"internal_runtime_tool_access"`
}

type LicenseConfig struct {
	Path             string
	Required         bool
	PublicKeyPath    string
	EnforceSignature bool
}

type LicenseEnvelope struct {
	Payload   json.RawMessage `json:"payload"`
	Signature string          `json:"signature"`
}

type LicenseClaims struct {
	LicenseID   string      `json:"license_id"`
	Edition     Edition     `json:"edition"`
	Licensee    string      `json:"licensee,omitempty"`
	TenancyMode TenancyMode `json:"tenancy_mode,omitempty"`
	MaxTenants  int         `json:"max_tenants"`
	Features    []string    `json:"features,omitempty"`
	ExpiresAt   *time.Time  `json:"expires_at,omitempty"`
}

type LicenseState struct {
	Present    bool       `json:"present"`
	Valid      bool       `json:"valid"`
	Licensee   string     `json:"licensee,omitempty"`
	MaxTenants int        `json:"max_tenants,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
}

type Runtime struct {
	BuildEdition     Edition      `json:"build_edition"`
	EffectiveEdition Edition      `json:"effective_edition"`
	TenancyMode      TenancyMode  `json:"tenancy_mode"`
	Capabilities     Capabilities `json:"capabilities"`
	MaxTenants       int          `json:"-"`
	License          LicenseState `json:"license"`
}

type TenantManager interface {
	CreateTenant(context.Context, string) (tenant.CreatedTenant, error)
	ListTenants(context.Context) ([]tenant.Tenant, error)
}

type SettingsStore interface {
	GetSystemSetting(context.Context, string) (string, error)
	UpsertSystemSetting(context.Context, string, string) error
}

var (
	ErrUnsupportedEditionValue = errors.New("unsupported edition value")
	ErrUnsupportedTenancyMode  = errors.New("unsupported tenancy mode")
	ErrInvalidEditionTenancy   = errors.New("invalid edition tenancy combination")
	ErrCommunityTenantLimit    = errors.New("community edition requires exactly one tenant")
	ErrTenantLimitExceeded     = errors.New("tenant limit exceeded")

	mu      sync.RWMutex
	current Runtime
)

func Parse(rawEdition, rawTenancy string) (Runtime, error) {
	parsedEdition := Edition(strings.TrimSpace(strings.ToLower(rawEdition)))
	if parsedEdition == "" {
		parsedEdition = EditionInternal
	}
	parsedTenancy := TenancyMode(strings.TrimSpace(strings.ToLower(rawTenancy)))
	if parsedTenancy == "" {
		parsedTenancy = defaultTenancyForEdition(parsedEdition)
	}
	switch parsedEdition {
	case EditionCloud, EditionCommunity, EditionInternal:
	default:
		return Runtime{}, fmt.Errorf("%w: %s", ErrUnsupportedEditionValue, rawEdition)
	}
	switch parsedTenancy {
	case TenancySingle, TenancyMulti:
	default:
		return Runtime{}, fmt.Errorf("%w: %s", ErrUnsupportedTenancyMode, rawTenancy)
	}
	if parsedEdition == EditionCommunity && parsedTenancy != TenancySingle {
		return Runtime{}, fmt.Errorf("%w: edition=%s tenancy_mode=%s", ErrInvalidEditionTenancy, parsedEdition, parsedTenancy)
	}
	return Runtime{
		BuildEdition:     parsedEdition,
		EffectiveEdition: parsedEdition,
		TenancyMode:      parsedTenancy,
		Capabilities:     capabilitiesForEdition(parsedEdition),
		MaxTenants:       defaultMaxTenantsForEdition(parsedEdition),
	}, nil
}

func Resolve(buildEdition string, runtimeRequested Runtime, licenseCfg LicenseConfig, embeddedPublicKeyBase64 string) (Runtime, error) {
	parsedBuild := Edition(strings.TrimSpace(strings.ToLower(buildEdition)))
	switch parsedBuild {
	case EditionCommunity, EditionCloud, EditionInternal:
	default:
		return Runtime{}, fmt.Errorf("%w: %s", ErrUnsupportedEditionValue, buildEdition)
	}
	if runtimeRequested.BuildEdition != "" && runtimeRequested.BuildEdition != parsedBuild {
		return Runtime{}, fmt.Errorf("runtime edition %s does not match build edition %s", runtimeRequested.BuildEdition, parsedBuild)
	}
	requestedTenancy := runtimeRequested.TenancyMode
	if requestedTenancy == "" {
		requestedTenancy = defaultTenancyForEdition(parsedBuild)
	}
	switch requestedTenancy {
	case TenancySingle, TenancyMulti:
	default:
		return Runtime{}, fmt.Errorf("%w: %s", ErrUnsupportedTenancyMode, requestedTenancy)
	}

	licenseState, claims, err := loadLicense(licenseCfg, embeddedPublicKeyBase64)
	if err != nil {
		return Runtime{}, err
	}
	if claims != nil {
		if claims.Edition != parsedBuild {
			return Runtime{}, fmt.Errorf("license edition %s does not match build edition %s", claims.Edition, parsedBuild)
		}
		if claims.ExpiresAt != nil && time.Now().UTC().After(claims.ExpiresAt.UTC()) {
			return Runtime{}, fmt.Errorf("license expired at %s", claims.ExpiresAt.UTC().Format(time.RFC3339))
		}
	}

	maxTenants := defaultMaxTenantsForEdition(parsedBuild)
	if claims != nil && claims.MaxTenants > 0 && (maxTenants == 0 || claims.MaxTenants < maxTenants) {
		maxTenants = claims.MaxTenants
	}
	if claims != nil && claims.TenancyMode == TenancySingle {
		maxTenants = forceSingleTenantCap(maxTenants)
	}
	if requestedTenancy == TenancySingle {
		maxTenants = forceSingleTenantCap(maxTenants)
	}
	if parsedBuild == EditionCommunity {
		maxTenants = 1
	}
	effectiveTenancy := requestedTenancy
	if maxTenants == 1 {
		effectiveTenancy = TenancySingle
	}

	caps := capabilitiesForEdition(parsedBuild)
	if maxTenants == 1 {
		caps.MultiTenant = false
		caps.CrossTenantAdmin = false
	}
	return Runtime{
		BuildEdition:     parsedBuild,
		EffectiveEdition: parsedBuild,
		TenancyMode:      effectiveTenancy,
		Capabilities:     caps,
		MaxTenants:       maxTenants,
		License:          licenseState,
	}, nil
}

func SetRuntime(runtime Runtime) {
	mu.Lock()
	current = runtime
	mu.Unlock()
}

func Current() Edition {
	mu.RLock()
	defer mu.RUnlock()
	return current.EffectiveEdition
}

func CurrentRuntime() Runtime {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

func GetCapabilities() Capabilities {
	mu.RLock()
	defer mu.RUnlock()
	return current.Capabilities
}

func (r Runtime) IsCommunity() bool {
	return r.EffectiveEdition == EditionCommunity
}

func (r Runtime) BannerLines() []string {
	switch r.EffectiveEdition {
	case EditionCommunity:
		return []string{
			"Groot Community Edition",
			"Single-tenant mode enabled",
			"Commercial resale and SaaS hosting prohibited by license",
		}
	case EditionCloud:
		return []string{
			"Groot Cloud Edition",
			"Multi-tenant mode enabled",
			"Hosted billing and operator capabilities enabled",
		}
	default:
		return []string{
			"Groot Internal Edition",
			"Multi-tenant mode enabled",
			"Private deployment capabilities enabled",
		}
	}
}

func EnsureCommunityTenant(ctx context.Context, runtime Runtime, tenants TenantManager, settings SettingsStore, tenantName string) (tenant.Tenant, bool, error) {
	if !runtime.IsCommunity() {
		return tenant.Tenant{}, false, nil
	}
	name := strings.TrimSpace(tenantName)
	if name == "" {
		return tenant.Tenant{}, false, fmt.Errorf("COMMUNITY_TENANT_NAME is required in community edition")
	}
	records, err := tenants.ListTenants(ctx)
	if err != nil {
		return tenant.Tenant{}, false, fmt.Errorf("list tenants: %w", err)
	}
	switch len(records) {
	case 0:
		created, err := tenants.CreateTenant(ctx, name)
		if err != nil {
			return tenant.Tenant{}, false, fmt.Errorf("create community bootstrap tenant: %w", err)
		}
		if err := settings.UpsertSystemSetting(ctx, CommunityBootstrapTenantSettingKey, created.Tenant.ID.String()); err != nil {
			return tenant.Tenant{}, false, fmt.Errorf("store community bootstrap tenant id: %w", err)
		}
		return created.Tenant, true, nil
	case 1:
		record := records[0]
		if err := settings.UpsertSystemSetting(ctx, CommunityBootstrapTenantSettingKey, record.ID.String()); err != nil {
			return tenant.Tenant{}, false, fmt.Errorf("store community bootstrap tenant id: %w", err)
		}
		return record, false, nil
	default:
		return tenant.Tenant{}, false, ErrCommunityTenantLimit
	}
}

func LoadBootstrapTenantID(ctx context.Context, runtime Runtime, settings SettingsStore) (*uuid.UUID, error) {
	if !runtime.IsCommunity() {
		return nil, nil
	}
	value, err := settings.GetSystemSetting(ctx, CommunityBootstrapTenantSettingKey)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get community bootstrap tenant id: %w", err)
	}
	parsed, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return nil, fmt.Errorf("parse community bootstrap tenant id: %w", err)
	}
	return &parsed, nil
}

func ValidateTenantCount(runtime Runtime, count int) error {
	if runtime.MaxTenants > 0 && count > runtime.MaxTenants {
		return fmt.Errorf("%w: max_tenants=%d current=%d", ErrTenantLimitExceeded, runtime.MaxTenants, count)
	}
	return nil
}

func defaultTenancyForEdition(value Edition) TenancyMode {
	switch value {
	case EditionCommunity:
		return TenancySingle
	default:
		return TenancyMulti
	}
}

func defaultMaxTenantsForEdition(value Edition) int {
	if value == EditionCommunity {
		return 1
	}
	return 0
}

func forceSingleTenantCap(current int) int {
	if current == 0 || current > 1 {
		return 1
	}
	return current
}

func capabilitiesForEdition(value Edition) Capabilities {
	switch value {
	case EditionCloud:
		return Capabilities{
			MultiTenant:               true,
			CrossTenantAdmin:          true,
			TenantCreationAllowed:     true,
			HostedBillingEnabled:      true,
			InternalRuntimeToolAccess: true,
		}
	case EditionCommunity:
		return Capabilities{
			MultiTenant:               false,
			CrossTenantAdmin:          false,
			TenantCreationAllowed:     false,
			HostedBillingEnabled:      false,
			InternalRuntimeToolAccess: true,
		}
	default:
		return Capabilities{
			MultiTenant:               true,
			CrossTenantAdmin:          true,
			TenantCreationAllowed:     true,
			HostedBillingEnabled:      false,
			InternalRuntimeToolAccess: true,
		}
	}
}

func loadLicense(cfg LicenseConfig, embeddedPublicKeyBase64 string) (LicenseState, *LicenseClaims, error) {
	path := strings.TrimSpace(cfg.Path)
	if path == "" {
		if cfg.Required {
			return LicenseState{}, nil, fmt.Errorf("license file is required")
		}
		return LicenseState{}, nil, nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return LicenseState{}, nil, fmt.Errorf("read license file: %w", err)
	}
	var envelope LicenseEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return LicenseState{}, nil, fmt.Errorf("decode license file: %w", err)
	}
	if len(envelope.Payload) == 0 {
		return LicenseState{}, nil, fmt.Errorf("license payload is required")
	}
	if cfg.EnforceSignature {
		publicKey, err := loadPublicKey(strings.TrimSpace(cfg.PublicKeyPath), embeddedPublicKeyBase64)
		if err != nil {
			return LicenseState{}, nil, err
		}
		signature, err := base64.StdEncoding.DecodeString(strings.TrimSpace(envelope.Signature))
		if err != nil {
			return LicenseState{}, nil, fmt.Errorf("decode license signature: %w", err)
		}
		if !ed25519.Verify(publicKey, canonicalizePayload(envelope.Payload), signature) {
			return LicenseState{}, nil, fmt.Errorf("license signature verification failed")
		}
	}

	var claims LicenseClaims
	if err := json.Unmarshal(envelope.Payload, &claims); err != nil {
		return LicenseState{}, nil, fmt.Errorf("decode license claims: %w", err)
	}
	if claims.Edition == "" {
		return LicenseState{}, nil, fmt.Errorf("license edition is required")
	}
	if claims.MaxTenants < 1 {
		return LicenseState{}, nil, fmt.Errorf("license max_tenants must be at least 1")
	}
	state := LicenseState{
		Present:    true,
		Valid:      true,
		Licensee:   strings.TrimSpace(claims.Licensee),
		MaxTenants: claims.MaxTenants,
		ExpiresAt:  claims.ExpiresAt,
	}
	return state, &claims, nil
}

func loadPublicKey(path string, embeddedBase64 string) (ed25519.PublicKey, error) {
	var encoded string
	if path != "" {
		body, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read license public key: %w", err)
		}
		encoded = strings.TrimSpace(string(body))
	} else {
		encoded = strings.TrimSpace(embeddedBase64)
	}
	if encoded == "" {
		return nil, fmt.Errorf("license public key is required")
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode license public key: %w", err)
	}
	if len(decoded) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("license public key must be %d bytes", ed25519.PublicKeySize)
	}
	return ed25519.PublicKey(decoded), nil
}

func canonicalizePayload(payload json.RawMessage) []byte {
	var parsed any
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return payload
	}
	body, err := json.Marshal(parsed)
	if err != nil {
		return payload
	}
	return body
}
