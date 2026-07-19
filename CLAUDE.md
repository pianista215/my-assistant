# CLAUDE.md

Guidance for Claude Code when working in this repository.

## What this is

A Go REST service that decides and serves what an e-ink screen connected to an ESP32 should display. The ESP32 will poll the endpoint hourly; content will change with the time of day and, in future iterations, will come from Google Calendar and Google Sheets. Runs autonomously on a VPS. This is the user's first Go project — prioritize idiomatic, explainable code over shortcuts.

See [README.md](README.md) for usage, endpoint, and binary format details.

## Target hardware

Seeed reTerminal E1001 (GDEY075T7 panel, UC8179 controller): **800×480 px, 4 grayscale levels (2 bits/pixel)**. There's no lightweight standard image format worth adopting for this, hence the custom binary format in `internal/display/codec.go` and the `cmd/preview` CLI to inspect it visually.

## Commands

```bash
go build ./...
go vet ./...
go test ./...
go run ./cmd/server
go run ./cmd/preview --file buffer.bin
```

Unlike other projects of this user, tests **can be run directly here** (`go test ./...`) during implementation — this is a new, small, isolated Go project, without the "don't run tests, give me the command" policy that applies to other repos (that policy belongs to a different, unrelated project).

## Conventions for this project

- **No single-use packages**: don't create an `internal/auth` package just for the token middleware — it lives in `internal/server/middleware.go`, alongside the rest of the HTTP server. If in the future several middlewares are shared across different servers, extracting `internal/middleware` (the `mid` pattern from Ardan Labs Service) would then be justified.
- **`internal/display`, not `internal/eink`**: the package represents *what will be displayed* (image + codec), not the panel driver/firmware. Consistent with the `/api/v1/display` endpoint.
- **Authentication token**: loaded from an environment variable (`AUTH_TOKEN`), with `.env` support in development via `github.com/joho/godotenv`. In production the VPS sets real environment variables (systemd `EnvironmentFile=`), there's no `.env` on the server. Token comparison uses `crypto/subtle.ConstantTimeCompare`.
- **Custom image format**: `internal/display/codec.go` packs 2 bits/pixel with no external standard, meant to minimize memory usage on the ESP32. Any format change must preserve the `Encode`/`Decode` roundtrip and its test.

## Iteration scope

- **Iteration 1 (current)**: REST structure, token auth middleware, `/api/v1/display` endpoint with a "Hello World" + current time placeholder, visualization CLI, tests. **No** Google integration yet — not even stub packages; those get added once that phase is designed.
- **Next iterations**: Google Calendar/Sheets integration as the real content source, time-of-day variation logic, ESP32 firmware, VPS deployment.
