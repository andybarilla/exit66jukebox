---
status: in-progress
phase: 1
updated: 2026-06-19
---

# Auth MFA TOTP Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add optional TOTP MFA with single-use recovery codes and an admin MFA policy while preserving unchanged login behavior for users without MFA.

**Architecture:** Keep pure MFA primitives in `internal/auth`, persistence in `internal/store`, and HTTP/session flow in `internal/api`. MFA login issues a short-lived hashed ticket instead of a session, then creates the normal `exit66_session` cookie only after TOTP or recovery verification. TOTP secrets are AES-GCM encrypted using env-only `EXIT66_MFA_KEY` material and stored in SQLite as ciphertext, nonce, and key version.

**Tech Stack:** Go stdlib crypto, modernc SQLite, Svelte 5 + Vite + Vitest, existing Makefile build/test commands.

## Global Constraints

- Do not implement SMS/email MFA, WebAuthn/passkeys, OAuth/OIDC, or fine-grained permissions.
- Use RFC 6238 defaults: 160-bit random base32 secret, 6 digits, 30-second step, SHA-1.
- Accept TOTP codes within ±1 time step; track `last_accepted_step` and reject same-or-older steps.
- Read MFA encryption material only from `EXIT66_MFA_KEY`; accept only 32-byte base64 or hex.
- Use AES-GCM stdlib; do not reuse `app_secret`; store ciphertext/nonce/key version in SQLite.
- MFA login response for enabled users is `{"mfa_required": true, "ticket": "..."}` and sets no session cookie.
- MFA tickets are random, hashed, short-lived, and single-use.
- Recovery codes are CSPRNG-generated, normalized, hashed, shown once, transactional, and single-use.
- Disable MFA and recovery-code regeneration require password plus current TOTP or unused recovery code.
- Enforce `admin_mfa_required` in `requireAdmin`; admins without MFA may sign in as normal users to enroll, but admin routes return 403.
- Admin MFA policy changes must prevent locking out all admins and require current admin MFA.
- Do not log TOTP secrets, otpauth URIs, recovery codes, MFA tickets, or plaintext recovery input.
- Do not commit untracked root `package-lock.json` unless frontend dependency changes intentionally require it.

---

## Goal

Add optional TOTP MFA, recovery codes, and an admin MFA requirement that protects admin routes without changing non-MFA login behavior.

## Context & Decisions

| Decision | Rationale | Source |
|----------|-----------|--------|
| Follow existing Go/Svelte layering: `internal/auth` for primitives, `internal/store` for SQLite, `internal/api` for handlers, and `web/src` for UI/API wrappers. | The repo already uses these seams for password hashing, sessions, settings, admin handlers, and frontend auth state. | `ref:subtle-harlequin-puma` |
| Use additive schema/migration changes in embedded SQL plus guarded `ALTER TABLE ADD COLUMN` checks. | The store has no versioned migrations and uses idempotent schema/migration helpers. | `ref:subtle-harlequin-puma` |
| Fix current auth primitives before exposing MFA flows. | Existing partial commit has constant-time recovery hash comparison and TOTP URI escaping issues that must be corrected first. | `ref:liable-tan-marmot` |

## Phase 1: Existing MFA primitive hardening [IN PROGRESS]

### Task 1.1: Fix recovery-code verification and TOTP URI escaping ← CURRENT

**Files:** `internal/auth/recovery.go`, `internal/auth/recovery_test.go`, `internal/auth/totp.go`, `internal/auth/totp_test.go`

**Interfaces:** Consumes existing `VerifyRecoveryCode`, `HashRecoveryCode`, and `TOTPURI`; produces constant-time recovery-code hash checks and correct otpauth label escaping.

- [ ] **1.1.1 Write failing tests** for malformed recovery hashes, decoded hash comparison via `subtle.ConstantTimeCompare`, and a URI label containing `otpauth://totp/Exit66%3AHub:admin%2Btest%40example.com`.
- [ ] **1.1.2 Run targeted failing tests:** `go test ./internal/auth -run 'Test.*Recovery|Test.*TOTPURI'`; expected FAIL on the new expectations.
- [ ] **1.1.3 Implement fixes:** use `subtle.ConstantTimeCompare` on decoded hashes; escape issuer/account independently and join with one literal colon.
- [ ] **1.1.4 Run passing tests:** `go test ./internal/auth`; expected PASS.
- [ ] **1.1.5 Commit:** `git add internal/auth/recovery.go internal/auth/recovery_test.go internal/auth/totp.go internal/auth/totp_test.go && git commit -m "fix(auth): harden MFA primitives"`.

## Phase 2: MFA key config and encryption [PENDING]

### Task 2.1: Add env-only MFA key parsing

**Files:** `internal/config/config.go`, `internal/config/config_test.go`

**Interfaces:** Produces `Config.MFAKey []byte` and `LoadMFAKey(value string) ([]byte, error)` accepting 32-byte base64 or hex only.

- [ ] **2.1.1 Write failing tests** for valid base64, valid hex, missing key returning nil, and invalid length returning an error containing `EXIT66_MFA_KEY must be 32 bytes`.
- [ ] **2.1.2 Run failing tests:** `go test ./internal/config -run MFAKey`; expected FAIL because symbols do not exist.
- [ ] **2.1.3 Implement parsing** without SQLite or `app_secret` fallback.
- [ ] **2.1.4 Run passing tests:** `go test ./internal/config -run MFAKey`; expected PASS.
- [ ] **2.1.5 Commit:** `git add internal/config/config.go internal/config/config_test.go && git commit -m "feat(auth): load MFA encryption key"`.

### Task 2.2: Add AES-GCM TOTP secret encryption helpers

**Files:** create `internal/auth/mfacrypto.go`, `internal/auth/mfacrypto_test.go`

**Interfaces:** Produces `EncryptedSecret`, `EncryptTOTPSecret(key []byte, secret string, keyVersion int) (EncryptedSecret, error)`, and `DecryptTOTPSecret(key []byte, enc EncryptedSecret) (string, error)`.

- [ ] **2.2.1 Write failing tests** for round trip, random nonce/ciphertext, wrong-key failure, and invalid key length failure.
- [ ] **2.2.2 Run failing tests:** `go test ./internal/auth -run 'MFA|Encrypt|Decrypt'`; expected FAIL.
- [ ] **2.2.3 Implement AES-GCM helpers** with errors that mention `MFA encryption key` and contain no secret material.
- [ ] **2.2.4 Run passing tests:** `go test ./internal/auth -run 'MFA|Encrypt|Decrypt'`; expected PASS.
- [ ] **2.2.5 Commit:** `git add internal/auth/mfacrypto.go internal/auth/mfacrypto_test.go && git commit -m "feat(auth): encrypt TOTP secrets"`.

## Phase 3: Store schema, migrations, settings, and MFA persistence [PENDING]

### Task 3.1: Add MFA schema and admin setting

**Files:** `internal/store/schema.sql`, `internal/store/migrate.go`, `internal/store/settings.go`, `internal/store/settings_test.go`

**Interfaces:** Produces `AdminMFARequired(ctx context.Context) (bool, error)`, `SetAdminMFARequired(ctx context.Context, enabled bool) error`, and tables `mfa_factor`, `mfa_ticket`, `mfa_recovery_code`.

- [ ] **3.1.1 Write failing tests** proving fresh and migrated in-memory databases have MFA tables/indexes and default `admin_mfa_required=false`.
- [ ] **3.1.2 Run failing tests:** `go test ./internal/store -run 'MFA|Settings|Migration'`; expected FAIL.
- [ ] **3.1.3 Implement idempotent schema/migration** using `CREATE TABLE IF NOT EXISTS`, `CREATE INDEX IF NOT EXISTS`, and guarded `ALTER TABLE ADD COLUMN` only if needed.
- [ ] **3.1.4 Run passing tests:** `go test ./internal/store -run 'MFA|Settings|Migration'`; expected PASS.
- [ ] **3.1.5 Commit:** `git add internal/store/schema.sql internal/store/migrate.go internal/store/settings.go internal/store/settings_test.go && git commit -m "feat(auth): add MFA store schema"`.

### Task 3.2: Add store MFA helper methods

**Files:** create `internal/store/mfa.go`, `internal/store/mfa_test.go`

**Interfaces:** Produces `MFAFactor`, `GetMFAFactor`, `UpsertMFAFactor`, `DisableMFAFactor`, `UpdateMFALastAcceptedStep`, `CreateMFATicket`, `ConsumeMFATicket`, `ReplaceRecoveryCodes`, `ListRecoveryCodeHashes`, and `MarkRecoveryCodeUsed`.

- [ ] **3.2.1 Write failing tests** for factor lifecycle, ticket single-use/expiry, recovery-code replacement, and recovery-code single-use marking.
- [ ] **3.2.2 Run failing tests:** `go test ./internal/store -run MFA`; expected FAIL.
- [ ] **3.2.3 Implement helpers** with context-aware DB calls, hashed ticket storage, and transactions for recovery-code replacement/use.
- [ ] **3.2.4 Run passing tests:** `go test ./internal/store -run MFA`; expected PASS.
- [ ] **3.2.5 Commit:** `git add internal/store/mfa.go internal/store/mfa_test.go && git commit -m "feat(auth): persist MFA factors and challenges"`.

## Phase 4: Backend MFA login and account endpoints [PENDING]

### Task 4.1: Change login to issue MFA challenge before session

**Files:** `internal/api/auth.go`, `internal/api/auth_test.go`, `internal/api/server.go`

**Interfaces:** Non-MFA login returns existing user JSON; MFA login returns `{"mfa_required": true, "ticket": "..."}` with no `Set-Cookie`; `POST /api/auth/mfa/complete` sets session after valid `{ "ticket": string, "code": string }`.

- [ ] **4.1.1 Write failing tests** for non-MFA login unchanged, MFA login no session cookie, valid TOTP completes session, reused/expired ticket rejected, reused time step rejected, and recovery code completes session once.
- [ ] **4.1.2 Run failing tests:** `go test ./internal/api -run 'Login|MFAComplete'`; expected FAIL.
- [ ] **4.1.3 Implement challenge flow** with existing `allowAttempt`, hashed tickets, ticket single-use, recovery single-use, and no secret/code/ticket logging.
- [ ] **4.1.4 Run passing tests:** `go test ./internal/api -run 'Login|MFAComplete'`; expected PASS.
- [ ] **4.1.5 Commit:** `git add internal/api/auth.go internal/api/auth_test.go internal/api/server.go && git commit -m "feat(auth): require MFA challenge before session"`.

### Task 4.2: Add enrollment, confirmation, disable, and recovery-code regeneration endpoints

**Files:** `internal/api/auth.go`, `internal/api/auth_test.go`, `internal/api/server.go`

**Interfaces:** Produces `POST /api/auth/mfa/enroll/begin`, `POST /api/auth/mfa/enroll/confirm`, `POST /api/auth/mfa/disable`, `POST /api/auth/mfa/recovery/regenerate`; begin returns `{ "secret": string, "otpauth_uri": string }`; confirm/regenerate return `{ "recovery_codes": []string }` shown once.

- [ ] **4.2.1 Write failing tests** for missing/invalid `EXIT66_MFA_KEY`, valid confirm, recovery codes shown once and stored hashed, disable/regenerate requiring password plus TOTP/recovery, and no plaintext values in logs.
- [ ] **4.2.2 Run failing tests:** `go test ./internal/api -run 'MFAEnroll|MFADisable|Recovery'`; expected FAIL.
- [ ] **4.2.3 Implement endpoints** using encrypted TOTP secret, transactional recovery-code replacement, and existing `currentUser` session checks.
- [ ] **4.2.4 Run passing tests:** `go test ./internal/api -run 'MFAEnroll|MFADisable|Recovery'`; expected PASS.
- [ ] **4.2.5 Commit:** `git add internal/api/auth.go internal/api/auth_test.go internal/api/server.go && git commit -m "feat(auth): add MFA account endpoints"`.

## Phase 5: Admin MFA policy and enforcement [PENDING]

### Task 5.1: Enforce admin MFA requirement in `requireAdmin`

**Files:** `internal/api/admin.go`, `internal/api/admin_test.go`, `internal/api/adminusers.go`, `internal/api/adminusers_test.go`

**Interfaces:** Produces server-side 403 for admin users without MFA when `admin_mfa_required=true`; admin payloads include `admin_mfa_required` and per-user `mfa_enabled`.

- [ ] **5.1.1 Write failing tests** for admin without MFA getting 403 on admin routes, normal authenticated routes still working, admin with MFA allowed, and user list including `mfa_enabled`.
- [ ] **5.1.2 Run failing tests:** `go test ./internal/api -run 'Admin.*MFA|RequireAdmin|AdminSettings|ListUsers'`; expected FAIL.
- [ ] **5.1.3 Implement `requireAdmin` policy check** after `currentUser`/`isAdmin`; return 403 without clearing normal session.
- [ ] **5.1.4 Run passing tests:** `go test ./internal/api -run 'Admin.*MFA|RequireAdmin|AdminSettings|ListUsers'`; expected PASS.
- [ ] **5.1.5 Commit:** `git add internal/api/admin.go internal/api/admin_test.go internal/api/adminusers.go internal/api/adminusers_test.go && git commit -m "feat(auth): enforce admin MFA policy"`.

### Task 5.2: Add safe admin policy mutation

**Files:** `internal/api/adminusers.go`, `internal/api/adminusers_test.go`

**Interfaces:** Consumes `SetAdminMFARequired` and current-user MFA status; produces policy update that requires current admin MFA and at least one MFA-enabled admin before enabling.

- [ ] **5.2.1 Write failing tests** for enabling denied when current admin lacks MFA, enabling denied when no admin has MFA, enabling succeeds when current admin has MFA, and disabling allowed for current admin.
- [ ] **5.2.2 Run failing tests:** `go test ./internal/api -run 'AdminSettings.*MFA|Lockout'`; expected FAIL.
- [ ] **5.2.3 Implement mutation checks** in `setAdminSettings`; return 400 for lockout risk and 403 when current admin lacks MFA.
- [ ] **5.2.4 Run passing tests:** `go test ./internal/api -run 'AdminSettings.*MFA|Lockout'`; expected PASS.
- [ ] **5.2.5 Commit:** `git add internal/api/adminusers.go internal/api/adminusers_test.go && git commit -m "feat(auth): prevent admin MFA lockout"`.

## Phase 6: Frontend auth, account, and admin UI [PENDING]

### Task 6.1: Add frontend auth helper coverage for MFA flows

**Files:** `web/src/lib/auth.js`, `web/src/lib/auth.test.js`, `web/src/lib/store.svelte.js`

**Interfaces:** Produces `completeMfaLogin`, `beginMfaEnrollment`, `confirmMfaEnrollment`, `disableMfa`, and `regenerateRecoveryCodes`; consumes `mfa_required` without setting authenticated state until completion.

- [ ] **6.1.1 Write failing Vitest tests** for MFA challenge login, complete login state, enrollment helpers, disable/regenerate helpers, and admin settings with `admin_mfa_required`.
- [ ] **6.1.2 Run failing tests:** `npm test --prefix web -- --run web/src/lib/auth.test.js`; expected FAIL.
- [ ] **6.1.3 Implement helpers/state** with existing fetch conventions.
- [ ] **6.1.4 Run passing tests:** `npm test --prefix web -- --run web/src/lib/auth.test.js`; expected PASS.
- [ ] **6.1.5 Commit:** `git add web/src/lib/auth.js web/src/lib/auth.test.js web/src/lib/store.svelte.js && git commit -m "feat(web): add MFA auth helpers"`.

### Task 6.2: Add login challenge, account MFA, and admin policy UI

**Files:** `web/src/lib/components/Login.svelte`, create `web/src/lib/components/MfaAccount.svelte`, `web/src/lib/components/AdminPanel.svelte`, component/source-string tests following current repo conventions.

**Interfaces:** Consumes Task 6.1 helpers; produces UI for challenge code entry, recovery-code display once, disable/regenerate verification, per-user MFA status, and `admin_mfa_required`.

- [ ] **6.2.1 Write failing frontend tests** proving MFA challenge renders after login, enrollment displays URI/secret and one-time recovery codes, disable/regenerate require password plus code, and admin policy toggle is present.
- [ ] **6.2.2 Run failing tests:** `npm test --prefix web -- --run`; expected FAIL on missing MFA UI strings/flows.
- [ ] **6.2.3 Implement UI** with labels `Authenticator code`, `Recovery code`, `Enable MFA`, `Disable MFA`, and `Require MFA for admin access`.
- [ ] **6.2.4 Run passing tests:** `npm test --prefix web -- --run`; expected PASS.
- [ ] **6.2.5 Commit:** `git add web/src/lib/components/Login.svelte web/src/lib/components/MfaAccount.svelte web/src/lib/components/AdminPanel.svelte web/src/lib && git commit -m "feat(web): add MFA account UI"`.

## Phase 7: Embedded assets, full verification, and handoff [PENDING]

### Task 7.1: Rebuild embedded frontend assets

**Files:** `internal/web/dist/**` or repo-specific embedded frontend output generated by `make build`; do not add root `package-lock.json` unless intentionally required.

**Interfaces:** Consumes completed implementation; produces rebuilt web assets embedded for Go binary builds.

- [ ] **7.1.1 Build frontend:** `npm run build --prefix web`; expected PASS.
- [ ] **7.1.2 Build application:** `make build`; expected PASS.
- [ ] **7.1.3 Inspect changes:** `git status --short`; expected only intentional changes and no accidental root `package-lock.json` staged.
- [ ] **7.1.4 Commit assets:** `git add internal/web/dist web/dist 2>/dev/null || true` then `git commit -m "chore(web): rebuild embedded assets"`.

### Task 7.2: Run full verification and prepare PR handoff

**Files:** no production code changes expected.

**Interfaces:** Produces final verification evidence for issue #93 handoff.

- [ ] **7.2.1 Run backend tests:** `go test ./...`; expected PASS.
- [ ] **7.2.2 Run vet:** `go vet ./...`; expected PASS.
- [ ] **7.2.3 Run frontend tests:** `npm test --prefix web -- --run`; expected PASS. If scripts require it, use `npm run test --prefix web -- --run` and record that command in PR notes.
- [ ] **7.2.4 Run full build:** `make build`; expected PASS.
- [ ] **7.2.5 Confirm clean intentional diff:** `git status --short`; expected no unstaged tracked changes and no accidental root `package-lock.json` staged.
- [ ] **7.2.6 Commit verification-only adjustments if any:** `git add docs/superpowers/plans/2026-06-19-auth-mfa-totp.md internal/auth/recovery.go internal/auth/recovery_test.go internal/auth/totp.go internal/auth/totp_test.go internal/config/config.go internal/config/config_test.go internal/store/schema.sql internal/store/migrate.go internal/store/settings.go internal/store/mfa.go internal/store/mfa_test.go internal/store/users.go internal/api/auth.go internal/api/auth_test.go internal/api/admin.go internal/api/adminusers.go internal/api/server.go web/src/lib/auth.js web/src/lib/auth.test.js web/src/lib/components/Login.svelte web/src/lib/components/AdminPanel.svelte internal/web/dist` then `git commit -m "test(auth): verify MFA flows"`; skip when verification produces no file changes.

## Acceptance Coverage

- Non-MFA login unchanged: Task 4.1.
- MFA challenge before session: Task 4.1.
- TOTP skew and replay rejection: Tasks 1.1 and 4.1.
- Recovery codes show once, hash, single-use, regenerate transactionally: Tasks 3.2 and 4.2.
- Disable verification with password plus TOTP/recovery: Task 4.2.
- TOTP secrets encrypted at rest with env-only key outside SQLite: Tasks 2.1, 2.2, 3.2, 4.2.
- Missing/invalid key fails clearly when setup/login needs it: Tasks 2.1 and 4.2.
- Admin policy avoids lockout and requires current admin MFA: Task 5.2.
- Server-side admin enforcement: Task 5.1.
- No logs of secrets, URIs, codes, tickets, or plaintext recovery: Tasks 4.1 and 4.2.
- Backend/frontend tests and rebuilt internal web dist: Tasks 6.1, 6.2, 7.1, and 7.2.

## Notes

- 2026-06-19: Branch `issue-93-auth-mfa-totp` already exists and issue #93 is In Progress.
- 2026-06-19: Partial commit `2cdd6fc Add MFA auth primitives` already added `internal/auth/totp.go`, `internal/auth/totp_test.go`, `internal/auth/recovery.go`, and `internal/auth/recovery_test.go`; harden these before exposing API routes.
- 2026-06-19: Ignore any untracked root `package-lock.json` from setup unless a deliberate dependency change requires it.
