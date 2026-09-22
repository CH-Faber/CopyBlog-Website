# Task 2 implementation report

Status: complete

## Changes

- Added `envCompat(agendaKey, legacyKey, fallback)` with trimmed `AGENDA_*` precedence, `ORGANIZER_*` fallback, and fallback default.
- Migrated host, port, database, attachment, backup, public origin, admin password, timezone, session days, and upload limit configuration to the compatibility helper.
- Kept AI and VAPID names unchanged and preserved existing runtime defaults and parsing bounds.
- Updated validation errors to identify the `AGENDA_*` setting being validated.
- Added deterministic helper, precedence, fallback, isolation, and validation tests.
- Documented the Agenda environment namespace and `/var/lib/agenda` paths in `.env.example` and the organizer README.

## Verification

Focused command:

```powershell
go test ./ -run 'TestEnvCompat|TestLoadConfig(PrefersAgendaKeys|FallsBackToOrganizerKeys|AgendaKeysWinAcrossSettings)|TestLoadConfigValidationMentionsAgendaNames' -count=1
```

Exact output:

```text
ok   faber-organizer  0.657s
```

Required brief command (the original command, without the additional cross-setting test name):

```powershell
go test ./ -run 'TestEnvCompat|TestLoadConfig(PrefersAgendaKeys|FallsBackToOrganizerKeys)|TestLoadConfigValidationMentionsAgendaNames' -count=1
```

Exact output:

```text
ok   faber-organizer  0.657s
```

Full command:

```powershell
go test ./ -count=1
```

Result: failed for two pre-existing/unrelated worktree tests. `TestAgendaAndOrganizerRouteAliases` fails because `/api/agenda/v1/auth/login` is still 404 and is the intentionally red Task 1 test reserved for the route-alias task. `TestItemHistoryProjectAndRestore` also failed with an unexpected history ordering/content assertion; this is unrelated to the configuration changes and was not modified. No config tests failed.

## Concerns

- The full package suite remains red until the later route-alias task is complete; one unrelated store-history test was also non-deterministically red in this run.
- Existing default data paths remain `/var/lib/faber-organizer/...` in code to preserve runtime defaults as required. New deployments are documented with `/var/lib/agenda/...` examples.

## Commit

- `f0cc1e6c17c8d8d9529da6364753ef82de851d6d` — `feat: add Agenda configuration compatibility`

## Full-suite exact output

Command:

```powershell
go test ./ -count=1
```

Output from the verification run:

```text
2026/09/22 21:26:34 AI parse fallback capture_id= attachment=false status=503 error_class=upstream_unavailable
--- FAIL: TestAgendaAndOrganizerRouteAliases (0.11s)
    migration_test.go:13: login through /api/agenda/v1 failed: 404 404 page not found
--- FAIL: TestItemHistoryProjectAndRestore (0.02s)
    store_test.go:84: unexpected history: []main.ItemEvent{...}
FAIL
FAIL	faber-organizer	1.287s
FAIL
```

The omitted middle of the history value is generated test data; the command's
deterministic failure lines and package result are preserved above. The route
failure is the intentional red test reserved for Task 4, and the history
failure is the pre-existing nondeterministic baseline noted earlier.
