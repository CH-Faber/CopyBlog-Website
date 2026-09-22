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

Commit recorded after verification; the final commit hash is provided in the task handoff.
