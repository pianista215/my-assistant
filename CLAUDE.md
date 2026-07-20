# CLAUDE.md

Guidance for Claude Code when working in this repository.

## What this is

A Go REST service that decides and serves what an e-ink screen connected to an ESP32 should display. The ESP32 will poll the endpoint hourly; the endpoint renders today's agenda from a single reference Google Calendar (via a one-time-authorized OAuth credential, refreshed unattended — see "Calendar credentials" below), and in future iterations will also draw from Google Sheets and vary by time of day. Runs autonomously on a VPS. This is the user's first Go project — prioritize idiomatic, explainable code over shortcuts.

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
- **`internal/calendar`, not `internal/googlecalendar`**: represents the domain concept (today's agenda as a list of `Row`), backed by the Google Calendar API client internally. `internal/display` stays agnostic to where its text rows came from (`NewTextRows(header string, rows []string)`), so it's reusable once Sheets is added — the calendar package owns `Row.String()`, the formatting logic for its own data.
- **Reminder vs. event**: the Calendar API has no `reminder` field. The classification rule is: `Start == End` (zero duration) → reminder (show only start time); otherwise → event (show start-end). All-day items (`Start.Date` set instead of `Start.DateTime`) are a third case, shown as "All day".
- **Past-event visibility**: a row stays on the display until an hour after it ends (`visibleAfterEnd` in `internal/calendar/events.go`), so only what's upcoming, ongoing, or just-finished shows up.
- **Calendar credentials**: originally planned as a service account JSON key, but personal (org-less) Google Cloud projects have their `iam.disableServiceAccountKeyCreation` org policy locked with no self-serve override — there's no Organization resource to attach an exception to, and `gcloud ... org-policies disable-enforce` fails with a permission error for the same reason. Pivoted to a **one-time OAuth authorization of the user's own Google account** instead: `cmd/oauthsetup` (see below) turns a Desktop-type OAuth client (from Google Cloud Console) into an `authorized_user`-format credentials file via a single interactive login. `GOOGLE_CREDENTIALS_FILE` (env var, a path not JSON content) points at that file either way — `option.WithCredentialsFile` auto-detects both service-account and `authorized_user` JSON shapes, so `internal/calendar/client.go` needed no code change for this pivot. No calendar-sharing step is needed since the authorized account is the calendar owner. Timezone is a required env var (`TZ`), used both for "today"'s bounds and for formatting event times — there's no other timezone handling in the app.
- **`cmd/oauthsetup`**: a one-time, interactive CLI (not used at server runtime) that exchanges a downloaded OAuth Desktop client JSON for a long-lived `authorized_user` credentials file, via a local loopback HTTP listener catching Google's redirect. Run once per Google account; never touched again afterward, including on the VPS.
- **`cmd/calendarcheck`**: a small permanent diagnostic CLI that dumps today's raw Calendar API events as JSON, bypassing `internal/calendar`'s `Row` abstraction on purpose (it exists to inspect the real shape Google sends, e.g. when adding Sheets next or debugging unexpected calendar data).

## Toolchain

The original system Go (apt `golang-1.18`, from 2022) was too old for the current `google.golang.org/api` client (needs Go 1.24+). The user has since replaced it with a system-wide Go 1.26.5 install (via sudo, at `/usr/local/go/bin/go`); `go.mod`'s `go` directive matches. Just use `go` normally.

## Iteration scope

- **Iteration 1**: REST structure, token auth middleware, `/api/v1/display` endpoint with a "Hello World" + current time placeholder, visualization CLI, tests.
- **Iteration 2 (current)**: Google Calendar integration — `/api/v1/display` renders today's agenda (events + reminders) from one fixed reference calendar, replacing the placeholder. `NewHelloWorld` is kept around (still used by `cmd/preview`/tests) but is no longer wired into the server.
- **Next iterations**: Google Sheets integration, time-of-day variation logic, ESP32 firmware, VPS deployment.
