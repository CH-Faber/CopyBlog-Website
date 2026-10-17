# Agenda Foundation and Service Boundary Implementation Plan

> For agentic workers: REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox syntax for tracking.

**Goal:** Establish the first independently deployable Agenda API boundary without breaking the existing website, Android client, Obsidian plugin, or SQLite data.

**Architecture:** Keep the existing Go organizer implementation as the source during this first slice, but make its configuration, database ownership, and HTTP routes Agenda-specific. Add a versioned default workspace migration, expose /api/agenda/v1 beside the old /api/organizer/v1 routes, and move clients to the new route while retaining an explicit compatibility path. The physical service move is performed only after behavior is covered by tests.

**Tech Stack:** Go with the existing SQLite store and net/http; Astro/React/TypeScript; pnpm workspace; Playwright E2E.

**Spec:** docs/superpowers/specs/2026-09-22-agenda-independent-platform-design.md

## Global Constraints

- Agenda owns private data; the personal website must not access the database directly.
- Existing /api/organizer/v1 and /agenda/ behavior remains available during this slice.
- New and migrated items are private by default; this slice does not publish any item.
- AGENDA_* configuration keys take precedence; ORGANIZER_* keys remain supported as migration aliases.
- No production code is written before a failing test demonstrates the requested behavior.
- Do not reset, stash, or overwrite unrelated user changes already present in the worktree.
- Do not introduce Google Calendar, CalDAV, PostgreSQL, full registration, or public projection behavior in this plan.

---

## Task 1: Establish the Phase 0 regression and migration test harness

**Files:**
- Create: scripts/organizer/config_test.go
- Create: scripts/organizer/migration_test.go
- Modify: scripts/organizer/server_test.go
- Modify: tests/e2e/agenda.spec.ts

**Interfaces:**
- Consumes: existing testServer, requestJSON, bearerJSON, openStore, and the current Playwright login flow.
- Produces: executable tests for configuration precedence, route compatibility, migration idempotence, and browser regression.

- [ ] **Step 1: Write failing configuration-precedence tests**

Use t.Setenv and temporary directories. Assert that AGENDA_PORT wins over ORGANIZER_PORT and that ORGANIZER_PORT is used when AGENDA_PORT is absent.

- [ ] **Step 2: Run the configuration tests and verify the expected red failure**

~~~powershell
go test ./ -run 'TestLoadConfig(PrefersAgendaKeys|FallsBackToOrganizerKeys)' -count=1
~~~

Expected: FAIL because loadConfig currently reads only ORGANIZER_* keys.

- [ ] **Step 3: Write the failing route-alias test**

~~~go
func TestAgendaAndOrganizerRouteAliases(t *testing.T) {
    _, handler, password := testServer(t)
    for _, prefix := range []string{"/api/organizer/v1", "/api/agenda/v1"} {
        response := requestJSON(t, handler, http.MethodPost, prefix+"/auth/login", map[string]string{"password": password}, nil)
        if response.Code != http.StatusOK {
            t.Fatalf("%s: %d %s", prefix, response.Code, response.Body.String())
        }
    }
}
~~~

- [ ] **Step 4: Run the route test and verify the expected red failure**

~~~powershell
go test ./ -run TestAgendaAndOrganizerRouteAliases -count=1
~~~

Expected: FAIL because /api/agenda/v1 is not registered.

- [ ] **Step 5: Preserve the existing browser regression and add a new-prefix observation**

Keep the existing lifecycle test on /agenda/. Add an assertion that a new API request is observed under /api/agenda/v1. Do not remove the old-prefix contract.

- [ ] **Step 6: Run the focused browser test**

~~~powershell
pnpm exec playwright test tests/e2e/agenda.spec.ts --grep "complete item lifecycle" --project=chromium
~~~

Expected before implementation: the old flow remains green and the new observation fails only because the new prefix is absent.

- [ ] **Step 7: Commit the test-only baseline**

~~~powershell
git add scripts/organizer/config_test.go scripts/organizer/migration_test.go scripts/organizer/server_test.go tests/e2e/agenda.spec.ts
git commit -m "test: define Agenda service boundary contracts"
~~~

## Task 2: Add Agenda configuration keys with backward-compatible precedence

**Files:**
- Modify: scripts/organizer/config.go
- Test: scripts/organizer/config_test.go
- Modify: .env.example
- Modify: scripts/organizer/README.md

**Interfaces:**
- Consumes: the failing tests from Task 1.
- Produces: AGENDA_* support with ORGANIZER_* fallback and unchanged runtime values.

- [ ] **Step 1: Add focused key-precedence cases**

Cover Agenda value wins, Organizer fallback, and validation messages that name AGENDA_* settings. Clear both key families before each case.

- [ ] **Step 2: Run the focused tests and verify the intended red failure**

~~~powershell
go test ./ -run 'TestEnvCompat|TestLoadConfigValidationMentionsAgendaNames' -count=1
~~~

Expected: FAIL because no compatibility helper or AGENDA_* lookup exists.

- [ ] **Step 3: Implement explicit precedence**

~~~go
func envCompat(agendaKey, legacyKey, fallback string) string {
    if value := strings.TrimSpace(os.Getenv(agendaKey)); value != "" {
        return value
    }
    if value := strings.TrimSpace(os.Getenv(legacyKey)); value != "" {
        return value
    }
    return fallback
}
~~~

Use it for host, port, database path, attachment directory, backup directory, public origin, password hash, timezone, session days, and max upload size. Keep AI_* and VAPID names unchanged in this slice.

- [ ] **Step 4: Run focused and full Go tests**

~~~powershell
go test ./ -run 'TestEnvCompat|TestLoadConfigValidationMentionsAgendaNames' -count=1
go test ./ -count=1
~~~

Expected: PASS with no new warnings.

- [ ] **Step 5: Update environment documentation**

Add an Agenda section to .env.example using /var/lib/agenda paths. Document that ORGANIZER_* is a temporary alias and AGENDA_* wins.

- [ ] **Step 6: Commit the configuration migration**

~~~powershell
git add scripts/organizer/config.go scripts/organizer/config_test.go .env.example scripts/organizer/README.md
git commit -m "feat: add Agenda configuration namespace"
~~~

## Task 3: Introduce versioned migrations and the default personal workspace

**Files:**
- Create: scripts/organizer/migrations.go
- Create: scripts/organizer/migrations_test.go
- Modify: scripts/organizer/store.go
- Modify: scripts/organizer/models.go
- Modify: scripts/organizer/store_test.go
- Test: scripts/organizer/migration_test.go

**Interfaces:**
- Consumes: current SQLite schema and openStore.
- Produces: idempotent migrations, workspace ownership, and Store.defaultWorkspaceID().

- [ ] **Step 1: Write the failing legacy-database migration test**

Create a temporary legacy database, insert one project, item, capture, and memory, close it, reopen through migrations, and assert defaultWorkspaceID is non-empty, all rows remain, each has that workspace ID, and reopening creates no second default workspace.

- [ ] **Step 2: Run migration tests and verify the expected red failure**

~~~powershell
go test ./ -run 'Test(LegacyDatabaseGetsDefaultWorkspace|MigrationIsIdempotent)' -count=1
~~~

Expected: FAIL because there is no schema version table, workspace table, or WorkspaceID field.

- [ ] **Step 3: Define migration version 1**

Create schema_migrations, users, workspaces, and workspace_members. Create one deterministic default personal user/workspace. Add workspace_id columns to captures, items, projects, memories, correction_events, push_subscriptions, and device_tokens when absent. Existing rows receive the default workspace ID. Check PRAGMA table_info before each ALTER TABLE.

Add a WorkspaceID field to Capture, Item, Project, and Memory with JSON name workspaceId; omit it where the existing client does not need it.

- [ ] **Step 4: Run migrations before normal store use**

Make openStore create schema_migrations, apply migrations in ascending order inside transactions, record versions only after success, and expose Store.defaultWorkspaceID(). Do not delete or rewrite existing data.

- [ ] **Step 5: Update write and read paths**

Update capture, item, project, memory, correction-event, push-subscription, and device-token queries to include and filter by the default workspace ID. Keep handler signatures compatible until Principal is introduced in a later plan.

- [ ] **Step 6: Run migration, store, and server tests**

~~~powershell
go test ./ -run 'Test(LegacyDatabaseGetsDefaultWorkspace|MigrationIsIdempotent|AuthenticatedCaptureFlow|DeviceTokenCanUseOrganizerAPI)' -count=1
go test ./ -count=1
~~~

Expected: PASS, with existing behavior unchanged and migrated records attached to the default workspace.

- [ ] **Step 7: Commit the migration slice**

~~~powershell
git add scripts/organizer/migrations.go scripts/organizer/migrations_test.go scripts/organizer/store.go scripts/organizer/store_test.go scripts/organizer/migration_test.go scripts/organizer/models.go
git commit -m "feat: add versioned Agenda workspace migration"
~~~

## Task 4: Register the new Agenda API prefix while preserving the organizer prefix

**Files:**
- Create: scripts/organizer/routes.go
- Modify: scripts/organizer/server.go
- Modify: scripts/organizer/server_test.go
- Modify: scripts/organizer/deploy/nginx-organizer-snippet.conf

**Interfaces:**
- Consumes: route-alias tests and the existing authorized wrapper.
- Produces: registerAgendaRoutes(mux *http.ServeMux, prefix string), used for /api/agenda/v1 and /api/organizer/v1.

- [ ] **Step 1: Add representative alias tests**

Test POST /auth/login, GET /session, POST /captures, and GET /items through both prefixes. Assert equivalent status codes and JSON fields. Add a test that unknown paths under either prefix return 404.

- [ ] **Step 2: Run route tests and verify the expected red failure**

~~~powershell
go test ./ -run 'TestAgendaAndOrganizerRouteAliases|TestAgendaRouteUnknownPath' -count=1
~~~

Expected: new-prefix cases fail before implementation.

- [ ] **Step 3: Extract route registration without changing handlers**

Move current v1 registrations into one helper. Register login, device login, session, captures, items, projects, memories, push, export, and password for both prefixes. Do not duplicate handlers or change payloads.

- [ ] **Step 4: Add the Nginx compatibility location**

Add a specific /api/agenda/ location proxying to the same port with current headers and timeout. Keep /api/organizer/ as the old-client location.

- [ ] **Step 5: Run all Go tests**

~~~powershell
go test ./ -count=1
~~~

Expected: PASS, including old and new prefix tests.

- [ ] **Step 6: Commit the API boundary**

~~~powershell
git add scripts/organizer/routes.go scripts/organizer/server.go scripts/organizer/server_test.go scripts/organizer/deploy/nginx-organizer-snippet.conf
git commit -m "feat: expose Agenda API route namespace"
~~~

## Task 5: Move client defaults to the Agenda API with explicit compatibility

**Files:**
- Modify: src/lib/organizer-api.ts
- Modify: src/lib/agenda-native.ts
- Modify: scripts/obsidian-sync-plugin/src/organizer-api-client.ts
- Modify: scripts/obsidian-sync-plugin/main.ts
- Modify: tests/e2e/agenda.spec.ts
- Modify: astro.config.mjs
- Modify: apps/agenda-android/vite.config.ts

**Interfaces:**
- Consumes: /api/agenda/v1 and current runtime configuration.
- Produces: an agendaApiBase default, an explicit legacy override, and no hard-coded Android dependency on /api/organizer/v1.

- [ ] **Step 1: Write pure base URL resolver tests**

Test these exact cases:

~~~ts
resolveAgendaApiBase({}) === "/api/agenda/v1"
resolveAgendaApiBase({ legacy: true }) === "/api/organizer/v1"
resolveAgendaApiBase({ configured: "https://agenda.example/api/agenda/v1/" }) === "https://agenda.example/api/agenda/v1"
~~~

- [ ] **Step 2: Run the client test and verify the expected red failure**

~~~powershell
pnpm exec vitest run src/lib/agenda-api-base.test.ts
~~~

Expected: FAIL because the resolver and test setup do not exist. If Vitest is unavailable, add the smallest runner only after confirming the failure is a missing runner.

- [ ] **Step 3: Implement base URL resolution**

Make the browser default /api/agenda/v1, normalize one trailing slash, and allow PUBLIC_AGENDA_API_BASE_URL. Keep an explicit legacyOrganizerApi option for rollback; do not probe two endpoints for every request. Change Android’s production base to https://faberhu.top/api/agenda/v1 and preserve stored tokens. Update the Obsidian plugin’s description and default endpoint while accepting a user-supplied legacy value.

- [ ] **Step 4: Update browser proxy and E2E observations**

Proxy /api/agenda to the local Go server in astro.config.mjs. Observe /api/agenda/v1 in new E2E coverage and retain one compatibility test for /api/organizer/v1.

- [ ] **Step 5: Run TypeScript checks, build, and focused E2E tests**

~~~powershell
pnpm check
pnpm build
pnpm exec playwright test tests/e2e/agenda.spec.ts --project=chromium
~~~

Expected: PASS with the browser using the new route and the old route available.

- [ ] **Step 6: Commit the client migration**

~~~powershell
git add src/lib/organizer-api.ts src/lib/agenda-native.ts scripts/obsidian-sync-plugin/src/organizer-api-client.ts scripts/obsidian-sync-plugin/main.ts tests/e2e/agenda.spec.ts astro.config.mjs apps/agenda-android/vite.config.ts
git commit -m "feat: migrate clients to Agenda API namespace"
~~~

## Task 6: Make the API service independently runnable and document deployment

**Files:**
- Create: services/agenda-api/README.md
- Create: services/agenda-api/Makefile
- Modify: scripts/organizer/README.md
- Modify: scripts/organizer/deploy/nginx-organizer-snippet.conf
- Modify: .env.example
- Modify: package.json
- Modify: pnpm-workspace.yaml

**Interfaces:**
- Consumes: tested Go service and AGENDA_* configuration.
- Produces: independent start/test/build/migrate commands and a migration-safe deployment entry point. The physical move from scripts/organizer to services/agenda-api occurs only after previous tasks are green and preserves user changes.

- [ ] **Step 1: Write a bounded health/version smoke test**

Start the service with a temporary database and assert GET /healthz returns 200 and GET /version returns a non-empty version. Use a context timeout.

- [ ] **Step 2: Run the smoke test and verify the expected red failure**

~~~powershell
go test ./ -run TestServiceHealthAndVersion -count=1
~~~

Expected: FAIL because only the existing organizer health path exists.

- [ ] **Step 3: Implement health, readiness, and version endpoints**

Add /healthz returning {"status":"ok"}, /readyz checking database and attachment directory, and /version returning {"service":"agenda-api","version":"dev","schemaVersion":1}. Keep the old health endpoint.

- [ ] **Step 4: Add independent service commands**

The service Makefile exposes test, build, run, and migrate. Root package scripts may provide agenda:api:test and agenda:api:build, but must not silently deploy or delete a server directory.

- [ ] **Step 5: Move the Go module only after all tests pass**

Use git mv, then update references found by rg -n "scripts/organizer|/api/organizer|ORGANIZER_" . --glob "!node_modules/**" --glob "!dist/**". Keep old URL and configuration aliases, then rerun tests from the new directory.

- [ ] **Step 6: Verify the complete foundation slice**

~~~powershell
go test ./...
pnpm check
pnpm build
pnpm exec playwright test tests/e2e/agenda.spec.ts --project=chromium
~~~

Expected: all commands exit 0. Record unrelated failures separately instead of weakening tests.

- [ ] **Step 7: Commit the independent service boundary**

~~~powershell
git add services/agenda-api scripts/organizer .env.example package.json pnpm-workspace.yaml
git commit -m "refactor: make Agenda API independently runnable"
~~~

## Task 7: Final foundation review and handoff

**Files:**
- Create: docs/agenda/foundation-migration.md
- Modify: docs/superpowers/specs/2026-09-22-agenda-independent-platform-design.md only if implementation decisions materially changed the approved design.

**Interfaces:**
- Consumes: green outputs from Tasks 1–6.
- Produces: operator documentation and a clear boundary for the next publication plan.

- [ ] Step 1: Document migration and rollback commands

Document go run . migrate, go test ./..., AGENDA_DATABASE_PATH, AGENDA_ATTACHMENT_DIR, AGENDA_BACKUP_DIR, the new route, and the compatibility route.

- [ ] Step 2: Verify the acceptance checklist

Evidence must cover: API starts without Astro; both prefixes return equivalent private responses; a legacy SQLite database migrates without data loss; migrated rows have the default workspace ID; Web, Android, and Obsidian flows remain usable; no public endpoint exposes private items; health/readiness/version work.

- [ ] Step 3: Run final verification before claiming completion

~~~powershell
git diff --check
go test ./...
pnpm check
pnpm build
pnpm exec playwright test tests/e2e/agenda.spec.ts --project=chromium
~~~

Do not claim the foundation is complete unless every command exits 0 and the acceptance checklist is evidenced.

- [ ] Step 4: Commit the handoff documentation

~~~powershell
git add docs/agenda/foundation-migration.md
git commit -m "docs: record Agenda foundation migration"
~~~

## Follow-up plans

After this plan, create separate plans so each subsystem remains independently testable:

1. agenda-public-projection-plan.md — publications, public entries, public API, website schedule, and ICS export.
2. agenda-client-extraction-plan.md — contracts, shared client, shared UI, standalone Agenda Web, and website removal of private UI.
3. agenda-android-platform-plan.md — platform adapters, notification reconciliation, notification deep links, and offline behavior.
4. agenda-calendar-read-plan.md — ICS import, external events, provider abstraction, and read-only calendar views.
5. agenda-open-source-multitenancy-plan.md — initialization, user/workspace auth, Docker Compose, self-hosting, and PostgreSQL option.

Each follow-up plan must read the updated implementation state and the original spec before defining file-level tasks.

