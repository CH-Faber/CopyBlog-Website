# Agenda foundation migration handoff

## Runtime boundary

Agenda is now a separately addressable API surface. The same private handlers
are available at `/api/agenda/v1/*` and the compatibility path
`/api/organizer/v1/*`. Web development proxies both prefixes to the Go process;
Nginx has matching locations. Android defaults to
`https://faberhu.top/api/agenda/v1` and keeps its stored device token.

The personal website must not read the Agenda SQLite database directly. Public
projection endpoints remain a subsequent plan; no private `items` filtering is
used as a substitute.

## Configuration and data migration

Use `AGENDA_*` settings for new deployments. Existing `ORGANIZER_*` settings are
temporary aliases and lose to non-empty `AGENDA_*` values. New examples use
`/var/lib/agenda`; existing defaults remain compatible with `/var/lib/faber-organizer`.

Starting the API from `scripts/organizer` runs versioned SQLite migrations before
normal store access. Migration 1 creates a deterministic personal user and
workspace, adds `workspace_id` to core tables, and backfills legacy rows without
deleting data. Reopening the database is idempotent.

```powershell
Set-Location scripts/organizer
go test ./...
go run .
```

The service-only development entry points are also documented in
`services/agenda-api/Makefile`; the physical source move is intentionally staged
for a later cleanup once active user changes have landed.

## Verification endpoints

- `GET /api/agenda/v1/healthz` → `{"status":"ok"}`
- `GET /api/agenda/v1/readyz` → database readiness
- `GET /api/agenda/v1/version` → service and schema version
- Legacy `/api/organizer/v1/*` remains available for rollback and old clients.

## Rollback

Set `PUBLIC_AGENDA_API_BASE_URL=/api/organizer/v1` for the browser, or use the
explicit legacy resolver option in the shared client. Keep the old Nginx location
and `ORGANIZER_*` environment values until all clients have migrated.

## Next plan

The next independent plan should implement an explicit public Agenda projection
(`/api/agenda/public/v1`) with allowlisted fields, publication/revocation state,
expiry, cache policy, and website-only read access. External calendar sync,
multi-user registration, and open-source packaging remain out of this foundation
slice.
