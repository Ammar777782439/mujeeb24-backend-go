# 🔒 Mujeeb24 Security — Pending Vulnerabilities Roadmap

> **Status as of 2026-09-28:** 4 CRITICAL + 4 HIGH + 8 MEDIUM issues were fixed in commit `988ec40` (backend) and `3f1e3e5` (frontend). The issues below are deferred — they require architectural decisions, new dependencies, or migrations that go beyond a security patch.
>
> **Owner:** backend dev + security reviewer.
> **Scope:** this document is the single source of truth for outstanding security work. Cross items off as they get fixed.

---

## 🔴 HIGH — Must fix before public launch

### H-PEND-1: Refresh-token family/chain revocation

**Severity:** HIGH (account takeover via stolen refresh token)
**Audit ref:** auth audit H-3
**Files:**
- `internal/adapters/secondary/persistence/postgres/authentication_repository.go:210` (`Consume`)
- `internal/application/services/authentication.go:73` (issues new session_id, no chain link)

**Current behavior:**
- `RefreshSession.Consume()` only marks the single consumed row revoked.
- The newly issued session has a fresh `session_id` with NO linkage to the consumed one.
- An attacker who steals a refresh cookie and rotates it FIRST (before the legitimate user) receives a new token tied to a fresh `session_id`. The user's eventual `/auth/logout` only knows the original session_id (already consumed) — it cannot revoke the attacker's session.

**Required work:**
1. Add a migration: `ALTER TABLE refresh_sessions ADD COLUMN family_id UUID NOT NULL DEFAULT gen_random_uuid();`
2. Update `Create(...)` to persist `family_id`.
3. Update `Consume()` to:
   - Read the consumed row's `family_id`.
   - Insert the new session with the SAME `family_id`.
   - If the consumed row was already `used_at IS NOT NULL` (replay) → revoke the ENTIRE family (all sessions with same `family_id`) → log `[Auth] REFRESH_FAMILY_REVOKED family=%s triggering_session=%s`.
4. Update tests: `authentication_repository_integration_test.go` + `authentication_http_integration_test.go` to verify family revocation on replay.

**Estimated effort:** 4-6 hours.
**Priority:** P0 — must fix before any production launch with real users.

---

### H-PEND-2: Rate limiting on `/auth/login`, `/auth/refresh`, `/auth/logout`, `/team/invitations/accept`

**Severity:** HIGH (brute-force / credential stuffing)
**Audit ref:** auth audit H-2
**Files:** `internal/adapters/primary/http/middleware/` (no rate-limit middleware exists)

**Current behavior:**
- The OpenAPI contract advertises 429 on `authenticatePrincipal` but the server returns 200/401 forever.
- An attacker can run unlimited brute-force / credential-stuffing attempts against `/auth/login`.

**Required work:**
1. Choose implementation:
   - **Option A:** Redis-backed sliding window (production-grade, distributed).
   - **Option B:** In-memory token bucket (single-instance only — works for now, but breaks on horizontal scale).
2. New middleware: `middleware/rate_limit.go` exposing `RateLimit(perIP, perPrincipal, window, max)` Huma wrapper.
3. Wire into the public auth routes:
   - `/auth/login`: 10 req/min per IP.
   - `/auth/refresh`: 30 req/min per IP.
   - `/auth/logout`: 10 req/min per principal.
   - `/team/invitations/accept`: 5 req/min per IP.
4. Return `429 Too Many Requests` with `Retry-After` header.
5. Log `[RateLimit] THROTTLED ip=%s route=%s count=%d`.

**Estimated effort:** 6-8 hours (Redis variant), 2-3 hours (in-memory variant).
**Priority:** P0 — must fix before any production launch.

---

### H-PEND-3: Frontend — Content Security Policy (CSP) + ErrorBoundary

**Severity:** HIGH (XSS → full account takeover in single-injection scenarios)
**Audit ref:** frontend audit F-CRIT-1, F-HIGH-2
**Files:**
- `/home/z/my-project/repos/mujeeb24-frontend/index.html` (add `<meta http-equiv="Content-Security-Policy">`)
- `/home/z/my-project/repos/mujeeb24-frontend/vite.config.ts` (add `vite-plugin-csp` or roll custom plugin)
- New: `/home/z/my-project/repos/mujeeb24-frontend/src/components/error-boundary.tsx`
- Edit: `/home/z/my-project/repos/mujeeb24-frontend/src/App.tsx` (wrap `<AppRouter />` in `<ErrorBoundary>`)

**Current behavior:**
- No CSP meta tag, no `X-Frame-Options`, no SRI on Google Fonts/GTM.
- No `ErrorBoundary` — uncaught render errors crash the SPA and expose React component stack traces in production builds.

**Required work:**
1. Add `<meta http-equiv="Content-Security-Policy" content="default-src 'self'; script-src 'self' https://www.googletagmanager.com; connect-src 'self' wss:; frame-ancestors 'none'; base-uri 'self'; form-action 'self'">` to `index.html`.
2. Add `X-Frame-Options: DENY` and `X-Content-Type-Options: nosniff` headers via a small vite plugin.
3. Add SRI hashes to Google Fonts and GTM script tags.
4. Create `src/components/error-boundary.tsx` — class component implementing `getDerivedStateFromError` + `componentDidCatch`. Log to backend (`POST /api/v1/client-errors`) with sanitized payload. Render a generic fallback UI in production.
5. Wrap `<AppRouter />` in `<ErrorBoundary>` in `App.tsx`.

**Estimated effort:** 3-4 hours.
**Priority:** P0 — must fix before any customer-facing deployment.

---

## 🟡 MEDIUM — Fix within 2 weeks

### M-PEND-1: AI run trace child tables not scoped by `business_id` at SQL level

**Severity:** MEDIUM (latent IDOR if a future API endpoint surfaces these tables without re-checking ownership)
**Audit ref:** multi-tenancy audit M-1
**Files:** `internal/adapters/secondary/persistence/postgres/ai_run_trace_repository.go:268-577`

**Current behavior:**
- `ListAttempts`, `ListToolCalls`, `ListGeminiInteractions`, `ListCatalogBatches`, `UpdateAttempt`, `UpdateToolCall`, `UpdateCatalogBatch` — all filter by `ai_run_id` only, NOT by `business_id`.
- The parent `ai_runs` table IS scoped at `GetRun:87` and `UpdateRunStatus:178`.
- No HTTP endpoint currently exposes these tables directly. But if a future endpoint (e.g. `GET /businesses/{id}/ai-runs/{run_id}/attempts`) reuses these methods without first validating business ownership, a leaked `run_id` would expose telemetry.

**Required work:**
1. Add `BusinessID string` parameter to all 7 port method signatures on `AIRunRepository`.
2. Update SQL: `WHERE ai_run_id::text = $1 AND EXISTS (SELECT 1 FROM ai_runs r WHERE r.id = ai_run_attempts.ai_run_id AND r.business_id::text = $2)`.
3. Thread `BusinessID` through all callers in `services/auto_reply.go` + `services/merchant_catalog_ai_agent.go`.
4. Update the file's header comment to match implementation (the comment already promises this; the implementation should match).

**Estimated effort:** 3-4 hours.
**Priority:** P1.

---

### M-PEND-2: Pagination cursors unsigned (no HMAC)

**Severity:** MEDIUM (cursor forgery → row-existence probing within own tenant)
**Audit ref:** multi-tenancy audit M-3
**Files:** 9 cursor encode/decode pairs across `internal/adapters/secondary/persistence/postgres/`:
- `customer_runtime_repository.go:223-246`
- `conversation_runtime_repository.go:184-207`
- `lead_repository.go:273-292`
- `catalog_repository.go` (`catalogCursor`, `schemaCursor`)
- `inbox_repositories.go:234-255`
- `automation_repositories.go:261-282`
- `message_repository.go:124-152`
- `authentication_repository.go:102-119`
- `channel_connection_runtime_repository.go:91-114`

**Current behavior:**
- All cursors are `base64.RawURLEncoding.EncodeToString([]byte(time|UUID))`.
- An attacker can craft a forged cursor embedding another tenant's UUID. The list query still filters by `business_id`, so a forged cursor would either return an empty page (silent no-op) or return rows in the attacker's own tenant whose `(updated_at, id)` happen to be less than the forged value.
- Not exploitable for cross-tenant data today, but defense-in-depth gap.

**Required work:**
1. Add a `cursorSigningKey` to config (`CURSOR_SIGNING_KEY` env var, 32 bytes).
2. Refactor all cursor encoders into a shared `signCursor(businessID, payload string) (string, error)` helper that:
   - Builds `payload = businessID + "|" + innerPayload`.
   - Computes `HMAC-SHA256(cursorSigningKey, payload)`.
   - Returns `base64(payload + "|" + hex(hmac))`.
3. Refactor all decoders into `verifyCursor(businessID, cursor string) (innerPayload string, err error)` that:
   - Splits cursor on last `|`.
   - Recomputes HMAC and compares with `hmac.Equal`.
   - Returns error on mismatch.
4. Update all 9 sites.

**Estimated effort:** 4-5 hours.
**Priority:** P1.

---

### M-PEND-3: Refresh-cookie path too narrow for logout cookie clearing

**Severity:** LOW (defense-in-depth)
**Audit ref:** auth audit L-3, M-3
**Files:** `internal/adapters/primary/http/handlers/system_facades.go:209, 162`

**Current behavior:**
- Logout sends `Set-Cookie: mujeeb_refresh=; Path=/api/v1/auth; MaxAge=-1`. The browser will only clear the cookie IF it was set with the same Path. The login + refresh paths use `Path=/api/v1/auth`. Logout uses the same Path. ✅ Aligned.
- SameSite=Strict is the only CSRF defense. No double-submit token, no Origin check.

**Required work:**
1. Add `Origin` / `Referer` header check on `/auth/login` — reject if Origin is not in the configured allowlist (frontend URL).
2. Optionally add a per-session CSRF token for sensitive operations.

**Estimated effort:** 2-3 hours.
**Priority:** P2.

---

### M-PEND-4: JWT missing `aud` and `nbf` validation

**Severity:** MEDIUM (latent cross-service token confusion)
**Audit ref:** auth audit M-4
**Files:** `internal/adapters/secondary/auth/ed25519jwt/token_issuer.go:69-74`

**Current behavior:**
- `VerifyAccessToken` validates `alg` (EdDSA), `iss`, `exp`. Does NOT validate `aud` or `nbf`.
- `IssueAccessToken` does not set `aud` or `nbf`.

**Required work:**
1. Add `Issuer.Audience string` to `Config`.
2. `IssueAccessToken`: set `aud = cfg.Audience` and `nbf = now`.
3. `VerifyAccessToken`: add `jwt.WithAudience(cfg.Audience)` and `jwt.WithNotBefore()`.
4. Update tests.

**Estimated effort:** 1-2 hours.
**Priority:** P2.

---

### M-PEND-5: AutoReply unbounded concurrency (no worker pool)

**Severity:** LOW (cost amplification under webhook flood)
**Audit ref:** injection audit L-4
**Files:** `internal/application/services/webhook_ingestion.go:239` (the goroutine now has recover but still unbounded)

**Current behavior:**
- Each inbound `dm.received` spawns a fresh goroutine with `context.WithTimeout(120s)`.
- No worker pool, no concurrency cap.
- A webhook flood → thousands of concurrent Gemini API calls.

**Required work:**
1. Introduce a bounded worker pool (semaphore with N=10).
2. Alternatively: route AutoReply through the outbox pattern (already exists for other external operations) so retries + concurrency control are handled by the existing outbox worker.

**Estimated effort:** 4 hours (semaphore variant) / 8 hours (outbox variant).
**Priority:** P2.

---

### M-PEND-6: Automation execution Complete() not scoped by business_id

**Severity:** MEDIUM (latent IDOR)
**Audit ref:** multi-tenancy audit M-2
**Files:** `internal/adapters/secondary/persistence/postgres/automation_repositories.go:230-249`

**Current behavior:**
- `Complete()` filters by execution `id` only.
- Caller is `automation_execution_service.go:61` (server-generated UUID).
- No HTTP endpoint exposes this directly, but future admin tools could misuse it.

**Required work:**
1. Add `BusinessID` to `AutomationExecutionPatch` port.
2. Update SQL: `WHERE id = $1::uuid AND business_id = $2::uuid AND result = 'processing'`.
3. Thread `BusinessID` through all callers.

**Estimated effort:** 1-2 hours.
**Priority:** P2.

---

### M-PEND-7: GTM injected without SRI / CSP whitelist

**Severity:** MEDIUM (remote script injection point)
**Audit ref:** frontend audit F-MED-3, F-MED-4
**Files:** `/home/z/my-project/repos/mujeeb24-frontend/src/utils/analytics.ts:36, 47`

**Current behavior:**
- `gtmScript.innerHTML = template\`...(GTM_ID)\`` — GTM_ID from build-time env var.
- GTM `<iframe>` (noscript fallback) has no `sandbox=""` or `referrerPolicy="no-referrer"`.
- No `subresource-integrity` on Google Fonts stylesheet.

**Required work:**
1. Add SRI hash to Google Fonts `<link>` in `index.html`.
2. Whitelist `https://www.googletagmanager.com` in CSP `script-src` (covered by H-PEND-3).
3. Add `sandbox=""` and `referrerPolicy="no-referrer"` to the GTM noscript iframe.

**Estimated effort:** 1 hour.
**Priority:** P2 (paired with H-PEND-3).

---

## 🟢 LOW — Defense in depth (long-tail)

### L-PEND-1: Bcrypt cost = 10 (DefaultCost)

**Audit ref:** auth audit L-4
**Files:** `cmd/bootstrap-principal/main.go:49`, `cmd/bootstrap-platform-admin/main.go:41`, `cmd/seed/main.go:50`
**Required work:** bump to `bcrypt.DefaultCost + 2` (or hard-code 12). Config-driven via `BCRYPT_COST` env var.
**Effort:** 1 hour.
**Priority:** P3.

---

### L-PEND-2: Seed script logs plaintext admin password

**Audit ref:** auth audit L-1
**Files:** `cmd/seed/main.go:1106-1107`
**Required work:**
1. Stop logging the password.
2. Refuse to seed if `APP_ENV != development`.
**Effort:** 30 minutes.
**Priority:** P3.

---

### L-PEND-3: MFA (TOTP) support

**Audit ref:** auth audit L-5
**Required work:**
1. New migration: `mfa_factors` table (`principal_id`, `factor_type='totp'`, `secret_encrypted`, `recovery_codes_hash`, `enabled_at`).
2. New endpoints: `/me/mfa/setup`, `/me/mfa/verify`, `/me/mfa/disable`.
3. New middleware: `RequireMFA` for owner/admin role on sensitive operations.
4. Use `github.com/pquerna/otp` library (well-maintained).
**Effort:** 8-12 hours.
**Priority:** P3.

---

### L-PEND-4: Move Gemini API key from URL query to header-only

**Audit ref:** injection audit L-2
**Files:** `internal/adapters/secondary/ai/gemini/client.go:291`, `client_contracts.go:179`, `batch_client.go:254`, `token_counter.go:130`
**Required work:** drop `?key=` from URL; rely solely on `x-goog-api-key` header.
**Effort:** 30 minutes.
**Priority:** P3.

---

### L-PEND-5: Audit-event writes for all auth transitions

**Audit ref:** auth audit M-6
**Files:** `internal/application/services/authentication.go` (Handle, Rotate, Revoke)
**Required work:** call `AuditEvents.Append(ctx, ports.AuditEventDraft{...})` for:
- `auth.login_succeeded` (principal_id, ip, user_agent)
- `auth.login_failed` (email, ip, user_agent, reason_code)
- `auth.refresh_succeeded` (principal_id, session_id)
- `auth.refresh_failed` (session_id, reason_code)
- `auth.logout_succeeded` (principal_id, session_id)
- `auth.refresh_replay_detected` (session_id, family_id) — once H-PEND-1 is implemented
**Effort:** 3 hours.
**Priority:** P2.

---

### L-PEND-6: `defer rows.Close()` / `defer resp.Body.Close()` errors never checked

**Audit ref:** error-handling audit B-MED-1
**Files:** 60+ `defer Close()` calls across postgres + gemini + socialapi packages.
**Required work:** wrap with `defer func() { if err := rows.Close(); err != nil { log.Printf("[DB] ROWS_CLOSE_FAILED op=%s err=%v", op, err) } }()` — but this is a wide change. Evaluate case-by-case: HTTP body close errors are benign (keep-alive), but pgx rows.Close() errors can indicate incomplete scans.
**Effort:** 4 hours (selective).
**Priority:** P3.

---

## 📋 Implementation order recommendation

1. **H-PEND-3 (CSP + ErrorBoundary)** — fastest, highest defense-in-depth payoff.
2. **H-PEND-2 (Rate limit)** — in-memory variant first, Redis later.
3. **H-PEND-1 (Refresh-token family)** — needs migration + careful test coverage.
4. **M-PEND-1 (AI run trace business_id)** — pure hardening, no behavior change.
5. **M-PEND-2 (Cursor HMAC)** — pure hardening, wide blast radius.
6. **L-PEND-5 (Audit-event auth)** — pairs with H-PEND-1 once family_id exists.
7. Everything else as time permits.

---

## 🛡️ Existing defenses (already in place — do not regress)

These were verified during the audit and should NOT be regressed in future work:

- ✅ **JWT alg-confusion mitigation** — `ed25519jwt/token_issuer.go` explicitly rejects non-EdDSA methods; key is asymmetric (Ed25519) so the public key cannot be abused as an HMAC secret.
- ✅ **Refresh tokens stored SHA-256 hashed** — DB dump yields no usable tokens; 256-bit entropy brute-force-infeasible.
- ✅ **Refresh cookie path scoped to `/api/v1/auth`** — not sent on non-auth requests.
- ✅ **Refresh tokens NOT in JSON login response** — only in HttpOnly cookie.
- ✅ **Single-use refresh enforced atomically at DB layer** — `WHERE revoked_at IS NULL AND used_at IS NULL`.
- ✅ **`bcrypt.CompareHashAndPassword` is constant-time** — no need for separate constant-time check.
- ✅ **Access/refresh tokens NOT logged** — verified across all `log.*` call sites.
- ✅ **All business-owned SQL queries filter by `business_id`** — verified across 31 files in postgres adapter.
- ✅ **All `requireScope` calls re-derive `BusinessID` from principal membership** — URL/body `business_id` is NEVER trusted directly.
- ✅ **Cross-tenant merge is rejected at SQL layer** — `customer_runtime_repository.go:124` enforces `target.business_id = source.business_id`.
- ✅ **AI context builder re-validates ownership** — every catalog item / variant / offer / knowledge document / business policy is independently re-checked against `input.BusinessID` even after Gemini selection.
- ✅ **ReferenceValidator + TenantValidator** — Gemini's selected UUIDs are independently re-verified against DB EXISTS queries scoped by business_id.
- ✅ **SocialAPI webhook signature verified via `hmac.Equal`** (constant-time) on raw body + timestamp.
- ✅ **Webhook `account_id` → `business_id` resolution via unique index** — `uq_channel_connections_provider_account` prevents cross-tenant binding.
- ✅ **Realtime/SSE isolation** — `LocalHub.Broadcast` only delivers to connections where `event.BusinessID == conn.BusinessID`.
- ✅ **Audit log isolation** — `audit_events` always filtered by `business_id`.
- ✅ **No file upload subsystem** — no path traversal attack surface.
- ✅ **No SSRF attack surface** — all outbound HTTP uses config-controlled base URLs; webhook `redirect_uri` requires `https://` + valid host.
- ✅ **SQL injection CLEAN** — all 31 postgres files use parameterized queries; no `fmt.Sprintf` with user data in SQL; `ORDER BY` / `LIMIT` use hard-coded column names + integer params.
- ✅ **Access token in-memory only on the frontend** — never touches localStorage/sessionStorage.
- ✅ **No open redirect** — all `<a href>` use internal anchors or build-time paths.
- ✅ **No postMessage listeners** — no postMessage attack surface on the frontend.
- ✅ **No hardcoded secrets in frontend** — ripgrep returned zero matches for `apiKey=`, `password:`, `secret:`, `JWT_SECRET`, `Authorization: Bearer ...`.

---

## 📊 Audit history

| Date | Auditor | Scope | Findings | Fixed in commit |
|------|---------|-------|----------|------------------|
| 2026-09-28 | GLM-5.2 security auditor | Auth + session + multi-tenancy + injection + webhook + error-handling + frontend | 4 CRITICAL + 11 HIGH + 14 MEDIUM + 7 LOW | 988ec40 (backend), 3f1e3e5 (frontend) — fixes 4 CRITICAL + 4 HIGH + 8 MEDIUM |

Next audit: after H-PEND-1, H-PEND-2, H-PEND-3 are closed. Recommended: re-run all 4 audit agents to verify regressions haven't been introduced.
