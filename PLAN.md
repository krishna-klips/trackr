# TRACKR - Enterprise Link Management Platform

## PROJECT SPECIFICATION & IMPLEMENTATION PLAN

---

## 1. SYSTEM ARCHITECTURE OVERVIEW

### 1.1 Core Architecture Pattern
```
Request Flow:
1. Caddy (TLS Termination) → Go HTTP Server
2. Auth Middleware → Turso (Global DB) for user validation
3. Tenant Middleware → SQLite (Local) for org-specific data
4. Handler Execution → Response

Redirect Flow:
1. /:shortcode → In-Memory Cache Check
2. Cache Miss → Local SQLite Query
3. Rules Evaluation → Device/Geo/Time checks
4. 301/302 Redirect + Async Click Logging
```

### 1.2 Data Isolation Strategy
- **Global Turso DB**: Cross-tenant data (auth, billing, SAML)
- **Local SQLite Files**: One `.db` file per organization (`org_<uuid>.db`)
- **Cache Layer**: Redis-compatible in-memory cache for hot links
- **No Cross-Tenant Queries**: Middleware enforces strict tenant scoping

---

## 2. DATABASE SCHEMAS

### 2.1 GLOBAL DATABASE (Turso/LibSQL)

#### Table: `organizations`
```sql
CREATE TABLE organizations (
    id TEXT PRIMARY KEY, -- UUID v7
    slug TEXT UNIQUE NOT NULL, -- trackr-corp
    name TEXT NOT NULL,
    domain TEXT UNIQUE NOT NULL, -- trackr.io (for auto-assignment)
    db_file_path TEXT NOT NULL, -- /data/dbs/org_<uuid>.db
    plan_tier TEXT DEFAULT 'enterprise', -- enterprise, team
    link_quota INTEGER DEFAULT 50000,
    member_quota INTEGER DEFAULT 100,
    saml_enabled BOOLEAN DEFAULT FALSE,
    webhook_secret TEXT, -- For signature verification
    created_at INTEGER NOT NULL, -- Unix timestamp
    updated_at INTEGER NOT NULL,
    deleted_at INTEGER -- Soft delete
);

CREATE INDEX idx_orgs_domain ON organizations(domain) WHERE deleted_at IS NULL;
CREATE INDEX idx_orgs_slug ON organizations(slug) WHERE deleted_at IS NULL;
```

#### Table: `users`
```sql
CREATE TABLE users (
    id TEXT PRIMARY KEY, -- UUID v7
    organization_id TEXT NOT NULL,
    email TEXT UNIQUE NOT NULL,
    email_verified BOOLEAN DEFAULT FALSE,
    password_hash TEXT, -- NULL if SAML-only user
    full_name TEXT NOT NULL,
    role TEXT NOT NULL, -- 'owner', 'admin', 'member'
    avatar_url TEXT,
    last_login_at INTEGER,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    deleted_at INTEGER,
    FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE
);

CREATE INDEX idx_users_org ON users(organization_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_users_email ON users(email) WHERE deleted_at IS NULL;
```

#### Table: `invites`
```sql
CREATE TABLE invites (
    id TEXT PRIMARY KEY, -- UUID v7
    organization_id TEXT NOT NULL,
    code TEXT UNIQUE NOT NULL, -- 16-char alphanumeric
    email TEXT, -- Optional: lock invite to specific email
    role TEXT DEFAULT 'member',
    invited_by TEXT NOT NULL, -- user_id
    status TEXT DEFAULT 'pending', -- pending, accepted, expired, revoked
    max_uses INTEGER DEFAULT 1,
    current_uses INTEGER DEFAULT 0,
    expires_at INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
    FOREIGN KEY (invited_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX idx_invites_code ON invites(code) WHERE status = 'pending';
CREATE INDEX idx_invites_org ON invites(organization_id);
```

#### Table: `saml_configs`
```sql
CREATE TABLE saml_configs (
    id TEXT PRIMARY KEY, -- UUID v7
    organization_id TEXT UNIQUE NOT NULL,
    entity_id TEXT NOT NULL, -- IdP Entity ID
    sso_url TEXT NOT NULL, -- SAML SSO Endpoint
    x509_cert TEXT NOT NULL, -- Base64 encoded certificate
    metadata_url TEXT, -- Optional: for auto-refresh
    name_id_format TEXT DEFAULT 'urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress',
    enforce_sso BOOLEAN DEFAULT FALSE, -- Block password login
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE
);
```

#### Table: `domains`
```sql
CREATE TABLE domains (
    id TEXT PRIMARY KEY, -- UUID v7
    organization_id TEXT NOT NULL,
    domain TEXT UNIQUE NOT NULL, -- acme.com
    verified BOOLEAN DEFAULT FALSE,
    verification_token TEXT, -- DNS TXT record value
    verified_at INTEGER,
    created_at INTEGER NOT NULL,
    FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE
);

CREATE INDEX idx_domains_org ON domains(organization_id);
CREATE INDEX idx_domains_lookup ON domains(domain) WHERE verified = TRUE;
```

#### Table: `api_keys`
```sql
CREATE TABLE api_keys (
    id TEXT PRIMARY KEY, -- UUID v7
    organization_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    name TEXT NOT NULL, -- "Production API"
    key_hash TEXT UNIQUE NOT NULL, -- SHA-256(key)
    key_prefix TEXT NOT NULL, -- First 8 chars for display
    scopes TEXT NOT NULL, -- JSON array: ["links:read", "links:write", "analytics:read"]
    last_used_at INTEGER,
    expires_at INTEGER,
    created_at INTEGER NOT NULL,
    revoked_at INTEGER,
    FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX idx_api_keys_hash ON api_keys(key_hash) WHERE revoked_at IS NULL;
```

#### Table: `audit_logs`
```sql
CREATE TABLE audit_logs (
    id TEXT PRIMARY KEY, -- UUID v7
    organization_id TEXT NOT NULL,
    user_id TEXT,
    action TEXT NOT NULL, -- 'link.created', 'user.invited'
    resource_type TEXT NOT NULL, -- 'link', 'user', 'invite'
    resource_id TEXT,
    metadata TEXT, -- JSON
    ip_address TEXT,
    user_agent TEXT,
    created_at INTEGER NOT NULL,
    FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE
);

CREATE INDEX idx_audit_org_time ON audit_logs(organization_id, created_at DESC);
```

---

### 2.2 TENANT DATABASE (Local SQLite per Org)

#### Table: `links`
```sql
CREATE TABLE links (
    id TEXT PRIMARY KEY, -- UUID v7
    short_code TEXT UNIQUE NOT NULL, -- 6-8 char alphanumeric
    destination_url TEXT NOT NULL,
    title TEXT,
    created_by TEXT NOT NULL, -- user_id from global DB
    
    -- Redirect behavior
    redirect_type TEXT DEFAULT 'temporary', -- temporary (302), permanent (301)
    
    -- Rules engine (JSON)
    rules TEXT, -- {"geo":{"US":"https://us.example.com"},"device":{"ios":"https://apps.apple.com/..."}}
    
    -- UTM & tracking
    default_utm_params TEXT, -- JSON: {"utm_source":"twitter","utm_campaign":"launch"}
    
    -- Status & metadata
    status TEXT DEFAULT 'active', -- active, paused, archived
    expires_at INTEGER, -- Unix timestamp for auto-expiry
    password_hash TEXT, -- Optional: password-protected links
    
    -- Stats cache (denormalized for performance)
    click_count INTEGER DEFAULT 0,
    last_click_at INTEGER,
    
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    
    -- Indexes
    UNIQUE(short_code)
);

CREATE INDEX idx_links_created_by ON links(created_by);
CREATE INDEX idx_links_status ON links(status, created_at DESC);
CREATE INDEX idx_links_clicks ON links(click_count DESC);
```

#### Table: `clicks`
```sql
CREATE TABLE clicks (
    id TEXT PRIMARY KEY, -- UUID v7
    link_id TEXT NOT NULL,
    short_code TEXT NOT NULL, -- Denormalized for faster queries
    
    -- Request metadata
    timestamp INTEGER NOT NULL, -- Unix timestamp in milliseconds
    ip_address TEXT,
    user_agent TEXT,
    
    -- Parsed attributes
    country_code TEXT, -- ISO 3166-1 alpha-2
    city TEXT,
    device_type TEXT, -- desktop, mobile, tablet, bot
    os TEXT, -- iOS, Android, Windows, macOS, Linux
    browser TEXT, -- Chrome, Safari, Firefox
    
    -- Referrer analysis
    referrer TEXT,
    referrer_domain TEXT, -- Extracted domain
    utm_source TEXT,
    utm_medium TEXT,
    utm_campaign TEXT,
    utm_term TEXT,
    utm_content TEXT,
    
    -- Final destination (after rules)
    destination_url TEXT NOT NULL,
    
    FOREIGN KEY (link_id) REFERENCES links(id) ON DELETE CASCADE
);

-- Performance-critical indexes
CREATE INDEX idx_clicks_link_time ON clicks(link_id, timestamp DESC);
CREATE INDEX idx_clicks_time ON clicks(timestamp DESC);
CREATE INDEX idx_clicks_country ON clicks(country_code, timestamp DESC);
CREATE INDEX idx_clicks_device ON clicks(device_type, timestamp DESC);
CREATE INDEX idx_clicks_referrer ON clicks(referrer_domain, timestamp DESC);
```

#### Table: `webhooks`
```sql
CREATE TABLE webhooks (
    id TEXT PRIMARY KEY, -- UUID v7
    url TEXT NOT NULL,
    events TEXT NOT NULL, -- JSON array: ["click.created", "link.created"]
    secret TEXT NOT NULL, -- For HMAC signature
    status TEXT DEFAULT 'active', -- active, paused, failed
    retry_count INTEGER DEFAULT 0,
    last_triggered_at INTEGER,
    last_error TEXT,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE INDEX idx_webhooks_status ON webhooks(status);
```

#### Table: `daily_stats`
```sql
-- Pre-aggregated daily statistics for fast dashboard queries
CREATE TABLE daily_stats (
    id TEXT PRIMARY KEY,
    link_id TEXT NOT NULL,
    date TEXT NOT NULL, -- YYYY-MM-DD
    clicks INTEGER DEFAULT 0,
    unique_ips INTEGER DEFAULT 0,
    top_country TEXT,
    top_referrer TEXT,
    top_device TEXT,
    created_at INTEGER NOT NULL,
    FOREIGN KEY (link_id) REFERENCES links(id) ON DELETE CASCADE,
    UNIQUE(link_id, date)
);

CREATE INDEX idx_daily_stats_link ON daily_stats(link_id, date DESC);
```

---

## 3. FOLDER STRUCTURE

```
trackr/
│
├── cmd/
│   ├── server/
│   │   └── main.go                    # Application entrypoint
│   ├── migrate/
│   │   └── main.go                    # Database migration tool
│   └── worker/
│       └── main.go                    # Background job processor
│
├── internal/
│   ├── platform/                      # Global infrastructure (Turso)
│   │   ├── auth/
│   │   │   ├── jwt.go                 # JWT generation/validation
│   │   │   ├── saml.go                # SAML 2.0 handler
│   │   │   ├── middleware.go          # Auth middleware
│   │   │   └── rbac.go                # Role-based access control
│   │   ├── database/
│   │   │   ├── turso.go               # Global DB connection
│   │   │   ├── tenant.go              # Tenant DB manager
│   │   │   └── migrations/            # SQL migration files
│   │   ├── models/
│   │   │   ├── organization.go
│   │   │   ├── user.go
│   │   │   ├── invite.go
│   │   │   └── saml_config.go
│   │   └── repositories/
│   │       ├── org_repo.go
│   │       ├── user_repo.go
│   │       └── invite_repo.go
│   │
│   ├── engine/                        # Tenant-specific logic (SQLite)
│   │   ├── links/
│   │   │   ├── service.go             # Link CRUD business logic
│   │   │   ├── shortcode.go           # Short code generation
│   │   │   ├── validator.go           # URL validation
│   │   │   └── repository.go          # SQLite queries
│   │   ├── redirect/
│   │   │   ├── handler.go             # Main redirect handler
│   │   │   ├── cache.go               # In-memory cache layer
│   │   │   ├── rules.go               # Device/Geo rules engine
│   │   │   └── logger.go              # Async click logging
│   │   ├── analytics/
│   │   │   ├── service.go             # Analytics queries
│   │   │   ├── aggregator.go          # Stats computation
│   │   │   └── repository.go
│   │   └── webhooks/
│   │       ├── dispatcher.go          # Event dispatcher
│   │       └── signer.go              # HMAC signature
│   │
│   ├── api/
│   │   ├── handlers/
│   │   │   ├── health.go
│   │   │   ├── auth_handler.go        # Login, signup, SAML
│   │   │   ├── org_handler.go         # Org management
│   │   │   ├── invite_handler.go
│   │   │   ├── link_handler.go        # Link CRUD
│   │   │   ├── analytics_handler.go
│   │   │   └── redirect_handler.go    # /:shortcode
│   │   ├── middleware/
│   │   │   ├── tenant.go              # Tenant context injection
│   │   │   ├── rate_limit.go
│   │   │   ├── cors.go
│   │   │   └── logging.go
│   │   └── router.go                  # Route definitions
│   │
│   ├── workers/
│   │   ├── stats_aggregator.go        # Daily stats rollup
│   │   ├── webhook_retry.go
│   │   └── link_expiry.go
│   │
│   └── pkg/
│       ├── email/
│       │   └── sender.go              # SMTP/SES client
│       ├── geoip/
│       │   └── resolver.go            # MaxMind GeoIP2
│       ├── parser/
│       │   ├── user_agent.go
│       │   └── referrer.go
│       ├── validator/
│       │   └── domain.go              # Corporate email check
│       └── errors/
│           └── errors.go              # Typed errors
│
├── migrations/
│   ├── global/
│   │   ├── 001_create_orgs.sql
│   │   ├── 002_create_users.sql
│   │   └── ...
│   └── tenant/
│       ├── 001_create_links.sql
│       ├── 002_create_clicks.sql
│       └── ...
│
├── configs/
│   ├── config.yaml                    # App configuration
│   └── blocked_domains.txt            # Gmail, Yahoo, etc.
│
├── scripts/
│   ├── setup.sh                       # Initial setup
│   └── backup.sh                      # DB backup script
│
├── Caddyfile                          # Caddy reverse proxy config
├── go.mod
├── go.sum
└── README.md
```

---

## 4. API ROUTES SPECIFICATION

### 4.1 Authentication Routes

#### POST `/api/v1/auth/signup`
```json
Request:
{
  "invite_code": "ABC123XYZ456",
  "email": "john@acme.com",
  "password": "SecurePass123!",
  "full_name": "John Doe"
}

Response (201):
{
  "user": {
    "id": "usr_01HQXY...",
    "email": "john@acme.com",
    "organization": {
      "id": "org_01HQXY...",
      "name": "Acme Corp",
      "slug": "acme-corp"
    },
    "role": "member"
  },
  "access_token": "eyJhbGc...",
  "refresh_token": "eyJhbGc..."
}

Validations:
- Invite code must be valid and not expired
- Email must not be from blocked domains (gmail.com, yahoo.com)
- Email domain must match organization's verified domains (optional)
- Password must meet strength requirements (8+ chars, uppercase, number, symbol)
```

#### POST `/api/v1/auth/login`
```json
Request:
{
  "email": "john@acme.com",
  "password": "SecurePass123!"
}

Response (200):
{
  "access_token": "eyJhbGc...",
  "refresh_token": "eyJhbGc...",
  "user": { /* user object */ }
}

Errors:
- 401: Invalid credentials
- 403: SAML SSO enforced, use /auth/saml/login
```

#### POST `/api/v1/auth/saml/acs`
```
Handles SAML Assertion Consumer Service callback from IdP.
Validates assertion, creates/updates user, issues JWT.
```

#### GET `/api/v1/auth/saml/metadata/:org_slug`
```
Returns SAML SP metadata XML for IdP configuration.
```

#### POST `/api/v1/auth/refresh`
```json
Request:
{
  "refresh_token": "eyJhbGc..."
}

Response (200):
{
  "access_token": "eyJhbGc..."
}
```

---

### 4.2 Organization Management

#### GET `/api/v1/organizations/current`
```
Auth: Required (Any Role)
Returns current user's organization details with quotas.
```

#### PATCH `/api/v1/organizations/current`
```json
Auth: Required (Owner/Admin)
Request:
{
  "name": "Updated Corp Name",
  "webhook_secret": "new_secret_key"
}
```

#### POST `/api/v1/organizations/domains`
```json
Auth: Required (Owner/Admin)
Request:
{
  "domain": "acme.com"
}

Response (201):
{
  "domain": "acme.com",
  "verification_token": "trackr-verify=abc123...",
  "instructions": "Add TXT record to DNS: trackr-verify=abc123..."
}
```

#### POST `/api/v1/organizations/domains/:domain_id/verify`
```
Auth: Required (Owner/Admin)
Checks DNS TXT record, marks domain as verified.
```

---

### 4.3 Invite Management

#### POST `/api/v1/invites`
```json
Auth: Required (Admin+)
Request:
{
  "email": "newuser@acme.com", // Optional
  "role": "member",
  "max_uses": 1,
  "expires_in_hours": 168 // 7 days
}

Response (201):
{
  "id": "inv_01HQXY...",
  "code": "ABC123XYZ456",
  "invite_url": "https://trackr.io/invite/ABC123XYZ456",
  "expires_at": 1702425600
}
```

#### GET `/api/v1/invites`
```
Auth: Required (Admin+)
Query Params: ?status=pending&page=1&limit=20
Returns paginated list of invites.
```

#### DELETE `/api/v1/invites/:invite_id`
```
Auth: Required (Admin+)
Revokes invite (sets status to 'revoked').
```

---

### 4.4 User Management

#### GET `/api/v1/users`
```
Auth: Required (Admin+)
Query Params: ?role=member&page=1&limit=50
Returns paginated list of organization users.
```

#### PATCH `/api/v1/users/:user_id/role`
```json
Auth: Required (Owner only)
Request:
{
  "role": "admin"
}
```

#### DELETE `/api/v1/users/:user_id`
```
Auth: Required (Owner only)
Soft deletes user (sets deleted_at).
```

---

### 4.5 Link Management

#### POST `/api/v1/links`
```json
Auth: Required (Any Role)
Request:
{
  "destination_url": "https://example.com/product",
  "title": "Product Launch Page",
  "short_code": "launch24", // Optional: custom short code
  "redirect_type": "temporary", // temporary (302) or permanent (301)
  "rules": {
    "geo": {
      "US": "https://us.example.com/product",
      "GB": "https://uk.example.com/product"
    },
    "device": {
      "ios": "https://apps.apple.com/...",
      "android": "https://play.google.com/..."
    }
  },
  "default_utm_params": {
    "utm_source": "twitter",
    "utm_campaign": "launch"
  },
  "expires_at": 1704067200, // Unix timestamp
  "password": "secret123" // Optional: password protection
}

Response (201):
{
  "id": "lnk_01HQXY...",
  "short_code": "launch24",
  "short_url": "https://trk.io/launch24",
  "destination_url": "https://example.com/product",
  "created_at": 1702425600,
  "qr_code_url": "https://api.trackr.io/v1/links/lnk_01HQXY.../qr"
}

Validations:
- destination_url must be valid HTTPS URL
- short_code must be 3-12 alphanumeric chars (if provided)
- Check link quota against organization limit
```

#### GET `/api/v1/links`
```
Auth: Required
Query Params:
  ?status=active
  &created_by=usr_01HQXY...
  &search=keyword
  &sort=clicks_desc
  &page=1
  &limit=50

Returns paginated links with basic stats.
```

#### GET `/api/v1/links/:link_id`
```
Auth: Required
Returns full link details including rules.
```

#### PATCH `/api/v1/links/:link_id`
```json
Auth: Required (Creator or Admin+)
Request:
{
  "destination_url": "https://updated.com",
  "status": "paused",
  "title": "Updated Title"
}
```

#### DELETE `/api/v1/links/:link_id`
```
Auth: Required (Creator or Admin+)
Archives link (sets status to 'archived').
```

#### GET `/api/v1/links/:link_id/qr`
```
Auth: Required
Query Params: ?size=512&format=png
Returns QR code image for the short URL.
```

---

### 4.6 Analytics Routes

#### GET `/api/v1/links/:link_id/analytics`
```
Auth: Required
Query Params:
  ?start_date=2024-01-01
  &end_date=2024-01-31
  &group_by=day // day, week, month
  &timezone=America/New_York

Response:
{
  "summary": {
    "total_clicks": 15420,
    "unique_ips": 8932,
    "avg_clicks_per_day": 497
  },
  "timeseries": [
    {"date": "2024-01-01", "clicks": 512, "unique_ips": 287},
    {"date": "2024-01-02", "clicks": 489, "unique_ips": 301}
  ],
  "top_countries": [
    {"country": "US", "clicks": 7821, "percentage": 50.7},
    {"country": "GB", "clicks": 2314, "percentage": 15.0}
  ],
  "top_referrers": [
    {"domain": "twitter.com", "clicks": 4521},
    {"domain": "facebook.com", "clicks": 2876}
  ],
  "devices": {
    "mobile": 9234,
    "desktop": 5127,
    "tablet": 1059
  },
  "browsers": {
    "Chrome": 8234,
    "Safari": 4521,
    "Firefox": 2665
  }
}
```

#### GET `/api/v1/links/:link_id/clicks`
```
Auth: Required
Query Params: ?start_date=...&end_date=...&page=1&limit=100
Returns raw click events with full metadata.
```

#### GET `/api/v1/analytics/overview`
```
Auth: Required
Returns organization-wide analytics dashboard data.
```

---

### 4.7 Webhook Management

#### POST `/api/v1/webhooks`
```json
Auth: Required (Admin+)
Request:
{
  "url": "https://api.acme.com/trackr-webhook",
  "events": ["click.created", "link.created", "link.updated"],
  "secret": "whsec_abc123..." // Optional: auto-generated if not provided
}
```

#### GET `/api/v1/webhooks`
```
Auth: Required (Admin+)
Returns list of configured webhooks.
```

#### DELETE `/api/v1/webhooks/:webhook_id`
```
Auth: Required (Admin+)
```

---

### 4.8 Redirect Route (Public)

#### GET `/:short_code`
```
Auth: None
Query Params: Any (preserved and passed through)

Flow:
1. Extract short_code from path
2. Check in-memory cache
3. If miss, query tenant SQLite DB
4. Evaluate rules (geo, device, time)
5. Apply UTM parameters
6. Check password protection
7. Issue 301/302 redirect
8. Fire async goroutine to log click

Response:
- 302/301: Location header with destination URL
- 404: Short code not found
- 410: Link expired
- 403: Password required (return password form)

Performance Target: <50ms P95 latency
```

---

### 4.9 API Key Routes

#### POST `/api/v1/api-keys`
```json
Auth: Required (Admin+)
Request:
{
  "name": "Production API Key",
  "scopes": ["links:read", "links:write", "analytics:read"],
  "expires_in_days": 365
}

Response (201):
{
  "id": "key_01HQXY...",
  "key": "trk_live_abc123xyz789...", // Only shown once
  "name": "Production API Key",
  "scopes": ["links:read", "links:write"],
  "created_at": 1702425600
}
```

#### GET `/api/v1/api-keys`
```
Auth: Required (Admin+)
Returns list with key_prefix for identification.
```

#### DELETE `/api/v1/api-keys/:key_id`
```
Auth: Required (Admin+)
Revokes API key (sets revoked_at).
```

---

## 5. CRITICAL IMPLEMENTATION DETAILS

### 5.1 Authentication Flow

#### JWT Structure
```go
type Claims struct {
    UserID         string   `json:"uid"`
    OrganizationID string   `json:"oid"`
    Role           string   `json:"role"`
    Email          string   `json:"email"`
    Scopes         []string `json:"scp"` // For API keys
    jwt.RegisteredClaims
}

// Token Lifetimes
AccessToken:  15 minutes
RefreshToken: 30 days
```

#### Middleware Chain
```go
// Example handler wrapping
handler := loggingMiddleware(
    authMiddleware(
        rbacMiddleware([]string{"admin", "owner"})(
            tenantMiddleware(
                rateLimitMiddleware(
                    actualHandler
                )
            )
        )
    )
)
```

#### SAML Flow
```
1. User clicks "Login with SSO" → /auth/saml/login?org=acme-corp
2. Server fetches SAML config from Turso
3. Generate SAML AuthnRequest, redirect to IdP SSO URL
4. IdP authenticates user, POSTs assertion to /auth/saml/acs
5. Validate signature, extract email from NameID
6. Lookup/create user in Global DB
7. Issue JWT tokens, redirect to dashboard
```

---

### 5.2 Tenant Isolation

#### Tenant Context Injection
```go
type TenantContext struct {
    OrgID      string
    OrgSlug    string
    DB         *sql.DB // SQLite connection
    LinkCache  *LinkCache
}

// Middleware extracts org from JWT, loads SQLite
func TenantMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        claims := r.Context().Value("claims").(*Claims)
        
        // Load org from Global DB
        org := orgRepo.GetByID(claims.OrganizationID)
        
        // Open tenant SQLite
        db := openSQLite(org.DBFilePath)
        
        ctx := context.WithValue(r.Context(), "tenant", &TenantContext{
            OrgID:   org.ID,
            OrgSlug: org.Slug,
            DB:      db,
        })
        
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

#### Database Connection Pool
```go
// Global singleton
type TenantDBPool struct {
    pools map[string]*sql.DB
    mu    sync.RWMutex
}

func (p *TenantDBPool) Get(orgID string, dbPath string) *sql.DB {
    p.mu.RLock()
    if db, exists := p.pools[orgID]; exists {
        p.mu.RUnlock()
        return db
    }
    p.mu.RUnlock()
    
    p.mu.Lock()
    defer p.mu.Unlock()
    
    // Double-check after acquiring write lock
    if db, exists := p.pools[orgID]; exists {
        return db
    }
    
    db, _ := sql.Open("sqlite3", dbPath+"?cache=shared&mode=rwc")
    db.SetMaxOpenConns(25)
    db.SetMaxIdleConns(5)
    p.pools[orgID] = db
    return db
}
```

---

### 5.3 High-Performance Redirect Engine

#### Cache Strategy
```go
type LinkCache struct {
    store sync.Map // map[short_code]*CachedLink
    ttl   time.Duration // 5 minutes
}

type CachedLink struct {
    ID             string
    DestinationURL string
    Rules          *RedirectRules
    RedirectType   string
    Status         string
    CachedAt       time.Time
}

// Cache flow
func (h *RedirectHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    shortCode := extractShortCode(r.URL.Path)
    
    // 1. Check cache
    if cached, ok := linkCache.Get(shortCode); ok {
        if time.Since(cached.CachedAt) < cacheTTL {
            h.redirect(w, r, cached)
            return
        }
    }
    
    // 2. Query SQLite
    link := tenantDB.GetLinkByShort
   ```go
    link := tenantDB.GetLinkByShortCode(shortCode)
    if link == nil {
        http.NotFound(w, r)
        return
    }
    
    // 3. Update cache
    linkCache.Set(shortCode, link)
    
    // 4. Redirect
    h.redirect(w, r, link)
}
```

#### Rules Engine
```go
type RedirectRules struct {
    Geo    map[string]string `json:"geo"`    // {"US": "https://...", "GB": "..."}
    Device map[string]string `json:"device"` // {"ios": "...", "android": "..."}
    Time   *TimeRule         `json:"time"`   // Business hours routing
}

type TimeRule struct {
    Timezone string            `json:"timezone"`
    Rules    map[string]string `json:"rules"` // {"weekday": "...", "weekend": "..."}
}

func (r *RedirectRules) Evaluate(ctx *RequestContext) string {
    // Priority: Device > Geo > Time > Default
    
    // 1. Device-based routing
    if r.Device != nil {
        if url, ok := r.Device[ctx.DeviceType]; ok {
            return url
        }
    }
    
    // 2. Geo-based routing
    if r.Geo != nil {
        if url, ok := r.Geo[ctx.CountryCode]; ok {
            return url
        }
    }
    
    // 3. Time-based routing
    if r.Time != nil {
        url := r.evaluateTimeRule(ctx.RequestTime)
        if url != "" {
            return url
        }
    }
    
    // 4. Default destination
    return ""
}

type RequestContext struct {
    IPAddress   string
    UserAgent   string
    CountryCode string
    DeviceType  string
    OS          string
    Browser     string
    Referrer    string
    RequestTime time.Time
}

func buildRequestContext(r *http.Request) *RequestContext {
    ip := extractIP(r)
    ua := r.UserAgent()
    
    return &RequestContext{
        IPAddress:   ip,
        UserAgent:   ua,
        CountryCode: geoip.Lookup(ip),
        DeviceType:  parseDeviceType(ua),
        OS:          parseOS(ua),
        Browser:     parseBrowser(ua),
        Referrer:    r.Referer(),
        RequestTime: time.Now(),
    }
}
```

#### Async Click Logging
```go
func (h *RedirectHandler) redirect(w http.ResponseWriter, r *http.Request, link *Link) {
    ctx := buildRequestContext(r)
    
    // Evaluate rules
    finalURL := link.DestinationURL
    if link.Rules != nil {
        if ruleURL := link.Rules.Evaluate(ctx); ruleURL != "" {
            finalURL = ruleURL
        }
    }
    
    // Apply UTM parameters
    finalURL = applyUTMParams(finalURL, link.DefaultUTMParams, r.URL.Query())
    
    // Fire-and-forget click logging (non-blocking)
    go func() {
        click := &Click{
            ID:             generateUUID(),
            LinkID:         link.ID,
            ShortCode:      link.ShortCode,
            Timestamp:      time.Now().UnixMilli(),
            IPAddress:      ctx.IPAddress,
            UserAgent:      ctx.UserAgent,
            CountryCode:    ctx.CountryCode,
            City:           geoip.LookupCity(ctx.IPAddress),
            DeviceType:     ctx.DeviceType,
            OS:             ctx.OS,
            Browser:        ctx.Browser,
            Referrer:       ctx.Referrer,
            ReferrerDomain: extractDomain(ctx.Referrer),
            UTMSource:      r.URL.Query().Get("utm_source"),
            UTMMedium:      r.URL.Query().Get("utm_medium"),
            UTMCampaign:    r.URL.Query().Get("utm_campaign"),
            DestinationURL: finalURL,
        }
        
        if err := clickRepo.Insert(click); err != nil {
            logger.Error("Failed to log click", "error", err)
        }
        
        // Update denormalized click count
        linkRepo.IncrementClickCount(link.ID)
        
        // Trigger webhooks
        webhookDispatcher.Dispatch("click.created", click)
    }()
    
    // Issue redirect
    status := http.StatusFound // 302
    if link.RedirectType == "permanent" {
        status = http.StatusMovedPermanently // 301
    }
    
    http.Redirect(w, r, finalURL, status)
}
```

---

### 5.4 Short Code Generation

```go
const (
    shortCodeChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
    shortCodeLength = 7 // Default length
)

func GenerateShortCode(customCode string, linkRepo *LinkRepository) (string, error) {
    // Use custom code if provided
    if customCode != "" {
        if !isValidShortCode(customCode) {
            return "", errors.New("invalid short code format")
        }
        
        // Check availability
        exists := linkRepo.ExistsByShortCode(customCode)
        if exists {
            return "", errors.New("short code already taken")
        }
        
        return customCode, nil
    }
    
    // Generate random code with collision retry
    maxRetries := 5
    for i := 0; i < maxRetries; i++ {
        code := generateRandomCode(shortCodeLength)
        
        exists := linkRepo.ExistsByShortCode(code)
        if !exists {
            return code, nil
        }
    }
    
    // If collisions persist, increase length
    return generateRandomCode(shortCodeLength + 1), nil
}

func generateRandomCode(length int) string {
    b := make([]byte, length)
    for i := range b {
        b[i] = shortCodeChars[rand.Intn(len(shortCodeChars))]
    }
    return string(b)
}

func isValidShortCode(code string) bool {
    if len(code) < 3 || len(code) > 12 {
        return false
    }
    
    // Only alphanumeric
    for _, c := range code {
        if !strings.ContainsRune(shortCodeChars, c) {
            return false
        }
    }
    
    // Reserved codes
    reserved := []string{"api", "admin", "dashboard", "login", "signup", "health"}
    for _, r := range reserved {
        if strings.EqualFold(code, r) {
            return false
        }
    }
    
    return true
}
```

---

### 5.5 RBAC (Role-Based Access Control)

```go
type Permission string

const (
    // Link permissions
    PermLinkCreate  Permission = "link:create"
    PermLinkRead    Permission = "link:read"
    PermLinkUpdate  Permission = "link:update"
    PermLinkDelete  Permission = "link:delete"
    
    // Analytics permissions
    PermAnalyticsRead Permission = "analytics:read"
    
    // Organization permissions
    PermOrgUpdate   Permission = "org:update"
    PermOrgDelete   Permission = "org:delete"
    
    // User management
    PermUserInvite  Permission = "user:invite"
    PermUserUpdate  Permission = "user:update"
    PermUserDelete  Permission = "user:delete"
    
    // Webhook permissions
    PermWebhookManage Permission = "webhook:manage"
)

var rolePermissions = map[string][]Permission{
    "member": {
        PermLinkCreate,
        PermLinkRead,
        PermLinkUpdate, // Own links only
        PermLinkDelete, // Own links only
        PermAnalyticsRead,
    },
    "admin": {
        PermLinkCreate,
        PermLinkRead,
        PermLinkUpdate, // All links
        PermLinkDelete, // All links
        PermAnalyticsRead,
        PermUserInvite,
        PermWebhookManage,
    },
    "owner": {
        // All permissions
        PermLinkCreate,
        PermLinkRead,
        PermLinkUpdate,
        PermLinkDelete,
        PermAnalyticsRead,
        PermOrgUpdate,
        PermOrgDelete,
        PermUserInvite,
        PermUserUpdate,
        PermUserDelete,
        PermWebhookManage,
    },
}

func HasPermission(role string, permission Permission) bool {
    perms, ok := rolePermissions[role]
    if !ok {
        return false
    }
    
    for _, p := range perms {
        if p == permission {
            return true
        }
    }
    return false
}

// Middleware
func RequirePermission(perm Permission) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            claims := r.Context().Value("claims").(*Claims)
            
            if !HasPermission(claims.Role, perm) {
                http.Error(w, "Forbidden", http.StatusForbidden)
                return
            }
            
            next.ServeHTTP(w, r)
        })
    }
}

// Resource ownership check
func CanModifyLink(claims *Claims, link *Link) bool {
    // Owner/Admin can modify any link
    if claims.Role == "owner" || claims.Role == "admin" {
        return true
    }
    
    // Members can only modify their own links
    return link.CreatedBy == claims.UserID
}
```

---

### 5.6 Corporate Email Validation

```go
var blockedDomains = []string{
    "gmail.com", "yahoo.com", "hotmail.com", "outlook.com",
    "aol.com", "icloud.com", "protonmail.com", "mail.com",
    "zoho.com", "yandex.com", "gmx.com", "live.com",
}

func IsCorpoateEmail(email string) error {
    parts := strings.Split(email, "@")
    if len(parts) != 2 {
        return errors.New("invalid email format")
    }
    
    domain := strings.ToLower(parts[1])
    
    // Check against blocked list
    for _, blocked := range blockedDomains {
        if domain == blocked {
            return errors.New("consumer email domains not allowed")
        }
    }
    
    // Additional validation: check MX records
    mx, err := net.LookupMX(domain)
    if err != nil || len(mx) == 0 {
        return errors.New("invalid email domain")
    }
    
    return nil
}

// Domain verification for auto-assignment
func CheckDomainOwnership(orgID, domain string) (bool, error) {
    // Expected TXT record: trackr-verify=<token>
    txtRecords, err := net.LookupTXT(domain)
    if err != nil {
        return false, err
    }
    
    expectedToken := getDomainVerificationToken(orgID, domain)
    expectedRecord := "trackr-verify=" + expectedToken
    
    for _, record := range txtRecords {
        if record == expectedRecord {
            return true, nil
        }
    }
    
    return false, nil
}
```

---

### 5.7 Rate Limiting

```go
type RateLimiter struct {
    store *sync.Map // map[string]*Bucket
}

type Bucket struct {
    tokens     int
    lastRefill time.Time
    mu         sync.Mutex
}

// Per-organization rate limits
var rateLimits = map[string]int{
    "redirect":    10000, // 10k redirects per minute per org
    "api_read":    1000,  // 1k API reads per minute
    "api_write":   100,   // 100 API writes per minute
    "analytics":   500,   // 500 analytics queries per minute
}

func (rl *RateLimiter) Allow(key string, limit int) bool {
    now := time.Now()
    
    val, _ := rl.store.LoadOrStore(key, &Bucket{
        tokens:     limit,
        lastRefill: now,
    })
    
    bucket := val.(*Bucket)
    bucket.mu.Lock()
    defer bucket.mu.Unlock()
    
    // Refill bucket (1 token per second)
    elapsed := now.Sub(bucket.lastRefill)
    refillTokens := int(elapsed.Seconds())
    if refillTokens > 0 {
        bucket.tokens = min(limit, bucket.tokens+refillTokens)
        bucket.lastRefill = now
    }
    
    // Check availability
    if bucket.tokens > 0 {
        bucket.tokens--
        return true
    }
    
    return false
}

func RateLimitMiddleware(limitType string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            tenant := r.Context().Value("tenant").(*TenantContext)
            key := fmt.Sprintf("%s:%s", tenant.OrgID, limitType)
            limit := rateLimits[limitType]
            
            if !rateLimiter.Allow(key, limit) {
                w.Header().Set("Retry-After", "60")
                http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
                return
            }
            
            next.ServeHTTP(w, r)
        })
    }
}
```

---

### 5.8 Webhook System

```go
type WebhookEvent struct {
    ID        string      `json:"id"`
    Event     string      `json:"event"` // "click.created", "link.created"
    Timestamp int64       `json:"timestamp"`
    OrgID     string      `json:"org_id"`
    Data      interface{} `json:"data"`
}

type WebhookDispatcher struct {
    queue chan *WebhookJob
}

type WebhookJob struct {
    Webhook *Webhook
    Event   *WebhookEvent
}

func (d *WebhookDispatcher) Dispatch(eventType string, data interface{}) {
    webhooks := webhookRepo.GetByEvent(eventType)
    
    event := &WebhookEvent{
        ID:        generateUUID(),
        Event:     eventType,
        Timestamp: time.Now().Unix(),
        Data:      data,
    }
    
    for _, webhook := range webhooks {
        d.queue <- &WebhookJob{
            Webhook: webhook,
            Event:   event,
        }
    }
}

func (d *WebhookDispatcher) Worker() {
    for job := range d.queue {
        d.deliver(job)
    }
}

func (d *WebhookDispatcher) deliver(job *WebhookJob) {
    payload, _ := json.Marshal(job.Event)
    
    // Generate HMAC signature
    signature := generateHMAC(job.Webhook.Secret, payload)
    
    req, _ := http.NewRequest("POST", job.Webhook.URL, bytes.NewBuffer(payload))
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("X-Trackr-Signature", signature)
    req.Header.Set("X-Trackr-Event", job.Event.Event)
    req.Header.Set("X-Trackr-Delivery", job.Event.ID)
    
    client := &http.Client{Timeout: 10 * time.Second}
    resp, err := client.Do(req)
    
    if err != nil || resp.StatusCode >= 400 {
        // Retry logic with exponential backoff
        d.scheduleRetry(job)
        webhookRepo.UpdateStatus(job.Webhook.ID, "failed")
    } else {
        webhookRepo.UpdateLastTriggered(job.Webhook.ID, time.Now().Unix())
    }
}

func generateHMAC(secret string, payload []byte) string {
    h := hmac.New(sha256.New, []byte(secret))
    h.Write(payload)
    return hex.EncodeToString(h.Sum(nil))
}
```

---

### 5.9 Analytics Aggregation

```go
// Background worker: runs daily at 00:00 UTC
func DailyStatsAggregator() {
    ticker := time.NewTicker(24 * time.Hour)
    defer ticker.Stop()
    
    for range ticker.C {
        aggregateYesterday()
    }
}

func aggregateYesterday() {
    yesterday := time.Now().Add(-24 * time.Hour).Format("2006-01-02")
    
    // Process each organization
    orgs := orgRepo.GetAll()
    for _, org := range orgs {
        tenantDB := openSQLite(org.DBFilePath)
        
        links := linkRepo.GetAll(tenantDB)
        for _, link := range links {
            stats := computeDailyStats(tenantDB, link.ID, yesterday)
            
            dailyStatsRepo.Upsert(tenantDB, &DailyStats{
                ID:           generateUUID(),
                LinkID:       link.ID,
                Date:         yesterday,
                Clicks:       stats.Clicks,
                UniqueIPs:    stats.UniqueIPs,
                TopCountry:   stats.TopCountry,
                TopReferrer:  stats.TopReferrer,
                TopDevice:    stats.TopDevice,
                CreatedAt:    time.Now().Unix(),
            })
        }
    }
}

func computeDailyStats(db *sql.DB, linkID, date string) *Stats {
    startTime := parseDate(date).Unix()
    endTime := startTime + 86400 // 24 hours
    
    var stats Stats
    
    // Total clicks
    db.QueryRow(`
        SELECT COUNT(*) FROM clicks 
        WHERE link_id = ? AND timestamp >= ? AND timestamp < ?
    `, linkID, startTime*1000, endTime*1000).Scan(&stats.Clicks)
    
    // Unique IPs
    db.QueryRow(`
        SELECT COUNT(DISTINCT ip_address) FROM clicks 
        WHERE link_id = ? AND timestamp >= ? AND timestamp < ?
    `, linkID, startTime*1000, endTime*1000).Scan(&stats.UniqueIPs)
    
    // Top country
    db.QueryRow(`
        SELECT country_code FROM clicks 
        WHERE link_id = ? AND timestamp >= ? AND timestamp < ?
        GROUP BY country_code 
        ORDER BY COUNT(*) DESC 
        LIMIT 1
    `, linkID, startTime*1000, endTime*1000).Scan(&stats.TopCountry)
    
    // Similar queries for TopReferrer, TopDevice...
    
    return &stats
}
```

---

### 5.10 QR Code Generation

```go
import "github.com/skip2/go-qrcode"

func GenerateQRCode(shortURL string, size int) ([]byte, error) {
    // Default size
    if size == 0 {
        size = 512
    }
    
    // Validate size
    if size < 128 || size > 2048 {
        return nil, errors.New("invalid size: must be between 128 and 2048")
    }
    
    // Generate QR code
    qr, err := qrcode.New(shortURL, qrcode.Medium)
    if err != nil {
        return nil, err
    }
    
    qr.DisableBorder = false
    
    // Return PNG bytes
    return qr.PNG(size)
}

// Handler
func (h *LinkHandler) GetQRCode(w http.ResponseWriter, r *http.Request) {
    linkID := chi.URLParam(r, "link_id")
    
    link := linkRepo.GetByID(linkID)
    if link == nil {
        http.NotFound(w, r)
        return
    }
    
    shortURL := fmt.Sprintf("https://trk.io/%s", link.ShortCode)
    
    size, _ := strconv.Atoi(r.URL.Query().Get("size"))
    if size == 0 {
        size = 512
    }
    
    qrBytes, err := GenerateQRCode(shortURL, size)
    if err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    
    w.Header().Set("Content-Type", "image/png")
    w.Header().Set("Cache-Control", "public, max-age=31536000") // 1 year
    w.Write(qrBytes)
}
```

---

## 6. CONFIGURATION

### 6.1 Application Config (config.yaml)

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  read_timeout: 10s
  write_timeout: 10s
  idle_timeout: 120s

database:
  global:
    url: "libsql://your-turso-db.turso.io"
    auth_token: "${TURSO_AUTH_TOKEN}"
    max_connections: 25
  tenant:
    base_path: "/var/lib/trackr/dbs"
    max_connections_per_org: 25

cache:
  link_ttl: 5m
  max_entries: 100000

jwt:
  secret: "${JWT_SECRET}"
  access_token_ttl: 15m
  refresh_token_ttl: 720h # 30 days

cors:
  allowed_origins:
    - "https://app.trackr.io"
    - "https://dashboard.trackr.io"
  allowed_methods:
    - GET
    - POST
    - PATCH
    - DELETE
  allowed_headers:
    - Authorization
    - Content-Type
  max_age: 3600

rate_limit:
  redirect_per_minute: 10000
  api_read_per_minute: 1000
  api_write_per_minute: 100

geoip:
  database_path: "/var/lib/trackr/geoip/GeoLite2-City.mmdb"

webhooks:
  worker_count: 10
  retry_attempts: 3
  retry_backoff: exponential # linear, exponential

logging:
  level: "info" # debug, info, warn, error
  format: "json" # json, text
  output: "stdout" # stdout, file
  file_path: "/var/log/trackr/app.log"

saml:
  sp_entity_id: "https://trackr.io/saml/metadata"
  sp_acs_url: "https://trackr.io/api/v1/auth/saml/acs"
  sp_cert_path: "/etc/trackr/saml/sp-cert.pem"
  sp_key_path: "/etc/trackr/saml/sp-key.pem"

email:
  provider: "smtp" # smtp, ses
  smtp:
    host: "smtp.sendgrid.net"
    port: 587
    username: "apikey"
    password: "${SENDGRID_API_KEY}"
    from_address: "noreply@trackr.io"
    from_name: "Trackr"

domains:
  short_domain: "trk.io"
  app_domain: "app.trackr.io"
  api_domain: "api.trackr.io"
```

---

### 6.2 Caddyfile

```caddy
{
    email admin@trackr.io
    auto_https on
}

# Main application
app.trackr.io {
    reverse_proxy localhost:8080
    
    encode gzip
    
    log {
        output file /var/log/caddy/app.log
        format json
    }
}

# API domain
api.trackr.io {
    reverse_proxy localhost:8080
    
    encode gzip
    
    # Rate limiting (via Caddy plugin if available)
    
    log {
        output file /var/log/caddy/api.log
        format json
    }
}

# Short link domain (redirects)
trk.io {
    reverse_proxy localhost:8080
    
    # No gzip for redirects (performance)
    
    log {
        output file /var/log/caddy/redirect.log
        format json
    }
}

# Wildcard for custom domains
*.trackr.io {
    reverse_proxy localhost:8080
    
    tls {
        on_demand
    }
}
```

---

## 7. SECURITY SPECIFICATIONS

### 7.1 Password Requirements

```go
func ValidatePassword(password string) error {
    if len(password) < 8 {
        return errors.New("password must be at least 8 characters")
    }
    
    var (
        hasUpper   bool
        hasLower   bool
        hasNumber  bool
        hasSpecial bool
    )
    
    for _, char := range password {
        switch {
        case unicode.IsUpper(char):
            hasUpper = true
        case unicode.IsLower(char):
            hasLower = true
        case unicode.IsNumber(char):
            hasNumber = true
        case unicode.IsPunct(char) || unicode.IsSymbol(char):
            hasSpecial = true
        }
    }
    
    if !hasUpper || !hasLower || !hasNumber || !hasSpecial {
        return errors.New("password must contain uppercase, lowercase, number, and special character")
    }
    
    return nil
}
```

### 7.2 Password Hashing

```go
import "golang.org/x/crypto/bcrypt"

func HashPassword(password string) (string, error) {
    bytes, err := bcrypt.GenerateFromPassword([]byte(password), 12)
    return string(bytes), err
}

func CheckPassword(password, hash string) bool {
    err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
    return err == nil
}
```

### 7.3 API Key Format

```
Format: trk_{env}_{random}
Examples:
  - trk_live_abc123xyz789def456ghi789
  - trk_test_xyz987wvu654tsr321qpo098

Stored as SHA-256 hash in database.
Display only first 8 chars: trk_live_abc*****
```

### 7.4 CORS Policy

```go
func CORSMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        origin := r.Header.Get("Origin")
        
        // Check against allowed origins
        if isAllowedOrigin(origin) {
            w.Header().Set("Access-Control-Allow-Origin", origin)
            w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
            w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
            w.Header().Set("Access-Control-Max-Age", "3600")
            w.Header().Set("Access-Control-Allow-Credentials", "true")
        }
        
        if r.Method == "OPTIONS" {
            w.WriteHeader(http.StatusOK)
            return
        }
        
        next.ServeHTTP(w, r)
    })
}
```

### 7.5 SQL Injection Prevention

```go
// ALWAYS use parameterized queries
// CORRECT:
db.Query("SELECT * FROM links WHERE short_code = ?", shortCode)

// NEVER concatenate user input:
// WRONG: db.Query("SELECT * FROM links WHERE short_code = '" + shortCode + "'")

// For dynamic column names, use allowlist validation:
func buildOrderByClause(sort string) string {
    allowed := map[string]string{
        "created_desc": "created_at DESC",
        "clicks_desc":  "click_count DESC",
        "title_asc":    "title ASC",
    }
    
    if clause, ok := allowed[sort]; ok {
        return clause
    }
    return "created_at DESC" // default
}
```

---

## 8. DEPLOYMENT & OPERATIONS

### 8.1 Initialization Script

```bash
#!/bin/bash
# scripts/setup.sh

set -e

echo "=== Trackr Setup ==="

# Create directories
mkdir -p /var/lib/trackr/dbs
mkdir -p /var/lib/trackr/geoip
mkdir -p /var/log/trackr
mkdir -p /etc/trackr/saml

# Download GeoIP database
echo "Downloading GeoIP database..."
wget -O /var/lib/trackr/geoip/GeoLite2-City.mmdb.tar.gz \
  "https://download.maxmind.com/app/geoip_download?license_key=${MAXMIND_LICENSE_KEY}&edition_id=GeoLite2-City&suffix=tar.gz"
tar -xzf /var/lib/trackr/geoip/GeoLite2-City.mmdb.tar.gz -C /var/lib/trackr/geoip --strip-components=1

# Generate SAML certificates
echo "Generating SAML certificates..."
openssl req -x509 -newkey rsa:2048 -keyout /etc/trackr/saml/sp-key.pem \
  -out /etc/trackr/saml/sp-cert.pem -days 3650 -nodes \
  -subj "/CN=trackr.io"

# Run database migrations
echo "Running global DB migrations..."
./trackr migrate --target=global --direction=up

echo "Setup complete!"
```

### 8.2 Migration Command

```go
// cmd/migrate/main.go
package main

import (
    "flag"
    "fmt"
    "log"
)

func main() {
    target := flag.String("target", "global", "Migration target: global or tenant")
    direction := flag.String("direction", "up", "Migration direction: up or down")
    orgID := flag.String("org", "", "Organization ID (required for tenant migrations)")
    
    flag.Parse()
    
    switch *target {
    case "global":
        if err := migrateGlobal(*direction); err != nil {
            log.Fatal(err)
        }
    case "tenant":
        if *orgID == "" {
            log.Fatal("--org flag required for tenant migrations")
        }
        if err := migrateTenant(*orgID, *direction); err != nil {
            log.Fatal(err)
        }
    default:
        log.Fatal("Invalid target: must be 'global' or 'tenant'")
    }
    
    fmt.Println("Migration completed successfully")
}
```

### 8.3 Health Check Endpoint

```go
// GET /health
type HealthResponse struct {
    Status    string            `json:"status"`
    Timestamp int64             `json:"timestamp"`
    Checks    map[string]string `json:"checks"`
}

func HealthHandler(w http.ResponseWriter, r *http.Request) {
    checks := make(map[string]string)
    
    // Check Global DB
    if err := globalDB.Ping(); err != nil {
        checks["global_db"] = "unhealthy: " + err.Error()
    } else {
        checks["global_db"] = "healthy"
    }
    
    // Check GeoIP
    if _, err := geoip.Lookup("8.8.8.8"); err != nil {
