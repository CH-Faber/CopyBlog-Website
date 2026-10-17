# Agenda API

The current Go implementation lives in `scripts/organizer` during the incremental
migration. It is already independently runnable: start it with `go run .` from
that directory, or use the root package scripts. The service exposes `/healthz`,
`/readyz`, and `/version` under both `/api/agenda/v1` and the legacy
`/api/organizer/v1` namespace.

Configuration uses `AGENDA_*` keys with `ORGANIZER_*` compatibility fallbacks;
see the root `.env.example`. No deployment or server mutation is performed by
these development commands.
