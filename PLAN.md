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
```go
        checks["geoip"] = "unhealthy: " + err.Error()
    } else {
        checks["geoip"] = "healthy"
    }
    
    // Check cache
    checks["cache"] = "healthy"
    
    // Determine overall status
    status := "healthy"
    for _, check := range checks {
        if strings.HasPrefix(check, "unhealthy") {
            status = "degraded"
            break
        }
    }
    
    response := HealthResponse{
        Status:    status,
        Timestamp: time.Now().Unix(),
        Checks:    checks,
    }
    
    statusCode := http.StatusOK
    if status == "degraded" {
        statusCode = http.StatusServiceUnavailable
    }
    
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(statusCode)
    json.NewEncoder(w).Encode(response)
}
```

### 8.4 Backup Strategy

```bash
#!/bin/bash
# scripts/backup.sh

BACKUP_DIR="/var/backups/trackr"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

# Backup Global DB (Turso - handled by Turso's built-in backups)
# But we export a snapshot for redundancy
echo "Exporting global DB snapshot..."
turso db shell trackr-global ".dump" > $BACKUP_DIR/global_${TIMESTAMP}.sql

# Backup all tenant databases
echo "Backing up tenant databases..."
mkdir -p $BACKUP_DIR/tenants_${TIMESTAMP}
cp /var/lib/trackr/dbs/*.db $BACKUP_DIR/tenants_${TIMESTAMP}/

# Compress backups
echo "Compressing..."
tar -czf $BACKUP_DIR/full_backup_${TIMESTAMP}.tar.gz \
  $BACKUP_DIR/global_${TIMESTAMP}.sql \
  $BACKUP_DIR/tenants_${TIMESTAMP}

# Cleanup old backups (keep last 30 days)
find $BACKUP_DIR -name "full_backup_*.tar.gz" -mtime +30 -delete

# Upload to S3 (optional)
# aws s3 cp $BACKUP_DIR/full_backup_${TIMESTAMP}.tar.gz s3://trackr-backups/

echo "Backup completed: full_backup_${TIMESTAMP}.tar.gz"
```

### 8.5 Monitoring Metrics

```go
// Expose Prometheus metrics
import "github.com/prometheus/client_golang/prometheus"

var (
    redirectLatency = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "trackr_redirect_duration_seconds",
            Help:    "Redirect latency distribution",
            Buckets: prometheus.DefBuckets,
        },
        []string{"status"},
    )
    
    redirectTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "trackr_redirects_total",
            Help: "Total number of redirects",
        },
        []string{"org_id", "status"},
    )
    
    clicksLogged = prometheus.NewCounter(
        prometheus.CounterOpts{
            Name: "trackr_clicks_logged_total",
            Help: "Total clicks successfully logged",
        },
    )
    
    cacheHitRate = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "trackr_cache_hit_rate",
            Help: "Cache hit rate percentage",
        },
        []string{"cache_type"},
    )
    
    activeOrganizations = prometheus.NewGauge(
        prometheus.GaugeOpts{
            Name: "trackr_active_organizations",
            Help: "Number of active organizations",
        },
    )
)

func init() {
    prometheus.MustRegister(
        redirectLatency,
        redirectTotal,
        clicksLogged,
        cacheHitRate,
        activeOrganizations,
    )
}

// Instrument redirect handler
func (h *RedirectHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    start := time.Now()
    
    // ... redirect logic ...
    
    duration := time.Since(start).Seconds()
    redirectLatency.WithLabelValues("success").Observe(duration)
    redirectTotal.WithLabelValues(orgID, "success").Inc()
}
```

---

## 9. ERROR HANDLING

### 9.1 Standard Error Responses

```go
type ErrorResponse struct {
    Error   string      `json:"error"`
    Message string      `json:"message"`
    Code    string      `json:"code"`
    Details interface{} `json:"details,omitempty"`
}

// Error codes
const (
    ErrCodeInvalidInput      = "INVALID_INPUT"
    ErrCodeUnauthorized      = "UNAUTHORIZED"
    ErrCodeForbidden         = "FORBIDDEN"
    ErrCodeNotFound          = "NOT_FOUND"
    ErrCodeConflict          = "CONFLICT"
    ErrCodeQuotaExceeded     = "QUOTA_EXCEEDED"
    ErrCodeRateLimitExceeded = "RATE_LIMIT_EXCEEDED"
    ErrCodeInternal          = "INTERNAL_ERROR"
)

func WriteError(w http.ResponseWriter, status int, code, message string, details interface{}) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    
    json.NewEncoder(w).Encode(ErrorResponse{
        Error:   http.StatusText(status),
        Message: message,
        Code:    code,
        Details: details,
    })
}

// Usage examples
func (h *LinkHandler) Create(w http.ResponseWriter, r *http.Request) {
    var req CreateLinkRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        WriteError(w, http.StatusBadRequest, ErrCodeInvalidInput, 
            "Invalid request body", nil)
        return
    }
    
    // Check quota
    if linkCount >= org.LinkQuota {
        WriteError(w, http.StatusForbidden, ErrCodeQuotaExceeded,
            "Link quota exceeded", map[string]interface{}{
                "current": linkCount,
                "limit":   org.LinkQuota,
            })
        return
    }
    
    // ... rest of handler
}
```

### 9.2 Validation Errors

```go
type ValidationError struct {
    Field   string `json:"field"`
    Message string `json:"message"`
}

func ValidateCreateLinkRequest(req *CreateLinkRequest) []ValidationError {
    var errors []ValidationError
    
    // Validate destination URL
    if req.DestinationURL == "" {
        errors = append(errors, ValidationError{
            Field:   "destination_url",
            Message: "Destination URL is required",
        })
    } else if !isValidURL(req.DestinationURL) {
        errors = append(errors, ValidationError{
            Field:   "destination_url",
            Message: "Invalid URL format",
        })
    }
    
    // Validate short code
    if req.ShortCode != "" && !isValidShortCode(req.ShortCode) {
        errors = append(errors, ValidationError{
            Field:   "short_code",
            Message: "Short code must be 3-12 alphanumeric characters",
        })
    }
    
    // Validate redirect type
    validTypes := []string{"temporary", "permanent"}
    if req.RedirectType != "" && !contains(validTypes, req.RedirectType) {
        errors = append(errors, ValidationError{
            Field:   "redirect_type",
            Message: "Must be 'temporary' or 'permanent'",
        })
    }
    
    return errors
}

// In handler
func (h *LinkHandler) Create(w http.ResponseWriter, r *http.Request) {
    var req CreateLinkRequest
    json.NewDecoder(r.Body).Decode(&req)
    
    if validationErrors := ValidateCreateLinkRequest(&req); len(validationErrors) > 0 {
        WriteError(w, http.StatusBadRequest, ErrCodeInvalidInput,
            "Validation failed", validationErrors)
        return
    }
    
    // ... proceed with creation
}
```

---

## 10. TESTING STRATEGY

### 10.1 Unit Tests Structure

```
internal/
├── engine/
│   ├── links/
│   │   ├── service.go
│   │   ├── service_test.go
│   │   ├── shortcode_test.go
│   │   └── validator_test.go
│   └── redirect/
│       ├── handler_test.go
│       ├── cache_test.go
│       └── rules_test.go
└── platform/
    └── auth/
        ├── jwt_test.go
        └── rbac_test.go
```

### 10.2 Critical Test Cases

```go
// internal/engine/redirect/rules_test.go
package redirect

import "testing"

func TestGeoRuleEvaluation(t *testing.T) {
    tests := []struct {
        name        string
        rules       *RedirectRules
        countryCode string
        expected    string
    }{
        {
            name: "US visitor gets US URL",
            rules: &RedirectRules{
                Geo: map[string]string{
                    "US": "https://us.example.com",
                    "GB": "https://uk.example.com",
                },
            },
            countryCode: "US",
            expected:    "https://us.example.com",
        },
        {
            name: "Unknown country gets default",
            rules: &RedirectRules{
                Geo: map[string]string{
                    "US": "https://us.example.com",
                },
            },
            countryCode: "FR",
            expected:    "",
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            ctx := &RequestContext{CountryCode: tt.countryCode}
            result := tt.rules.Evaluate(ctx)
            
            if result != tt.expected {
                t.Errorf("Expected %s, got %s", tt.expected, result)
            }
        })
    }
}

func TestDeviceRulePriority(t *testing.T) {
    rules := &RedirectRules{
        Device: map[string]string{
            "ios": "https://apps.apple.com/app",
        },
        Geo: map[string]string{
            "US": "https://us.example.com",
        },
    }
    
    ctx := &RequestContext{
        DeviceType:  "mobile",
        OS:          "iOS",
        CountryCode: "US",
    }
    
    // Device rules should take priority
    result := rules.Evaluate(ctx)
    expected := "https://apps.apple.com/app"
    
    if result != expected {
        t.Errorf("Device rule should take priority. Expected %s, got %s", expected, result)
    }
}
```

### 10.3 Integration Test Example

```go
// internal/api/handlers/link_handler_integration_test.go
package handlers

import (
    "bytes"
    "encoding/json"
    "net/http/httptest"
    "testing"
)

func TestLinkCreationFlow(t *testing.T) {
    // Setup test database
    db := setupTestDB(t)
    defer db.Close()
    
    // Create test organization and user
    org := createTestOrg(t, db)
    user := createTestUser(t, db, org.ID)
    token := generateTestJWT(user)
    
    // Test link creation
    reqBody := map[string]interface{}{
        "destination_url": "https://example.com/test",
        "title":           "Test Link",
        "short_code":      "test123",
    }
    body, _ := json.Marshal(reqBody)
    
    req := httptest.NewRequest("POST", "/api/v1/links", bytes.NewBuffer(body))
    req.Header.Set("Authorization", "Bearer "+token)
    req.Header.Set("Content-Type", "application/json")
    
    w := httptest.NewRecorder()
    router.ServeHTTP(w, req)
    
    // Assert response
    if w.Code != 201 {
        t.Errorf("Expected status 201, got %d", w.Code)
    }
    
    var response CreateLinkResponse
    json.NewDecoder(w.Body).Decode(&response)
    
    if response.ShortCode != "test123" {
        t.Errorf("Expected short_code 'test123', got '%s'", response.ShortCode)
    }
    
    // Test redirect functionality
    redirectReq := httptest.NewRequest("GET", "/test123", nil)
    redirectW := httptest.NewRecorder()
    router.ServeHTTP(redirectW, redirectReq)
    
    if redirectW.Code != 302 {
        t.Errorf("Expected redirect 302, got %d", redirectW.Code)
    }
    
    location := redirectW.Header().Get("Location")
    if location != "https://example.com/test" {
        t.Errorf("Expected redirect to 'https://example.com/test', got '%s'", location)
    }
}
```

### 10.4 Performance Benchmarks

```go
// internal/engine/redirect/benchmark_test.go
package redirect

import "testing"

func BenchmarkRedirectHandler(b *testing.B) {
    handler := setupBenchHandler()
    
    b.ResetTimer()
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            req := httptest.NewRequest("GET", "/abc123", nil)
            w := httptest.NewRecorder()
            handler.ServeHTTP(w, req)
        }
    })
}

func BenchmarkCacheLookup(b *testing.B) {
    cache := NewLinkCache()
    cache.Set("test", &CachedLink{
        DestinationURL: "https://example.com",
    })
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        cache.Get("test")
    }
}

// Target: <50ms P95 latency for redirects
// Target: <1ms for cache lookups
```

---

## 11. API REQUEST/RESPONSE EXAMPLES

### 11.1 Complete Link Creation Flow

```http
### Create Link
POST https://api.trackr.io/v1/links
Authorization: Bearer eyJhbGc...
Content-Type: application/json

{
  "destination_url": "https://example.com/product",
  "title": "Product Launch Page",
  "short_code": "launch24",
  "redirect_type": "temporary",
  "rules": {
    "geo": {
      "US": "https://us.example.com/product",
      "GB": "https://uk.example.com/product",
      "default": "https://global.example.com/product"
    },
    "device": {
      "ios": "https://apps.apple.com/app/product",
      "android": "https://play.google.com/store/apps/product"
    }
  },
  "default_utm_params": {
    "utm_source": "twitter",
    "utm_medium": "social",
    "utm_campaign": "launch2024"
  },
  "expires_at": 1735689600
}

### Response
HTTP/1.1 201 Created
Content-Type: application/json

{
  "id": "lnk_01HQXY2Z8K9NVMW7X6BQTA45BC",
  "short_code": "launch24",
  "short_url": "https://trk.io/launch24",
  "destination_url": "https://example.com/product",
  "title": "Product Launch Page",
  "redirect_type": "temporary",
  "status": "active",
  "created_by": "usr_01HQXY1A2B3C4D5E6F7G8H9I0J",
  "click_count": 0,
  "qr_code_url": "https://api.trackr.io/v1/links/lnk_01HQXY.../qr",
  "created_at": 1702425600,
  "updated_at": 1702425600
}
```

### 11.2 Analytics Query Example

```http
### Get Link Analytics
GET https://api.trackr.io/v1/links/lnk_01HQXY.../analytics?start_date=2024-01-01&end_date=2024-01-31&group_by=day
Authorization: Bearer eyJhbGc...

### Response
HTTP/1.1 200 OK
Content-Type: application/json

{
  "link_id": "lnk_01HQXY2Z8K9NVMW7X6BQTA45BC",
  "period": {
    "start": "2024-01-01",
    "end": "2024-01-31"
  },
  "summary": {
    "total_clicks": 15420,
    "unique_ips": 8932,
    "unique_devices": 7654,
    "avg_clicks_per_day": 497,
    "peak_day": {
      "date": "2024-01-15",
      "clicks": 1247
    }
  },
  "timeseries": [
    {
      "date": "2024-01-01",
      "clicks": 512,
      "unique_ips": 287
    },
    {
      "date": "2024-01-02",
      "clicks": 489,
      "unique_ips": 301
    }
  ],
  "geography": {
    "countries": [
      {
        "code": "US",
        "name": "United States",
        "clicks": 7821,
        "percentage": 50.7
      },
      {
        "code": "GB",
        "name": "United Kingdom",
        "clicks": 2314,
        "percentage": 15.0
      }
    ],
    "cities": [
      {
        "name": "New York",
        "country": "US",
        "clicks": 2145
      }
    ]
  },
  "referrers": [
    {
      "domain": "twitter.com",
      "clicks": 4521,
      "percentage": 29.3
    },
    {
      "domain": "facebook.com",
      "clicks": 2876,
      "percentage": 18.7
    },
    {
      "domain": "direct",
      "clicks": 3421,
      "percentage": 22.2
    }
  ],
  "devices": {
    "types": {
      "mobile": 9234,
      "desktop": 5127,
      "tablet": 1059
    },
    "operating_systems": {
      "iOS": 5421,
      "Android": 3813,
      "Windows": 3127,
      "macOS": 2198,
      "Linux": 861
    },
    "browsers": {
      "Chrome": 8234,
      "Safari": 4521,
      "Firefox": 2665
    }
  },
  "utm_parameters": {
    "sources": [
      {"value": "twitter", "clicks": 5234},
      {"value": "facebook", "clicks": 3421}
    ],
    "campaigns": [
      {"value": "launch2024", "clicks": 8932}
    ]
  }
}
```

### 11.3 Invite User Flow

```http
### Create Invite
POST https://api.trackr.io/v1/invites
Authorization: Bearer eyJhbGc...
Content-Type: application/json

{
  "email": "newuser@acme.com",
  "role": "member",
  "max_uses": 1,
  "expires_in_hours": 168
}

### Response
HTTP/1.1 201 Created

{
  "id": "inv_01HQXZ3A4B5C6D7E8F9G0H1I2J",
  "code": "TRK-ABC123-XYZ789",
  "email": "newuser@acme.com",
  "role": "member",
  "invite_url": "https://app.trackr.io/signup?code=TRK-ABC123-XYZ789",
  "status": "pending",
  "max_uses": 1,
  "current_uses": 0,
  "expires_at": 1703033600,
  "created_at": 1702428800
}

### User Signs Up
POST https://api.trackr.io/v1/auth/signup
Content-Type: application/json

{
  "invite_code": "TRK-ABC123-XYZ789",
  "email": "newuser@acme.com",
  "password": "SecurePass123!",
  "full_name": "New User"
}

### Response
HTTP/1.1 201 Created

{
  "user": {
    "id": "usr_01HQXZ4K5L6M7N8O9P0Q1R2S3T",
    "email": "newuser@acme.com",
    "full_name": "New User",
    "role": "member",
    "organization": {
      "id": "org_01HQXY...",
      "name": "Acme Corp",
      "slug": "acme-corp"
    }
  },
  "access_token": "eyJhbGc...",
  "refresh_token": "eyJhbGc...",
  "expires_in": 900
}
```

---

## 12. WORKER PROCESSES

### 12.1 Stats Aggregation Worker

```go
// cmd/worker/main.go
package main

import (
    "time"
    "log"
)

func main() {
    // Start daily stats aggregator
    go runDailyStatsWorker()
    
    // Start webhook retry worker
    go runWebhookRetryWorker()
    
    // Start link expiry worker
    go runLinkExpiryWorker()
    
    // Keep process alive
    select {}
}

func runDailyStatsWorker() {
    // Run at 01:00 UTC daily
    for {
        now := time.Now().UTC()
        next := time.Date(now.Year(), now.Month(), now.Day()+1, 1, 0, 0, 0, time.UTC)
        duration := next.Sub(now)
        
        log.Printf("Daily stats worker sleeping for %v", duration)
        time.Sleep(duration)
        
        log.Println("Running daily stats aggregation...")
        if err := aggregateDailyStats(); err != nil {
            log.Printf("Error aggregating stats: %v", err)
        }
    }
}

func aggregateDailyStats() error {
    yesterday := time.Now().Add(-24 * time.Hour).Format("2006-01-02")
    
    orgs, err := orgRepo.GetAll()
    if err != nil {
        return err
    }
    
    for _, org := range orgs {
        tenantDB, err := openTenantDB(org.DBFilePath)
        if err != nil {
            log.Printf("Failed to open tenant DB for org %s: %v", org.ID, err)
            continue
        }
        
        links, _ := linkRepo.GetAll(tenantDB)
        for _, link := range links {
            stats := computeDailyStats(tenantDB, link.ID, yesterday)
            
            if err := dailyStatsRepo.Upsert(tenantDB, stats); err != nil {
                log.Printf("Failed to save stats for link %s: %v", link.ID, err)
            }
        }
        
        tenantDB.Close()
    }
    
    log.Printf("Completed daily stats aggregation for %s", yesterday)
    return nil
}
```

### 12.2 Webhook Retry Worker

```go
func runWebhookRetryWorker() {
    ticker := time.NewTicker(5 * time.Minute)
    defer ticker.Stop()
    
    for range ticker.C {
        retryFailedWebhooks()
    }
}

func retryFailedWebhooks() {
    // Find webhooks that failed in the last hour
    failedWebhooks := webhookRepo.GetFailed(time.Now().Add(-1 * time.Hour).Unix())
    
    for _, webhook := range failedWebhooks {
        if webhook.RetryCount >= 3 {
            // Max retries reached, pause webhook
            webhookRepo.UpdateStatus(webhook.ID, "paused")
            log.Printf("Webhook %s paused after max retries", webhook.ID)
            continue
        }
        
        // Exponential backoff
        backoffDuration := time.Duration(math.Pow(2, float64(webhook.RetryCount))) * time.Minute
        if time.Since(time.Unix(webhook.LastTriggeredAt, 0)) < backoffDuration {
            continue
        }
        
        // Retry delivery
        if err := webhookDispatcher.DeliverSync(webhook); err != nil {
            webhookRepo.IncrementRetryCount(webhook.ID)
            webhookRepo.UpdateLastError(webhook.ID, err.Error())
        } else {
            webhookRepo.UpdateStatus(webhook.ID, "active")
            webhookRepo.ResetRetryCount(webhook.ID)
        }
    }
}
```

### 12.3 Link Expiry Worker

```go
func runLinkExpiryWorker() {
    ticker := time.NewTicker(1 * time.Hour)
    defer ticker.Stop()
    
    for range ticker.C {
        expireLinks()
    }
}

func expireLinks() {
    now := time.Now().Unix()
    
    orgs, _ := orgRepo.GetAll()
    for _, org := range orgs {
        tenantDB, _ := openTenantDB(org.DBFilePath)
        
        // Find links that have passed their expiry date
        expiredLinks, _ := linkRepo.GetExpired(tenantDB, now)
        
        for _, link := range expiredLinks {
            // Archive the link
            linkRepo.UpdateStatus(tenantDB, link.ID, "archived")
            log.Printf("Archived expired link: %s", link.ShortCode)
            
            // Trigger webhook
            webhookDispatcher.Dispatch("link.expired", map[string]interface{}{
                "link_id":    link.ID,
                "short_code": link.ShortCode,
                "expired_at": link.ExpiresAt,
            })
        }
        
        tenantDB.Close()
    }
}
```

---

## 13. CRITICAL PERFORMANCE OPTIMIZATIONS

### 13.1 SQLite Optimizations

```go
func openTenantDB(path string) (*sql.DB, error) {
    // Optimal pragmas for read-heavy workload
    dsn := path + "?" +
        "cache=shared&" +
        "mode=rwc&" +
        "_journal_mode=WAL&" +
        "_synchronous=NORMAL&" +
        "_cache_size=-64000&" + // 64MB cache
        "_temp_store=MEMORY&" +
        "_mmap_size=268435456" // 256MB mmap
    
    db, err := sql.Open("sqlite3", dsn)
    if err != nil {
        return nil, err
    }
    
    // Connection pool settings
    db.SetMaxOpenConns(25)
    db.SetMaxIdleConns(10)
    db.SetConnMaxLifetime(time.Hour)
    
    // Execute pragmas
    _, err = db.Exec(`
        PRAGMA journal_mode = WAL;
        PRAGMA synchronous = NORMAL;
        PRAGMA cache_size = -64000;
        PRAGMA temp_store = MEMORY;
        PRAGMA mmap_size = 268435456;
        PRAGMA page_size = 4096;
    `)
    
    return db, err
}
```

### 13.2 Prepared Statements

```go
type LinkRepository struct {
    db                *sql.DB
    getByShortStmt    *sql.Stmt
    insertClickStmt   *sql.Stmt
    incrementClickStmt *sql.Stmt
}

func NewLinkRepository(db *sql.DB) (*LinkRepository, error) {
    repo := &LinkRepository{db: db}
    
    var err error
    
    // Pre-compile frequently used queries
    repo.getByShortStmt, err = db.Prepare(`
        SELECT id, short_code, destination_url, rules, redirect_type, status, password_hash
        FROM links 
        WHERE short_code = ? AND status = 'active'
    `)
    if err != nil {
        return nil, err
    }
    
    repo.insertClickStmt, err = db.Prepare(`
        INSERT INTO clicks (
            id, link_id, short_code, timestamp, ip_address, user_agent,
            country_code, city, device_type, os, browser, referrer,
            referrer_domain, utm_source, utm_medium, utm_campaign, destination_url
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `)
    if err != nil {
        return nil, err
    }
    
    repo.incrementClickStmt, err = db.Prepare(`
        UPDATE links 
        SET click_count = click_count + 1, last_click_at = ?
        WHERE id = ?
    `)
    if err != nil {
        return nil, err
    }
    
    return repo, nil
}
```

### 13.3 Batch Inserts for Click Logging

```go
type ClickBuffer struct {
    clicks []Click
    mu     sync.Mutex
    size   int
}

func (b *ClickBuffer) Add(click Click) {
    b.mu.Lock()
    defer b.mu.Unlock()
    
    b.clicks = append(b.clicks, click)
    
    // Flush when buffer is full
    if len(b.clicks) >= b.size {
        go b.Flush()
    }
}

func (b *ClickBuffer) Flush() {
    b.mu.Lock()
    clicks := b.clicks
    b.clicks = nil
    b.mu.Unlock()
    
    if len(clicks) == 0 {
        return
    }
    
    // Batch insert
    tx, _ := db.Begin()
    stmt, _ := tx.Prepare(`
        INSERT INTO clicks (...) VALUES (?, ?, ...)
    `)
    
    for _, click := range clicks {
        stmt.Exec(click.ID, click.LinkID, /* ... */)
    }
    
    stmt.Close()
    tx.Commit()
}

// Periodic flush every 5 seconds
func (b *ClickBuffer) StartPeriodicFlush() {
    ticker := time.NewTicker(5 * time.Second)
    go func() {
        for range ticker.C {
            b.Flush()
        }
    }()
}
```

---

## 14. SAML IMPLEMENTATION DETAILS

### 14.1 SAML Configuration

```go
import (
    "github.com/crewjam/saml"
    "github.com/crewjam/saml/samlsp"
)

func initSAMLMiddleware(org *Organization, samlConfig *SAMLConfig) (*samlsp.Middleware, error) {
    keyPair, err := tls.LoadX509KeyPair(
        config.
```go
        config.SAML.CertPath,
        config.SAML.KeyPath,
    )
    if err != nil {
        return nil, err
    }
    
    keyPair.Leaf, err = x509.ParseCertificate(keyPair.Certificate[0])
    if err != nil {
        return nil, err
    }
    
    idpMetadataURL, _ := url.Parse(samlConfig.MetadataURL)
    idpMetadata, err := samlsp.FetchMetadata(
        context.Background(),
        http.DefaultClient,
        *idpMetadataURL,
    )
    if err != nil {
        return nil, err
    }
    
    rootURL, _ := url.Parse(fmt.Sprintf("https://%s", config.Domains.AppDomain))
    
    samlSP, err := samlsp.New(samlsp.Options{
        URL:            *rootURL,
        Key:            keyPair.PrivateKey.(*rsa.PrivateKey),
        Certificate:    keyPair.Leaf,
        IDPMetadata:    idpMetadata,
        EntityID:       samlConfig.EntityID,
        SignRequest:    true,
        UseArtifactResponse: false,
        CookieName:     fmt.Sprintf("saml_%s", org.Slug),
        CookieSecure:   true,
        CookieSameSite: http.SameSiteNoneMode,
    })
    
    return samlSP, err
}
```

### 14.2 SAML Assertion Processing

```go
func (h *AuthHandler) HandleSAMLCallback(w http.ResponseWriter, r *http.Request) {
    // Parse SAML response
    assertion := samlsp.AssertionFromContext(r.Context())
    if assertion == nil {
        http.Error(w, "Invalid SAML assertion", http.StatusBadRequest)
        return
    }
    
    // Extract user attributes
    email := extractAttribute(assertion, "email")
    if email == "" {
        email = assertion.Subject.NameID.Value
    }
    
    fullName := extractAttribute(assertion, "name")
    if fullName == "" {
        firstName := extractAttribute(assertion, "firstName")
        lastName := extractAttribute(assertion, "lastName")
        fullName = strings.TrimSpace(firstName + " " + lastName)
    }
    
    // Extract organization from email domain or RelayState
    domain := strings.Split(email, "@")[1]
    org, err := orgRepo.GetByDomain(domain)
    if err != nil {
        http.Error(w, "Organization not found", http.StatusNotFound)
        return
    }
    
    // Verify SAML is enabled for this org
    if !org.SAMLEnabled {
        http.Error(w, "SAML not enabled for organization", http.StatusForbidden)
        return
    }
    
    // Get or create user
    user, err := userRepo.GetByEmail(email)
    if err != nil {
        // Create new user via SAML
        user = &User{
            ID:             generateUUID(),
            OrganizationID: org.ID,
            Email:          email,
            EmailVerified:  true, // Trust IdP verification
            FullName:       fullName,
            Role:           "member",
            PasswordHash:   "", // SAML-only user
            CreatedAt:      time.Now().Unix(),
            UpdatedAt:      time.Now().Unix(),
        }
        
        if err := userRepo.Create(user); err != nil {
            http.Error(w, "Failed to create user", http.StatusInternalServerError)
            return
        }
    }
    
    // Update last login
    userRepo.UpdateLastLogin(user.ID, time.Now().Unix())
    
    // Generate JWT tokens
    accessToken, err := generateAccessToken(user, org)
    refreshToken, err := generateRefreshToken(user)
    
    // Set secure cookies
    http.SetCookie(w, &http.Cookie{
        Name:     "access_token",
        Value:    accessToken,
        Path:     "/",
        MaxAge:   900, // 15 minutes
        Secure:   true,
        HttpOnly: true,
        SameSite: http.SameSiteStrictMode,
    })
    
    http.SetCookie(w, &http.Cookie{
        Name:     "refresh_token",
        Value:    refreshToken,
        Path:     "/",
        MaxAge:   2592000, // 30 days
        Secure:   true,
        HttpOnly: true,
        SameSite: http.SameSiteStrictMode,
    })
    
    // Redirect to dashboard
    http.Redirect(w, r, "https://app.trackr.io/dashboard", http.StatusFound)
}

func extractAttribute(assertion *saml.Assertion, name string) string {
    for _, stmt := range assertion.AttributeStatements {
        for _, attr := range stmt.Attributes {
            if attr.Name == name || attr.FriendlyName == name {
                if len(attr.Values) > 0 {
                    return attr.Values[0].Value
                }
            }
        }
    }
    return ""
}
```

### 14.3 SAML Metadata Endpoint

```go
func (h *AuthHandler) GetSAMLMetadata(w http.ResponseWriter, r *http.Request) {
    orgSlug := chi.URLParam(r, "org_slug")
    
    org, err := orgRepo.GetBySlug(orgSlug)
    if err != nil {
        http.NotFound(w, r)
        return
    }
    
    samlConfig, err := samlConfigRepo.GetByOrgID(org.ID)
    if err != nil {
        http.Error(w, "SAML not configured", http.StatusNotFound)
        return
    }
    
    middleware, err := initSAMLMiddleware(org, samlConfig)
    if err != nil {
        http.Error(w, "SAML configuration error", http.StatusInternalServerError)
        return
    }
    
    // Generate SP metadata XML
    metadata := middleware.ServiceProvider.Metadata()
    metadataXML, err := xml.MarshalIndent(metadata, "", "  ")
    if err != nil {
        http.Error(w, "Failed to generate metadata", http.StatusInternalServerError)
        return
    }
    
    w.Header().Set("Content-Type", "application/samlmetadata+xml")
    w.Write([]byte(xml.Header))
    w.Write(metadataXML)
}
```

---

## 15. ENVIRONMENT VARIABLES

```bash
# .env.example

# Server Configuration
SERVER_HOST=0.0.0.0
SERVER_PORT=8080

# Database
TURSO_DB_URL=libsql://trackr-global.turso.io
TURSO_AUTH_TOKEN=your_turso_auth_token_here
TENANT_DB_PATH=/var/lib/trackr/dbs

# JWT
JWT_SECRET=your_jwt_secret_minimum_32_characters_here
JWT_ACCESS_TTL=15m
JWT_REFRESH_TTL=720h

# Domains
SHORT_DOMAIN=trk.io
APP_DOMAIN=app.trackr.io
API_DOMAIN=api.trackr.io

# GeoIP
GEOIP_DB_PATH=/var/lib/trackr/geoip/GeoLite2-City.mmdb
MAXMIND_LICENSE_KEY=your_maxmind_license_key

# Email (SMTP)
SMTP_HOST=smtp.sendgrid.net
SMTP_PORT=587
SMTP_USERNAME=apikey
SMTP_PASSWORD=your_sendgrid_api_key
EMAIL_FROM=noreply@trackr.io
EMAIL_FROM_NAME=Trackr

# SAML
SAML_CERT_PATH=/etc/trackr/saml/sp-cert.pem
SAML_KEY_PATH=/etc/trackr/saml/sp-key.pem
SAML_ENTITY_ID=https://trackr.io/saml/metadata

# Cache
CACHE_LINK_TTL=5m
CACHE_MAX_ENTRIES=100000

# Rate Limiting
RATE_LIMIT_REDIRECT=10000
RATE_LIMIT_API_READ=1000
RATE_LIMIT_API_WRITE=100

# Logging
LOG_LEVEL=info
LOG_FORMAT=json

# Feature Flags
ENABLE_WEBHOOKS=true
ENABLE_SAML=true
ENABLE_AUDIT_LOGS=true

# Monitoring
ENABLE_METRICS=true
METRICS_PORT=9090

# Development
DEBUG_MODE=false
```

---

## 16. ROUTER CONFIGURATION

```go
// internal/api/router.go
package api

import (
    "github.com/julienschmidt/httprouter"
    "net/http"
)

func NewRouter(deps *Dependencies) *httprouter.Router {
    router := httprouter.New()
    
    // Health & Metrics
    router.GET("/health", wrap(deps.HealthHandler.Check))
    router.GET("/metrics", wrap(deps.MetricsHandler.Export))
    
    // Public redirect endpoint (no auth)
    router.GET("/:short_code", wrap(deps.RedirectHandler.Handle))
    
    // Authentication routes
    router.POST("/api/v1/auth/signup", wrap(deps.AuthHandler.Signup))
    router.POST("/api/v1/auth/login", wrap(deps.AuthHandler.Login))
    router.POST("/api/v1/auth/refresh", wrap(deps.AuthHandler.Refresh))
    router.POST("/api/v1/auth/logout", wrap(deps.AuthHandler.Logout))
    
    // SAML routes
    router.GET("/api/v1/auth/saml/login", wrap(deps.AuthHandler.SAMLLogin))
    router.POST("/api/v1/auth/saml/acs", wrap(deps.AuthHandler.HandleSAMLCallback))
    router.GET("/api/v1/auth/saml/metadata/:org_slug", wrap(deps.AuthHandler.GetSAMLMetadata))
    
    // Protected routes - require authentication
    auth := deps.AuthMiddleware
    tenant := deps.TenantMiddleware
    
    // Organization management
    router.GET("/api/v1/organizations/current", 
        chain(deps.OrgHandler.GetCurrent, auth, tenant))
    router.PATCH("/api/v1/organizations/current",
        chain(deps.OrgHandler.Update, auth, tenant, requireRole("admin", "owner")))
    
    // Domain verification
    router.POST("/api/v1/organizations/domains",
        chain(deps.OrgHandler.AddDomain, auth, tenant, requireRole("admin", "owner")))
    router.POST("/api/v1/organizations/domains/:domain_id/verify",
        chain(deps.OrgHandler.VerifyDomain, auth, tenant, requireRole("admin", "owner")))
    router.GET("/api/v1/organizations/domains",
        chain(deps.OrgHandler.ListDomains, auth, tenant))
    
    // Invite management
    router.POST("/api/v1/invites",
        chain(deps.InviteHandler.Create, auth, tenant, requireRole("admin", "owner")))
    router.GET("/api/v1/invites",
        chain(deps.InviteHandler.List, auth, tenant, requireRole("admin", "owner")))
    router.GET("/api/v1/invites/:invite_id",
        chain(deps.InviteHandler.Get, auth, tenant, requireRole("admin", "owner")))
    router.DELETE("/api/v1/invites/:invite_id",
        chain(deps.InviteHandler.Revoke, auth, tenant, requireRole("admin", "owner")))
    
    // User management
    router.GET("/api/v1/users",
        chain(deps.UserHandler.List, auth, tenant, requireRole("admin", "owner")))
    router.GET("/api/v1/users/:user_id",
        chain(deps.UserHandler.Get, auth, tenant))
    router.PATCH("/api/v1/users/:user_id/role",
        chain(deps.UserHandler.UpdateRole, auth, tenant, requireRole("owner")))
    router.DELETE("/api/v1/users/:user_id",
        chain(deps.UserHandler.Delete, auth, tenant, requireRole("owner")))
    
    // Link management
    router.POST("/api/v1/links",
        chain(deps.LinkHandler.Create, auth, tenant, rateLimit("api_write")))
    router.GET("/api/v1/links",
        chain(deps.LinkHandler.List, auth, tenant, rateLimit("api_read")))
    router.GET("/api/v1/links/:link_id",
        chain(deps.LinkHandler.Get, auth, tenant, rateLimit("api_read")))
    router.PATCH("/api/v1/links/:link_id",
        chain(deps.LinkHandler.Update, auth, tenant, rateLimit("api_write")))
    router.DELETE("/api/v1/links/:link_id",
        chain(deps.LinkHandler.Delete, auth, tenant, rateLimit("api_write")))
    router.GET("/api/v1/links/:link_id/qr",
        chain(deps.LinkHandler.GetQRCode, auth, tenant))
    
    // Analytics
    router.GET("/api/v1/links/:link_id/analytics",
        chain(deps.AnalyticsHandler.GetLinkAnalytics, auth, tenant, rateLimit("analytics")))
    router.GET("/api/v1/links/:link_id/clicks",
        chain(deps.AnalyticsHandler.GetLinkClicks, auth, tenant, rateLimit("analytics")))
    router.GET("/api/v1/analytics/overview",
        chain(deps.AnalyticsHandler.GetOverview, auth, tenant, rateLimit("analytics")))
    
    // Webhooks
    router.POST("/api/v1/webhooks",
        chain(deps.WebhookHandler.Create, auth, tenant, requireRole("admin", "owner")))
    router.GET("/api/v1/webhooks",
        chain(deps.WebhookHandler.List, auth, tenant, requireRole("admin", "owner")))
    router.GET("/api/v1/webhooks/:webhook_id",
        chain(deps.WebhookHandler.Get, auth, tenant, requireRole("admin", "owner")))
    router.PATCH("/api/v1/webhooks/:webhook_id",
        chain(deps.WebhookHandler.Update, auth, tenant, requireRole("admin", "owner")))
    router.DELETE("/api/v1/webhooks/:webhook_id",
        chain(deps.WebhookHandler.Delete, auth, tenant, requireRole("admin", "owner")))
    
    // API Keys
    router.POST("/api/v1/api-keys",
        chain(deps.APIKeyHandler.Create, auth, tenant, requireRole("admin", "owner")))
    router.GET("/api/v1/api-keys",
        chain(deps.APIKeyHandler.List, auth, tenant, requireRole("admin", "owner")))
    router.DELETE("/api/v1/api-keys/:key_id",
        chain(deps.APIKeyHandler.Revoke, auth, tenant, requireRole("admin", "owner")))
    
    // Audit logs
    router.GET("/api/v1/audit-logs",
        chain(deps.AuditHandler.List, auth, tenant, requireRole("admin", "owner")))
    
    // CORS middleware for all routes
    return wrapWithCORS(router)
}

// Helper function to chain middlewares
func chain(handler http.HandlerFunc, middlewares ...func(http.HandlerFunc) http.HandlerFunc) httprouter.Handle {
    for i := len(middlewares) - 1; i >= 0; i-- {
        handler = middlewares[i](handler)
    }
    return wrap(handler)
}

// Convert http.HandlerFunc to httprouter.Handle
func wrap(handler http.HandlerFunc) httprouter.Handle {
    return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
        // Inject params into context
        ctx := context.WithValue(r.Context(), "params", ps)
        handler(w, r.WithContext(ctx))
    }
}

func requireRole(roles ...string) func(http.HandlerFunc) http.HandlerFunc {
    return func(next http.HandlerFunc) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
            claims := r.Context().Value("claims").(*Claims)
            
            allowed := false
            for _, role := range roles {
                if claims.Role == role {
                    allowed = true
                    break
                }
            }
            
            if !allowed {
                http.Error(w, "Forbidden", http.StatusForbidden)
                return
            }
            
            next(w, r)
        }
    }
}

func rateLimit(limitType string) func(http.HandlerFunc) http.HandlerFunc {
    return func(next http.HandlerFunc) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
            tenant := r.Context().Value("tenant").(*TenantContext)
            key := fmt.Sprintf("%s:%s", tenant.OrgID, limitType)
            
            if !rateLimiter.Allow(key, rateLimits[limitType]) {
                w.Header().Set("Retry-After", "60")
                http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
                return
            }
            
            next(w, r)
        }
    }
}
```

---

## 17. AUDIT LOGGING

```go
type AuditLog struct {
    ID             string
    OrganizationID string
    UserID         string
    Action         string // "link.created", "user.invited", "org.updated"
    ResourceType   string // "link", "user", "organization"
    ResourceID     string
    Metadata       map[string]interface{}
    IPAddress      string
    UserAgent      string
    CreatedAt      int64
}

func LogAudit(ctx context.Context, action, resourceType, resourceID string, metadata map[string]interface{}) {
    claims := ctx.Value("claims").(*Claims)
    tenant := ctx.Value("tenant").(*TenantContext)
    
    r := ctx.Value("request").(*http.Request)
    
    log := &AuditLog{
        ID:             generateUUID(),
        OrganizationID: tenant.OrgID,
        UserID:         claims.UserID,
        Action:         action,
        ResourceType:   resourceType,
        ResourceID:     resourceID,
        Metadata:       metadata,
        IPAddress:      extractIP(r),
        UserAgent:      r.UserAgent(),
        CreatedAt:      time.Now().Unix(),
    }
    
    // Async insert
    go func() {
        if err := auditRepo.Insert(log); err != nil {
            logger.Error("Failed to log audit event", "error", err)
        }
    }()
}

// Usage in handlers
func (h *LinkHandler) Create(w http.ResponseWriter, r *http.Request) {
    // ... create link logic ...
    
    LogAudit(r.Context(), "link.created", "link", link.ID, map[string]interface{}{
        "short_code":      link.ShortCode,
        "destination_url": link.DestinationURL,
    })
    
    // ... return response
}
```

---

## 18. SYSTEMD SERVICE CONFIGURATION

```ini
# /etc/systemd/system/trackr.service
[Unit]
Description=Trackr Link Management Platform
After=network.target

[Service]
Type=simple
User=trackr
Group=trackr
WorkingDirectory=/opt/trackr
ExecStart=/opt/trackr/bin/trackr-server
Restart=always
RestartSec=10

# Environment
Environment="TURSO_DB_URL=libsql://..."
Environment="TURSO_AUTH_TOKEN=..."
EnvironmentFile=/etc/trackr/env

# Security
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/trackr /var/log/trackr

# Limits
LimitNOFILE=65536
LimitNPROC=4096

[Install]
WantedBy=multi-user.target
```

```ini
# /etc/systemd/system/trackr-worker.service
[Unit]
Description=Trackr Background Workers
After=network.target trackr.service

[Service]
Type=simple
User=trackr
Group=trackr
WorkingDirectory=/opt/trackr
ExecStart=/opt/trackr/bin/trackr-worker
Restart=always
RestartSec=10

EnvironmentFile=/etc/trackr/env

[Install]
WantedBy=multi-user.target
```

---

## 19. MAKEFILE

```makefile
# Makefile

.PHONY: build run test clean migrate-global migrate-tenant install

# Variables
BINARY_NAME=trackr
WORKER_BINARY=trackr-worker
MIGRATE_BINARY=trackr-migrate
GO=go
GOFLAGS=-v

# Build
build:
	$(GO) build $(GOFLAGS) -o bin/$(BINARY_NAME) cmd/server/main.go
	$(GO) build $(GOFLAGS) -o bin/$(WORKER_BINARY) cmd/worker/main.go
	$(GO) build $(GOFLAGS) -o bin/$(MIGRATE_BINARY) cmd/migrate/main.go

# Run locally
run:
	$(GO) run cmd/server/main.go

# Run with hot reload (requires air)
dev:
	air -c .air.toml

# Tests
test:
	$(GO) test -v -race -coverprofile=coverage.out ./...

test-coverage:
	$(GO) tool cover -html=coverage.out -o coverage.html

# Benchmarks
bench:
	$(GO) test -bench=. -benchmem ./internal/engine/redirect/

# Database migrations
migrate-global-up:
	./bin/$(MIGRATE_BINARY) --target=global --direction=up

migrate-global-down:
	./bin/$(MIGRATE_BINARY) --target=global --direction=down

migrate-tenant-up:
	./bin/$(MIGRATE_BINARY) --target=tenant --org=$(ORG_ID) --direction=up

# Linting
lint:
	golangci-lint run

# Format code
fmt:
	$(GO) fmt ./...
	goimports -w .

# Clean
clean:
	rm -rf bin/
	rm -f coverage.out coverage.html

# Install dependencies
deps:
	$(GO) mod download
	$(GO) mod tidy

# Build for production
build-prod:
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 $(GO) build -ldflags="-s -w" -o bin/$(BINARY_NAME) cmd/server/main.go
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 $(GO) build -ldflags="-s -w" -o bin/$(WORKER_BINARY) cmd/worker/main.go

# Docker build
docker-build:
	docker build -t trackr:latest .

# Deploy
deploy:
	scp bin/$(BINARY_NAME) user@server:/opt/trackr/bin/
	ssh user@server 'sudo systemctl restart trackr'
```

---

## 20. IMPLEMENTATION CHECKLIST

### Phase 1: Core Infrastructure
- [ ] Initialize Go project with proper module structure
- [ ] Set up Turso global database connection
- [ ] Implement SQLite tenant database manager
- [ ] Create database migration system
- [ ] Set up configuration management (Viper/env)
- [ ] Implement logging infrastructure (structured logging)
- [ ] Set up error handling patterns

### Phase 2: Authentication & Authorization
- [ ] Implement JWT token generation and validation
- [ ] Build invite code system
- [ ] Create corporate email validator
- [ ] Implement RBAC middleware
- [ ] Build user registration flow
- [ ] Build user login flow
- [ ] Implement refresh token mechanism
- [ ] Add SAML 2.0 integration
- [ ] Build SAML metadata endpoint

### Phase 3: Multi-Tenant Architecture
- [ ] Implement tenant context middleware
- [ ] Build tenant database connection pooling
- [ ] Create organization onboarding flow
- [ ] Implement domain verification system
- [ ] Build tenant isolation tests

### Phase 4: Link Management
- [ ] Implement short code generator
- [ ] Build link CRUD operations
- [ ] Create link validation logic
- [ ] Implement rules engine (geo/device)
- [ ] Add UTM parameter handling
- [ ] Build link expiry mechanism
- [ ] Implement password-protected links

### Phase 5: High-Performance Redirect Engine
- [ ] Build in-memory link cache
- [ ] Implement redirect handler
- [ ] Create async click logger
- [ ] Integrate GeoIP lookup
- [ ] Build user-agent parser
- [ ] Implement referrer extraction
- [ ] Add redirect performance metrics
- [ ] Optimize SQLite queries with prepared statements

### Phase 6: Analytics System
- [ ] Build click event storage
- [ ] Implement daily stats aggregation worker
- [ ] Create analytics query API
- [ ] Build time-series aggregation
- [ ] Implement geographic analytics
- [ ] Add device/browser analytics
- [ ] Create referrer analytics
- [ ] Build UTM parameter tracking

### Phase 7: API & Webhooks
- [ ] Build REST API with httprouter
- [ ] Implement API key system
- [ ] Add rate limiting middleware
- [ ] Create webhook dispatcher
- [ ] Build webhook retry mechanism
- [ ] Implement HMAC signature verification
- [ ] Add webhook management API

### Phase 8: QR Codes & Utilities
- [ ] Implement QR code generator
- [ ] Build QR code caching
- [ ] Add QR code customization options

### Phase 9: Operations & Monitoring
- [ ] Create health check endpoint
- [ ] Implement Prometheus metrics
- [ ] Build audit logging system
- [ ] Create backup scripts
- [ ] Write deployment automation
- [ ] Set up systemd services
- [ ] Configure Caddy reverse proxy

### Phase 10: Testing & Documentation
- [ ] Write unit tests for core logic
- [ ] Create integration tests
- [ ] Build performance benchmarks
- [ ] Write API documentation
- [ ] Create deployment guide
- [ ] Write operations runbook

---

## FINAL NOTES

This plan provides a complete blueprint for building Trackr as an enterprise-grade link management platform. Key architectural decisions:

1. **Split-Brain Architecture**: Global Turso DB for cross-tenant data + local SQLite per organization ensures data isolation and performance
2. **Sub-50ms Redirects**: In-memory cache → SQLite → async logging achieves target latency
3. **Enterprise-First**: Invite-only, corporate email validation, SAML SSO, and RBAC from day one
4. **Scalable Design**: Connection pooling, prepared statements, batch inserts, and worker processes handle growth
5. **Production-Ready**: Comprehensive error handling, audit logs, metrics, and operational tooling

The implementation should be done incrementally following the phases, with each phase fully tested before moving to the next. All database queries use parameterized statements to prevent SQL injection, and all user inputs are validated before processing.
