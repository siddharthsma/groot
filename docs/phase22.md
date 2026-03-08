
# Groot — Phase 22

## Goal

Introduce Editions, Single-Tenant Community Mode, and Distribution Packaging.

Phase 22 enables three supported deployment modes of Groot from a single codebase:

1. Cloud Edition
   - multi-tenant
   - AWS SaaS deployment
   - full operator/admin capabilities

2. Community Edition
   - single tenant only
   - self-hosted
   - docker-compose deployment
   - no cross-tenant admin features

3. Internal Edition
   - multi-tenant
   - private deployments inside larger systems

This phase introduces edition enforcement, runtime configuration, and packaging.

No UI changes.

---

# Scope

Phase 22 implements:

1. Edition configuration model
2. Tenancy mode enforcement
3. Community edition single-tenant guardrails
4. Edition capability flags
5. Docker Compose distribution
6. Runtime edition detection
7. License banner and edition reporting
8. Packaging directories
9. Integration tests for edition behavior

---

# Edition Model

Add runtime configuration:

GROOT_EDITION=cloud | community | internal

Add tenancy mode:

GROOT_TENANCY_MODE=single | multi

Rules:

| Edition | Tenancy |
|-------|-------|
| cloud | multi |
| community | single |
| internal | multi |

If GROOT_EDITION=community and GROOT_TENANCY_MODE != single → startup failure.

---

# Runtime Edition Capabilities

Create internal package:

internal/edition

Define:

type Edition string

const (
    EditionCloud     Edition = "cloud"
    EditionCommunity Edition = "community"
    EditionInternal  Edition = "internal"
)

Capabilities struct:

type Capabilities struct {
    MultiTenant               bool
    CrossTenantAdmin          bool
    TenantCreationAllowed     bool
    HostedBillingEnabled      bool
    InternalRuntimeToolAccess bool
}

Capability mapping:

Cloud

MultiTenant = true
CrossTenantAdmin = true
TenantCreationAllowed = true
HostedBillingEnabled = true
InternalRuntimeToolAccess = true

Community

MultiTenant = false
CrossTenantAdmin = false
TenantCreationAllowed = false
HostedBillingEnabled = false
InternalRuntimeToolAccess = true

Internal

MultiTenant = true
CrossTenantAdmin = true
TenantCreationAllowed = true
HostedBillingEnabled = false
InternalRuntimeToolAccess = true

Expose globally:

edition.GetCapabilities()
edition.Current()

---

# Tenancy Enforcement

## Community Edition

Community mode must enforce exactly one tenant.

Rules:

- POST /tenants must return 403 in community edition
- bootstrap tenant must be created automatically on startup
- tenant id stored in config table

Add new env variable:

COMMUNITY_TENANT_NAME

Startup behavior:

1. if tenants table empty → create bootstrap tenant
2. if more than one tenant exists → startup failure
3. all tenant APIs implicitly operate on bootstrap tenant

---

# Tenant API Restrictions

Community edition disables:

POST /tenants
GET /tenants
GET /admin/tenants
PATCH /admin/tenants

Behavior:

return 403 community_edition_restriction

---

# Subscription Restrictions

Community edition does not change subscription behavior.

Subscriptions remain fully functional but scoped to the single tenant.

---

# Agent Restrictions

No feature removal.

Agents remain supported but must belong to the single tenant.

---

# Admin API Restrictions

Community edition must disable:

/admin/tenants
/admin/tenant-list
/admin cross-tenant queries

Other admin APIs remain available for the single tenant.

---

# Edition Reporting

Add endpoint:

GET /system/edition

Response:

{
  "edition": "community",
  "tenancy_mode": "single",
  "capabilities": {
    "multi_tenant": false,
    "cross_tenant_admin": false
  }
}

Purpose:

- debugging
- support
- packaging validation

---

# License Banner

On startup print:

Groot Community Edition
Single-tenant mode enabled
Commercial resale and SaaS hosting prohibited by license

For cloud/internal editions print appropriate banner.

---

# Docker Compose Distribution

Create directory:

deploy/docker-compose/community

Include:

docker-compose.yml
.env.example
README.md

Services:

postgres
kafka (or redpanda)
temporal
groot-api
groot-agent-runtime

Ports:

API: 8080
Agent runtime: 8090
Temporal UI: 8233

Environment defaults:

GROOT_EDITION=community
GROOT_TENANCY_MODE=single
AGENT_RUNTIME_ENABLED=true

Secrets configured via .env.

---

# Packaging Layout

Add directory structure:

/deploy
    /docker-compose
        /community
    /aws
        /cloud
/editions
    /community
    /cloud
    /internal
/licenses

Files:

LICENSE
COMMUNITY_LICENSE.md

---

# Startup Validation

At boot:

1. Load edition
2. Validate edition + tenancy compatibility
3. Initialize capability map
4. Enforce tenant limits
5. Print edition banner

Fatal conditions:

- community edition with >1 tenant
- unsupported edition value

---

# Integration Tests

Create test file:

tests/integration/phase22_editions_test.go

---

## Test 1 — Community tenant restriction

Scenario:

1. start server with

GROOT_EDITION=community

2. call POST /tenants

Expected:

403 community_edition_restriction

---

## Test 2 — Community bootstrap tenant

Scenario:

1. start server with empty DB
2. community edition

Verify:

- exactly one tenant created
- tenant name matches COMMUNITY_TENANT_NAME

---

## Test 3 — Community multi-tenant prevention

Scenario:

1. manually insert second tenant in DB
2. start server

Expected:

startup failure.

---

## Test 4 — Internal edition multi-tenant

Scenario:

1. start with

GROOT_EDITION=internal

2. create multiple tenants

Expected:

success.

---

## Test 5 — Edition endpoint

Scenario:

GET /system/edition

Verify:

- edition correct
- capability flags correct

---

# Observability

Add startup log fields:

edition
tenancy_mode
capabilities

Metric:

groot_edition_info{edition="community"}

---

# Documentation

Update:

README.md
AGENTS.md

Add sections:

- Deployment Modes
- Community Edition Setup
- Docker Compose Quickstart
- Edition Environment Variables

---

# Phase 22 Completion Criteria

All conditions must be met:

- edition configuration exists
- community edition enforces single tenant
- tenant creation blocked in community edition
- internal/cloud allow multi-tenant
- edition capability system implemented
- docker-compose community deployment works
- edition endpoint reports correct values
- startup validation prevents illegal configurations
- integration tests confirm edition restrictions
