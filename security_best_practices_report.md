# Security Best Practices Audit Report

**Project:** AI Ops Platform (`aiops`)
**Stack:** Go 1.26 + Gin + GORM + PostgreSQL + Redis + JWT (backend) / Vue 3 + TypeScript + Vite + Pinia + Arco Design (frontend)
**Date:** 2026-08-11
**Scope:** Full codebase security review against Go and Vue security best practices specs

---

## Executive Summary

The codebase demonstrates a **mature security posture** with strong defense-in-depth patterns in many critical areas. The project correctly uses bcrypt for password hashing, `crypto/rand` for all security tokens, parameterized SQL queries, JWT with algorithm confusion prevention, AES-256-GCM for credential encryption, and robust SSRF protection in the AI toolgateway. No SQL injection, OS command injection, path traversal, XSS, or template injection vulnerabilities were found.

However, **15 findings** were identified across the Go backend and Vue frontend, ranging from High to Low severity. The most critical issues are:

1. **No security headers** on either the Go API or the Nginx-served SPA (CSP, X-Frame-Options, X-Content-Type-Options, etc.)
2. **Missing `ReadHeaderTimeout`** on the HTTP server (Slowloris exposure)
3. **No trusted proxy configuration** in Gin (IP spoofing via X-Forwarded-For)
4. **Unbounded request body sizes** on 52+ API handlers
5. **Auth tokens stored in `localStorage`** (XSS token theft risk)
6. **Non-constant-time token comparison** for agent registration

---

## Findings Summary

| ID | Rule | Severity | Component | Description | Status |
|----|------|----------|-----------|-------------|--------|
| F-01 | VUE-HEADERS-001 | **High** | Frontend | No security headers in Nginx config | **Fixed** |
| F-02 | GO-HTTP-001 | **Medium** | Backend | Missing `ReadHeaderTimeout`, `IdleTimeout`, `MaxHeaderBytes` | **Fixed** |
| F-03 | GO-HTTP-003 | **Medium** | Backend | No `SetTrustedProxies` - trusts all proxies | **Fixed** |
| F-04 | GO-HTTP-002 | **Medium** | Backend | 52+ handlers with unbounded request body | **Fixed** |
| F-05 | GO-HTTP-004 | **Medium** | Backend | No security headers in Go middleware | **Fixed** |
| F-06 | GO-AUTH-001 | **Medium** | Backend | Non-constant-time register token comparison | **Fixed** |
| F-07 | VUE-AUTH-001 | **Medium** | Frontend | Tokens stored in `localStorage` | Open |
| F-08 | GO-SSRF-001 | **Low** | Backend | OAuth2/OIDC providers lack SSRF validation | Open |
| F-09 | GO-HTTPCLIENT-001 | **Low** | Backend | OAuth2 client missing TLS min version | Open |
| F-10 | GO-CONFIG-001 | **Low** | Backend | Default secrets in committed example configs | Open |
| F-11 | GO-SUPPLY-001 | **Low** | Backend | No `govulncheck` in CI/Makefile | Open |
| F-12 | VUE-ROUTER-002 | **Low** | Frontend | Login redirect without `/` validation | Open |
| F-13 | VUE-XSS-004 | **Low** | Frontend | OAuth redirect URL used without validation | Open |
| F-14 | VUE-SUPPLY-001 | **Low** | Frontend | No CI/CD, no `npm audit` | Open |
| F-15 | VUE-ROUTER-001 | **Low** | Frontend | Route guards check token presence only | Open |

---

## High Severity

### F-01: No Security Headers in Nginx Configuration (VUE-HEADERS-001)

**Impact:** Without CSP, any XSS vulnerability (even from a third-party dependency) would have unrestricted access to the page. Without `X-Frame-Options`, the application can be embedded in iframes for clickjacking attacks.

**Location:** `web/nginx.conf` (entire file)

**Evidence:**
```nginx
server {
    listen 8080;
    server_name _;
    root /usr/share/nginx/html;
    index index.html;
    location / { try_files $uri $uri/ /index.html; }
    location /api/ { proxy_pass ...; }
    # ... no add_header directives anywhere
}
```

**Missing headers:**
- `Content-Security-Policy` - no CSP, allowing arbitrary inline scripts/styles and external resource loading
- `X-Frame-Options` / `frame-ancestors` - no clickjacking protection
- `X-Content-Type-Options: nosniff` - no MIME type sniffing protection
- `Strict-Transport-Security` - no HSTS (verify if TLS termination is upstream)
- `Referrer-Policy` - no referrer control
- `Permissions-Policy` - no feature policy restrictions

**Fix:** Add security header directives to `web/nginx.conf`:
```nginx
add_header Content-Security-Policy "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; connect-src 'self'; frame-ancestors 'none';" always;
add_header X-Frame-Options "DENY" always;
add_header X-Content-Type-Options "nosniff" always;
add_header Referrer-Policy "strict-origin-when-cross-origin" always;
# Only add HSTS if TLS is terminated at this layer:
# add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
```

**False positive notes:** Verify whether TLS is terminated at Nginx or an upstream load balancer. HSTS should only be added at the TLS termination point.

---

## Medium Severity

### F-02: Missing HTTP Server Timeouts and Header Limits (GO-HTTP-001)

**Impact:** Without `ReadHeaderTimeout`, a client can hold the connection open by slowly sending headers (Slowloris attack), enabling DoS.

**Location:** `internal/server/server.go:28-33`

**Evidence:**
```go
httpSrv: &http.Server{
    Addr:         addr,
    Handler:      engine,
    ReadTimeout:  time.Duration(cfg.ReadTimeoutS) * time.Second,
    WriteTimeout: time.Duration(cfg.WriteTimeoutS) * time.Second,
},
```

`ReadTimeout` and `WriteTimeout` are set (defaults 15s/30s), but three critical fields are **missing**:
- `ReadHeaderTimeout` - not set; Slowloris mitigation gap
- `IdleTimeout` - not set; defaults to `ReadTimeout` (15s), acceptable but should be explicit
- `MaxHeaderBytes` - not set; defaults to 1MB via Gin, should be explicit

**Fix:**
```go
httpSrv: &http.Server{
    Addr:              addr,
    Handler:           engine,
    ReadTimeout:       time.Duration(cfg.ReadTimeoutS) * time.Second,
    ReadHeaderTimeout: 10 * time.Second,
    WriteTimeout:      time.Duration(cfg.WriteTimeoutS) * time.Second,
    IdleTimeout:       60 * time.Second,
    MaxHeaderBytes:    1 << 20, // 1MB
},
```

---

### F-03: No Trusted Proxy Configuration (GO-HTTP-003)

**Impact:** Gin's default behavior when `SetTrustedProxies` is not called is to trust all proxies (`0.0.0.0/0`). An attacker can set arbitrary `X-Forwarded-For` headers to spoof their IP, potentially bypassing IP-based rate limiting or polluting audit logs.

**Location:** `internal/server/router.go:58`

**Evidence:**
```go
r := gin.New()
```

No call to `r.SetTrustedProxies()` is made anywhere in the codebase. `c.ClientIP()` is used at `internal/alert/interfaces/http/handler_ingest.go:96` for audit logging.

**Fix:**
```go
r := gin.New()
// Configure trusted proxies from config, or disable proxy trust:
r.SetTrustedProxies([]string{"127.0.0.1", "10.0.0.0/8"})
// Or if ClientIP should always use direct connection IP:
// r.SetTrustedProxies(nil)
```

**False positive notes:** The Nginx configs do set `X-Forwarded-For $proxy_add_x_forwarded_for`, but without trusted proxy configuration, Gin cannot distinguish between the real Nginx proxy and a direct attacker.

---

### F-04: Unbounded Request Body Sizes on API Handlers (GO-HTTP-002)

**Impact:** Attackers can send very large JSON bodies to exhaust server memory (DoS). Gin's `ShouldBindJSON` uses `json.NewDecoder` which reads the entire body with no size cap.

**Location:** 52+ handlers across the codebase. Examples:

- `internal/identity/interfaces/http/handler.go:108`
- `internal/execution/interfaces/http/handler.go:57`
- `internal/asset/interfaces/http/handler.go:85`

**Evidence:**
```go
if err := c.ShouldBindJSON(&req); err != nil {
```

**Positive note:** Webhook ingestion (`handler_ingest.go:102-111`) and outbound HTTP calls properly use `io.LimitReader` with 1MB cap.

**Fix:** Add a global body size limit middleware:
```go
func MaxBodySize(maxBytes int64) gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
        c.Next()
    }
}
// Apply in router.go:
r.Use(MaxBodySize(10 << 20)) // 10MB
```

---

### F-05: No Security Headers in Go Backend (GO-HTTP-004)

**Impact:** API responses lack `X-Content-Type-Options`, `X-Frame-Options`, and other security headers that provide defense-in-depth.

**Location:** `internal/server/router.go:60-65`

**Evidence:**
```go
r.Use(middleware.Trace())
r.Use(middleware.Recovery())
r.Use(middleware.RequestLog())
r.Use(middleware.CORS(cfg.CORS))
```

No security header middleware exists. Grep for `Content-Security-Policy`, `X-Content-Type-Options`, `X-Frame-Options` returned zero matches in Go source.

**Fix:** Add a security headers middleware:
```go
func SecurityHeaders() gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Header("X-Content-Type-Options", "nosniff")
        c.Header("X-Frame-Options", "DENY")
        c.Header("Cache-Control", "no-store")
        c.Next()
    }
}
```

---

### F-06: Non-Constant-Time Register Token Comparison (GO-AUTH-001)

**Impact:** The agent registration token is compared using `!=` (non-constant-time), which could theoretically allow timing attacks to discover the token byte-by-byte. The register token grants agent enrollment capability.

**Location:** `internal/execution/interfaces/http/agent_handler.go:76`

**Evidence:**
```go
if expected == "" || token != expected {
    httpx.FailWith(c, apperr.CodePermissionDenied, "invalid register token")
    return
}
```

Compare with the webhook secret verification which correctly uses `subtle.ConstantTimeCompare` at `internal/alert/infrastructure/ingest/secret.go:29`.

**Fix:**
```go
if expected == "" || subtle.ConstantTimeCompare([]byte(token), []byte(expected)) != 1 {
    httpx.FailWith(c, apperr.CodePermissionDenied, "invalid register token")
    return
}
```

---

### F-07: Auth Tokens Stored in localStorage (VUE-AUTH-001)

**Impact:** Both access tokens and refresh tokens are stored in `localStorage`, which is accessible to any JavaScript running on the page. If an XSS vulnerability were introduced (even via a third-party dependency), an attacker could exfiltrate both tokens.

**Location:** `web/src/stores/auth.ts:25, 31, 37`

**Evidence:**
```ts
function setToken(t: string) {
    token.value = t
    if (t) localStorage.setItem(TOKEN_KEY, t)        // line 25
    else localStorage.removeItem(TOKEN_KEY)
}
function setRefreshToken(t: string) {
    refreshToken.value = t
    if (t) localStorage.setItem(REFRESH_KEY, t)       // line 31
    else localStorage.removeItem(REFRESH_KEY)
}
function setUser(u: CurrentUser | null) {
    user.value = u
    if (u) localStorage.setItem(USER_KEY, JSON.stringify(u))  // line 37
    else localStorage.removeItem(USER_KEY)
}
```

**Fix:** Migrate refresh token storage to `HttpOnly` cookies set by the backend. Keep access tokens in memory only (Pinia state without `localStorage` persistence). This eliminates the XSS token theft vector. If bearer tokens must remain, keep them short-lived and in memory only, rotating frequently.

---

## Low Severity

### F-08: OAuth2/OIDC Providers Lack SSRF Validation (GO-SSRF-001)

**Impact:** If an admin account is compromised or config is injected, OAuth2/OIDC endpoints could be used for SSRF to internal services.

**Location:**
- `internal/identity/infrastructure/identityprovider/oauth2.go:44, 91, 121`
- `internal/identity/infrastructure/identityprovider/oidc.go:127`

**Evidence:**
```go
// oauth2.go:91 - TokenURL used directly without validation
req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.TokenURL, ...)

// oidc.go:127 - Discovery URL constructed from admin-configured Issuer
discoveryURL := issuer + "/.well-known/openid-configuration"
```

**Assessment:** These URLs are admin-configured (not per-request user input), so risk is low. The AI toolgateway (`internal/ai/toolgateway/outbound.go`) has excellent SSRF protection with `ValidateOutboundURL`, DNS resolution checks, metadata host blocking, and dial-time re-validation. Consider applying similar validation to identity provider URLs.

**Fix:** Apply `ValidateOutboundURL` or similar SSRF validation to OAuth2/OIDC provider URLs before making outbound requests.

---

### F-09: OAuth2 Client Missing TLS Minimum Version (GO-HTTPCLIENT-001)

**Impact:** The OAuth2 HTTP client could negotiate TLS 1.0/1.1 if the identity provider server supports weak protocols.

**Location:** `internal/identity/infrastructure/identityprovider/oauth2.go:44`

**Evidence:**
```go
// OAuth2: NO TLS min version set
client: &http.Client{Timeout: timeout},
```

Compare with OIDC (`oidc.go:49-53`) which correctly sets:
```go
client: &http.Client{
    Timeout: timeout,
    Transport: &http.Transport{
        TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
    },
},
```

**Fix:**
```go
client: &http.Client{
    Timeout: timeout,
    Transport: &http.Transport{
        TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
    },
},
```

---

### F-10: Default Secrets in Committed Example Configs (GO-CONFIG-001)

**Impact:** Example config files contain placeholder secrets that could be accidentally used in production if not overridden.

**Locations:**
- `configs/config.example.yaml:34, 55, 61, 207, 210` - `password: aiops`, `jwt_secret: "please-change-me-in-production"`, `bootstrap_password: admin123`, etc.
- `pkg/config/config.go:521` - `v.SetDefault("database.password", "aiops")`
- `.env.example:32, 53, 55, 61, 69` - Same placeholder values
- `migrations/0016_seed_default_admin_user.up.sql:2, 11` - `bcrypt(admin123)`

**Mitigations in place (GOOD):**
- JWT secret validation (`pkg/config/jwt_secret.go:52-79`) rejects placeholder/weak secrets in non-dev environments
- Bootstrap admin blocked in prod (`config.go:394-399`)
- Default admin locked by migration 0044
- `configs/config.yaml` is in `.gitignore`
- Agent register token validation requires strong token in prod

**Fix:** Consider removing the default `database.password = "aiops"` from code and requiring it to be set explicitly. Ensure production deployments use environment variables or gitignored `.env.production` for all secrets.

---

### F-11: No govulncheck in CI/Makefile (GO-SUPPLY-001)

**Impact:** Known vulnerabilities in Go dependencies may go undetected.

**Location:** `Makefile` (lint target)

**Evidence:** The `lint` target runs `gofmt` + `go vet` + ESLint, but does not include `govulncheck`. No CI configuration for `govulncheck` was found.

**Positive note:** `GOSUMDB` is NOT disabled (defaults to `sum.golang.org`). No `GOINSECURE` or `GONOSUMDB` settings found. Go 1.26 is current.

**Fix:** Add `govulncheck ./...` to the `lint` or `test` Makefile target:
```makefile
lint: ## Run linters
	go fmt ./...
	go vet ./...
	govulncheck ./...
	# ... existing lint steps
```

---

### F-12: Login Redirect Without `/` Validation (VUE-ROUTER-002)

**Impact:** Protocol-relative URLs like `//evil.com` could potentially be interpreted differently by some browsers. The inconsistency with the router guard (which does validate) is a code quality concern.

**Location:** `web/src/views/login/index.vue:423-424` and `:443-444`

**Evidence:**
```js
const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/dashboard'
router.replace(redirect || '/dashboard')
```

Compare with `web/src/router/index.ts:109-110` which does validate:
```js
next({ path: redirect && redirect.startsWith('/') ? redirect : '/dashboard' })
```

**Fix:**
```js
const redirect = typeof route.query.redirect === 'string' && route.query.redirect.startsWith('/')
    ? route.query.redirect
    : '/dashboard'
router.replace(redirect)
```

---

### F-13: OAuth Redirect URL Used Without Client-Side Validation (VUE-XSS-004)

**Impact:** If an attacker could compromise the backend or inject a malicious provider configuration, this could be used as an open redirect.

**Location:** `web/src/views/login/index.vue:458`

**Evidence:**
```js
window.location.href = resp.authorization_url
```

**Assessment:** `resp` comes from `fetchOAuthAuthorizeURL(providerId)`, a trusted backend API. The `authorization_url` is constructed server-side. Low practical risk.

**Fix:** Add a protocol allow-list check before navigation:
```js
const url = new URL(resp.authorization_url)
if (url.protocol !== 'http:' && url.protocol !== 'https:') {
    throw new Error('Invalid authorization URL protocol')
}
window.location.href = resp.authorization_url
```

---

### F-14: No CI/CD Pipeline for Frontend (VUE-SUPPLY-001)

**Impact:** No automated `npm audit` or dependency scanning for the frontend.

**Location:** No `.github/workflows/` directory at project root.

**Positive note:** `package-lock.json` is committed. Dependencies are reasonably recent (Vue 3.5.10, Vite 5.4.8, etc.).

**Fix:** Add a CI pipeline with `npm audit --production` and consider Dependabot/Renovate for automated vulnerability alerts.

---

### F-15: Route Guards Check Token Presence Only (VUE-ROUTER-001)

**Impact:** The route guard only checks token presence (any non-empty string), not validity or permissions. All authenticated users can navigate to all routes client-side, including admin-only pages like `identity/ldap-import` and `identity/access-control`.

**Location:** `web/src/router/index.ts:103-122`

**Assessment:** This is **NOT a security vulnerability** because the backend enforces Bearer Token + RBAC on all protected API endpoints. The client-side guard is purely for UX. However, adding route-level permission checks would improve UX and reduce information exposure.

**Fix:** Add `meta.permission` to routes and check in `beforeEach`:
```ts
if (to.meta.permission && !auth.hasPermission(to.meta.permission as string)) {
    next({ path: '/dashboard' })
    return
}
```

---

## Areas of Strong Security Posture

The following areas were reviewed and found to be **well-secured**:

| Area | Assessment |
|------|------------|
| **SQL Injection** | All queries use GORM parameterized queries. No string concatenation in SQL. |
| **OS Command Injection** | No `exec.Command` or shell execution anywhere. Command policy regex validates against shell metacharacters. |
| **Password Hashing** | bcrypt with cost 12. `bcrypt.CompareHashAndPassword` is constant-time internally. |
| **Cryptographic Randomness** | `crypto/rand` used for all tokens. No `math/rand` found. |
| **JWT Validation** | Algorithm confusion prevention, issuer validation, expiry enforcement, token type validation. |
| **Credential Encryption** | AES-256-GCM with SHA-256 key derivation, random nonce, key versioning. |
| **Webhook Secret Comparison** | SHA-256 hash + `subtle.ConstantTimeCompare`. |
| **CORS Configuration** | Wildcard + credentials prevented at both middleware and config validation layers. |
| **AI Toolgateway SSRF** | Excellent: URL validation, DNS resolution checks, metadata host blocking, dial-time re-validation, redirect blocking. |
| **LDAP Injection Prevention** | `ldap.EscapeFilter(username)` applied to user input in filters. |
| **LDAP/OIDC TLS** | `MinVersion: tls.VersionTLS12` enforced. `InsecureSkipVerify` rejected in prod. |
| **No Debug Endpoints** | No `pprof` or `expvar` exposure. Only `/healthz`, `/readyz`, `/version`. |
| **No File Uploads** | No multipart/form handling. JSON-only REST API. |
| **No Template Rendering** | No server-side HTML rendering. No `text/template` or `html/template` usage. |
| **No Open Redirects (Backend)** | No `http.Redirect` or `c.Redirect` found. |
| **Concurrency Safety** | All mutable shared state protected by mutexes. Global maps are read-only. |
| **No unsafe/cgo** | No `unsafe.Pointer`, `import "C"`, or `// #cgo` found. |
| **Vue XSS** | No `v-html`, `innerHTML`, or DOM injection APIs anywhere. |
| **Vue No Secrets** | No hardcoded secrets, API keys, or tokens in frontend source. |
| **Vue No Console Logging** | Zero `console.log` calls in production source code. |
| **Vue Production Build** | Correct `vite build` with type checking, served via Nginx. No dev server in production. |
| **Vue No Third-Party Scripts** | No external CDN scripts, no dynamic script injection. |

---

## Remediation Priority

1. **Immediate (High):** Add security headers to `web/nginx.conf` (F-01)
2. **Short-term (Medium):** Add `ReadHeaderTimeout` to HTTP server (F-02), configure `SetTrustedProxies` (F-03), add body size limit middleware (F-04), add security headers to Go middleware (F-05), fix register token comparison (F-06)
3. **Medium-term (Medium):** Migrate token storage from `localStorage` to HttpOnly cookies (F-07)
4. **Backlog (Low):** SSRF validation for identity providers (F-08), OAuth2 TLS config (F-09), remove default secrets from code (F-10), add `govulncheck` (F-11), fix login redirect validation (F-12), OAuth URL validation (F-13), add CI/CD (F-14), route-level permissions (F-15)
