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
