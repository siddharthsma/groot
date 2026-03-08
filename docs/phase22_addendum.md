
# Groot — Phase 22 Addendum

## Goal

Strengthen Phase 22 so edition selection is not controlled solely by `.env`.

This addendum introduces:

1. build-time edition locking
2. optional signed license validation
3. runtime config precedence rules

The objective is:

- `.env` may configure runtime behavior
- `.env` may not elevate edition capabilities
- official builds enforce their edition even if runtime config is modified

No UI.

---

# Scope

This addendum implements:

1. Build-time edition embedding
2. Runtime startup validation against build edition
3. Optional signed license file support
4. Capability derivation from build edition + license
5. Community/community-like protections against edition escalation
6. Packaging updates for official edition-specific builds
7. Tests for edition override rejection and license validation

---

# Edition Precedence Model

Edition/capability resolution must follow this order:

1. Build Edition (required, source of truth)
2. License Claims (optional, may further restrict)
3. Runtime Config (.env)

Rule:

Runtime config may narrow behavior.
Runtime config may not elevate behavior.

Examples:

- community build + `GROOT_TENANCY_MODE=multi` → startup failure
- community build + license says `max_tenants=1` → valid
- cloud build + license says `max_tenants=1` → valid but restricted
- cloud build + missing/invalid required license (if license enforcement enabled) → startup failure

---

# Build-Time Edition Locking

Add build-time variable embedded into the binary.

Example variable:

```go
var BuildEdition = "community"
```

Valid values:

- community
- cloud
- internal

This value must be set via build flags in official builds.

Example build approach:

```
-ldflags "-X main.BuildEdition=community"
```

Rules:

- if `BuildEdition` empty or invalid → startup failure
- `GROOT_EDITION` in `.env` may still exist for diagnostics/dev tooling, but:
  - it must match `BuildEdition`
  - mismatch → startup failure

Recommended behavior:

- deprecate runtime edition override
- keep `GROOT_EDITION` only as optional assertion / diagnostic echo

---

# License File Support

Add env vars:

- `GROOT_LICENSE_PATH=` optional
- `GROOT_LICENSE_REQUIRED=false`
- `GROOT_LICENSE_PUBLIC_KEY_PATH=` optional if not embedded
- `GROOT_LICENSE_ENFORCE_SIGNATURE=true`

Rules:

- if `GROOT_LICENSE_REQUIRED=true` and no license file present → startup failure
- if license file present and signature invalid → startup failure
- if no license file present and not required → system runs using build edition defaults only

---

# License Claims

License payload format (JSON before signing):

```json
{
  "license_id": "uuid",
  "edition": "community",
  "licensee": "Acme Ltd",
  "tenancy_mode": "single",
  "max_tenants": 1,
  "features": [
    "core",
    "agents",
    "schemas",
    "replay"
  ],
  "expires_at": "optional RFC3339 timestamp"
}
```

Required claims:

- `edition`
- `max_tenants`

Optional claims:

- `licensee`
- `tenancy_mode`
- `features`
- `expires_at`

Rules:

- license `edition` must equal `BuildEdition`
- if mismatch → startup failure
- expired license → startup failure
- `max_tenants` may restrict behavior further than build defaults
- `features` may further restrict optional capabilities

---

# Signature Verification

Use asymmetric signing.

Recommended model:

- private key held by publisher
- public key embedded in binary or loaded from `GROOT_LICENSE_PUBLIC_KEY_PATH`

Startup verification steps:

1. read license file
2. parse signed structure
3. verify signature
4. parse claims
5. validate edition compatibility
6. validate expiration
7. apply restrictions

Do not use a symmetric shared secret for license signing.

---

# Effective Capability Resolution

Capabilities are computed as:

effective_capabilities =
    intersection(build_edition_capabilities, license_capabilities, runtime_requested_capabilities)

Meaning:

- build edition sets the upper bound
- license may reduce
- runtime config may reduce
- neither license nor runtime may exceed build edition

---

# Community Edition Enforcement

For official community builds:

- `BuildEdition=community`
- community binary must never run in multi-tenant mode
- if runtime requests:
  - `GROOT_TENANCY_MODE=multi`
  - `CrossTenantAdmin=true`
  - more than one tenant
  then startup fails

If a license file is present for community:

- it must also specify:
  - `edition=community`
  - `max_tenants=1`

---

# Internal / Cloud Enforcement

For official internal/cloud builds:

- `BuildEdition=internal` or `cloud`
- runtime may use `single` or `multi` only if allowed by build/license
- if license says `max_tenants=1`, system must enforce single-tenant behavior even on a cloud/internal build

---

# Startup Validation Changes

Extend Phase 22 startup validation:

1. Load `BuildEdition`
2. Load runtime config
3. If `GROOT_EDITION` is set and differs from `BuildEdition` → fail
4. Load and verify license if present/required
5. Compute effective capabilities
6. Validate tenancy mode against effective capabilities
7. Validate current DB tenant count against effective `max_tenants`
8. Print startup banner with:
   - build edition
   - licensee
   - effective tenancy mode
   - max tenants

---

# System Edition Endpoint Changes

Extend:

GET /system/edition

Response:

```json
{
  "build_edition": "community",
  "effective_edition": "community",
  "tenancy_mode": "single",
  "license": {
    "present": true,
    "licensee": "Acme Ltd",
    "max_tenants": 1,
    "expires_at": null
  },
  "capabilities": {
    "multi_tenant": false,
    "cross_tenant_admin": false
  }
}
```

Rules:

- endpoint remains unauthenticated
- do not expose raw license contents or signature
- only expose safe diagnostic metadata

---

# Packaging Changes

Official packaging must produce separate artifacts:

## Community

Artifact names:

- `groot-community`
- `groot-community:<version>` container image

Build flags:

- `BuildEdition=community`

Include:

- community compose bundle
- community license file if applicable

## Cloud

Artifact names:

- `groot-cloud`
- `groot-cloud:<version>`

Build flags:

- `BuildEdition=cloud`

## Internal

Artifact names:

- `groot-internal`
- `groot-internal:<version>`

Build flags:

- `BuildEdition=internal`

Rules:

- official release pipeline must not publish a generic unrestricted binary

---

# Repository / Build Layout

Add build/release helpers:

```
/build
  /community
  /cloud
  /internal
/scripts
  build-community.sh
  build-cloud.sh
  build-internal.sh
```

Optional:

```
/licenses/examples
  community.lic.example
```

---

# Logging / Observability

Add startup log fields:

- `build_edition`
- `license_present`
- `license_valid`
- `licensee`
- `effective_max_tenants`

Metric:

```
groot_license_info{edition="community",license_present="true"}
```

Do not log:

- raw license payload
- signatures
- keys

---

# Integration Tests

Add to:

```
tests/integration/phase22_editions_test.go
```

## Test 6 — Runtime override rejected

Scenario:

1. build/test binary with `BuildEdition=community`
2. set `GROOT_EDITION=cloud`
3. start server

Expected:

- startup failure

---

## Test 7 — Community multi-tenant override rejected

Scenario:

1. build edition = community
2. set `GROOT_TENANCY_MODE=multi`

Expected:

- startup failure

---

## Test 8 — Valid signed license accepted

Scenario:

1. provide valid signed community license
2. start server

Verify:

- startup succeeds
- `/system/edition` reports license present and correct max_tenants

---

## Test 9 — Invalid signature rejected

Scenario:

1. provide tampered license file

Expected:

- startup failure

---

## Test 10 — License/build mismatch rejected

Scenario:

1. build edition = community
2. license edition = cloud

Expected:

- startup failure

---

## Test 11 — Max tenant restriction enforced

Scenario:

1. internal build
2. valid license with `max_tenants=1`
3. database contains two tenants

Expected:

- startup failure

---

# Documentation

Update:

- `README.md`
- `AGENTS.md`
- deployment docs

Add sections:

- Edition Locking
- License Validation
- Build Artifacts
- Runtime Config Precedence

State clearly:

`.env does not control edition trust boundaries`

---

# Phase 22 Addendum Completion Criteria

All conditions must be met:

- build edition is embedded and required at startup
- runtime config cannot override build edition
- optional signed license validation is implemented
- effective capabilities are derived from build + license + runtime restrictions
- `/system/edition` reports safe diagnostic edition/license metadata
- official packaging produces separate edition-specific artifacts
- tests validate override rejection, valid license acceptance, invalid signature rejection, and tenant-limit enforcement
