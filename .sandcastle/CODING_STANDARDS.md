# Coding Standards

## Backend

- Keep the layered architecture clear: router -> controller -> service -> model.
- Prefer GORM abstractions over raw SQL. Any raw SQL must remain compatible with SQLite, MySQL >= 5.7.8, and PostgreSQL >= 9.6.
- Do not call `encoding/json` marshal or unmarshal functions directly in business code. Use the wrappers in `common/json.go`.
- Optional scalar fields in upstream relay request DTOs must use pointer types with `omitempty` so explicit zero and false values survive client JSON -> upstream JSON conversion.
- When adding a new relay channel, confirm whether the provider supports `StreamOptions`; if it does, add the channel to `streamSupportedChannels`.
- Before changing tiered or dynamic billing expression logic, read `pkg/billingexpr/expr.md` and follow its architecture.
- Do not remove, rename, or replace protected project or organization identifiers.

## Frontend

- Use Bun for frontend dependency and script commands.
- Frontend i18n lives in `web/default/src/i18n/locales/{lang}.json`; use `t('English key')` and keep supported locales in sync.

## Testing

- Run `gofmt -w` on changed Go files.
- Run targeted Go tests while iterating and `go test ./...` before committing backend changes.
- Run `go vet ./...` for wider backend changes when feasible.
- For default frontend changes, run `cd web/default && bun run typecheck && bun run build`.
- For classic frontend changes, run `cd web/classic && bun run build`.
