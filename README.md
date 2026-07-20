# my-assistant

A Go REST service that tells an ESP32 with an e-ink display what to show. The ESP32 polls the endpoint every hour; content will change with the time of day (and, in future iterations, will come from Google Calendar and Google Sheets). Designed to run autonomously on a VPS.

**Current status**: `/api/v1/display` renders today's agenda from a single reference Google Calendar — one row per event/reminder, dropped an hour after it ends — followed by the current shopping list read from a Google Sheet.

## Target hardware

Seeed Studio reTerminal E1001 e-ink display (the "E10xx" family), GDEY075T7 panel, UC8179 controller:

- Resolution: **800 × 480 px**
- **4 grayscale levels** (black, dark gray, light gray, white) = 2 bits per pixel

There's no lightweight standard format worth adopting for 4 grayscale levels, so the server uses a **custom binary format** designed to minimize memory usage on the ESP32 (see [`internal/display/codec.go`](internal/display/codec.go)):

```
offset  size  field
0       4     magic "EINK"
4       1     format version
5       2     width  (big-endian uint16)
7       2     height (big-endian uint16)
9       1     bits per pixel
10      ...   pixel data packed at 2 bits/pixel (4 pixels per byte)
```

## Requirements

- Go 1.18+

## Configuration

```bash
cp .env.example .env
# edit .env and set a random AUTH_TOKEN, e.g.: openssl rand -hex 32
```

In production (VPS) `.env` isn't used: real environment variables are set on the service itself (e.g. `EnvironmentFile=` in the systemd unit).

### Google Calendar setup

The service reads one fixed reference calendar. The original plan was a service account key, but personal (org-less) Google Cloud projects have "Secure by Default" policies that block service account key creation with no self-serve override — see [`iam.disableServiceAccountKeyCreation`](https://cloud.google.com/resource-manager/docs/organization-policy/org-policy-constraints). Instead, this uses a **one-time OAuth authorization of your own Google account**, whose resulting credentials file is then used unattended (no repeated login, no browser needed at runtime):

1. In [Google Cloud Console](https://console.cloud.google.com/), create (or reuse) a project and enable the **Google Calendar API**.
2. **APIs & Services → OAuth consent screen**: user type "External" (the only option without a Workspace organization), add the `.../auth/calendar.readonly` scope, and set publishing status to **"In production"** (not "Testing" — Testing-mode refresh tokens expire after 7 days; Production ones don't, even for an unverified app with a single user). You'll see an "unverified app" warning when authorizing below — that's expected for a personal, single-user app; click through "Advanced → Go to (app name)".
3. **APIs & Services → Credentials → Create credentials → OAuth client ID**, application type **"Desktop app"**. Download the resulting JSON.
4. Save that file as `secrets/oauth-client.json` in the repo root (the `secrets/` directory is gitignored — never commit it).
5. Run the one-time setup tool: `go run ./cmd/oauthsetup`. It opens a browser to Google's consent screen, catches the redirect on a local loopback listener, writes `secrets/credentials.json` (an `authorized_user`-format credentials file), and then prints every calendar the authorized account can see, with its ID — pick the one you want as your reference calendar from that list. Run this once per Google account; the server itself never needs a browser or interactive login, at any point, including on the VPS.
6. Set the three env vars below (see `.env.example`):
   - `GOOGLE_CREDENTIALS_FILE`: path to `secrets/credentials.json` from step 5.
   - `CALENDAR_ID`: one of the IDs printed in step 5 (`primary` for your main calendar, or a `...@group.calendar.google.com` id for a secondary one).
   - `TZ`: the timezone used to compute "today" and format event times (e.g. `Europe/Madrid`).

No calendar sharing step is needed: since this authorizes your own Google account, the service sees whatever calendars that account already has access to.

### Google Sheets setup

The service reads the current shopping list from a single reference spreadsheet — its first tab, one product per row starting at row 2 (row 1 is a header for your own use, e.g. "Producto"; it's never read). Reuses the same OAuth credentials file as Calendar:

1. In the same [Google Cloud Console](https://console.cloud.google.com/) project, enable the **Google Sheets API**.
2. On the **OAuth consent screen**, add the `.../auth/spreadsheets.readonly` scope alongside the existing calendar one.
3. Create the spreadsheet (or reuse one you already have), with a header in row 1 and products starting at row 2 of the first tab.
4. Set `GOOGLE_SHEET_ID` (see `.env.example`) to the ID from the sheet's URL: `https://docs.google.com/spreadsheets/d/<GOOGLE_SHEET_ID>/edit`.
5. **If `secrets/credentials.json` already exists from a previous Calendar-only setup, re-run `go run ./cmd/oauthsetup`** — Google requires re-consent whenever the requested scope set changes, so the existing refresh token won't grant Sheets access on its own.

## Running the server

```bash
go run ./cmd/server
```

Listens on `:8080` by default (configurable via `PORT`). Fails immediately at startup (before serving anything) if `AUTH_TOKEN`, `GOOGLE_CREDENTIALS_FILE`, `CALENDAR_ID`, `GOOGLE_SHEET_ID`, or `TZ` is missing or invalid — see [Configuration](#configuration), [Google Calendar setup](#google-calendar-setup), and [Google Sheets setup](#google-sheets-setup) above to get all five in place first.

**First-time setup, end to end:**

```bash
cp .env.example .env               # then edit it: AUTH_TOKEN, TZ
go run ./cmd/oauthsetup             # one-time OAuth login; prints calendar IDs
# edit .env: set GOOGLE_CREDENTIALS_FILE=secrets/credentials.json
# edit .env: set CALENDAR_ID to one of the printed IDs
# edit .env: set GOOGLE_SHEET_ID to your shopping-list spreadsheet's ID
go run ./cmd/server
```

## Endpoint

`GET /api/v1/display`

Requires an `Authorization: Bearer <AUTH_TOKEN>` header. The same token must be set in the ESP32 firmware.

```bash
curl -H "Authorization: Bearer $AUTH_TOKEN" http://localhost:8080/api/v1/display -o buffer.bin
```

- No token or wrong token → `401 Unauthorized`.
- Correct token → `200 OK`, `Content-Type: application/octet-stream`, body = image in the binary format described above.

The image shows today's agenda: a header with the date, then one row per event/reminder, each dropped from the list an hour after it ends (so only what's upcoming, ongoing, or just finished stays visible). A reminder (a calendar item with no real duration) shows just its start time; a regular event shows start-end; an all-day item is marked "All day". Below the agenda, one row per item currently on the shopping list. If the calendar can't be fetched, the endpoint still returns `200` with a rendered error message instead of the agenda, so a broken integration is visible on the panel itself; if only the shopping list can't be fetched, the agenda stays visible and just that section shows an error line instead.

## Visualization tool (`cmd/preview`)

Since no standard image format is used, `cmd/preview` lets you inspect what's being sent to the ESP32 without owning the physical panel, either in the terminal or as a native-resolution (800×480) PNG image.

**Image mode (recommended for a sharp view of the content):**

```bash
# generate a PNG and open it with the system's default viewer/browser
go run ./cmd/preview --url http://localhost:8080/api/v1/display --token "$AUTH_TOKEN" --open

# or against an already downloaded buffer
go run ./cmd/preview --file buffer.bin --open

# --png saves to a specific path instead of a temp file
go run ./cmd/preview --file buffer.bin --png output.png
```

`--open` uses `xdg-open` (Linux), `open` (macOS), or `start` (Windows) to open the PNG with the default application.

**Terminal mode:**

```bash
go run ./cmd/preview --file buffer.bin

# --cols controls the output width in terminal columns (default 120)
go run ./cmd/preview --file buffer.bin --cols 160
```

Renders the image using Unicode block characters and ANSI grayscale colors (232-255), using half-blocks (`▀`) to double the apparent vertical resolution. Both the terminal mode and the PNG generation start from the same fully decoded buffer, so the PNG always shows the real detail without downsampling.

## OAuth setup tool (`cmd/oauthsetup`)

Turns the OAuth desktop client downloaded from Google Cloud Console into the long-lived `authorized_user` credentials file described in [Google Calendar setup](#google-calendar-setup) above, then prints the calendars the authorized account can see (name + ID) so you know what to set `CALENDAR_ID` to. Run once per Google account:

```bash
go run ./cmd/oauthsetup
# --client-json and --out override the default secrets/ paths
```

## Diagnostic tool (`cmd/calendarcheck`)

Dumps today's raw events from the configured reference calendar as JSON, straight from the Google Calendar API (bypassing the app's own event model). Useful to check how a given calendar item actually looks — e.g. whether a "reminder" really comes through with an identical start/end, or to debug why an event isn't showing up as expected.

```bash
go run ./cmd/calendarcheck
```

Requires the same env vars as the server (`GOOGLE_CREDENTIALS_FILE`, `CALENDAR_ID`, `TZ`).

## Diagnostic tool (`cmd/sheetscheck`)

Dumps the raw values read from the configured shopping-list spreadsheet as JSON, straight from the Google Sheets API (bypassing the app's own parsing/blank-row filtering). Useful to check exactly how a row comes through — e.g. to confirm what a genuinely blank row looks like.

```bash
go run ./cmd/sheetscheck
```

Requires the same env vars as the server (`GOOGLE_CREDENTIALS_FILE`, `GOOGLE_SHEET_ID`).

## Tests

```bash
go test ./...
```

Covers: token validation (auth middleware), round-trip encoding/decoding of the custom binary format, the endpoint handler via `httptest` (including calendar and shopping list fetch success/failure, using fake fetchers — no real network calls), config validation, calendar event classification (reminder vs. event vs. all-day, and the "hide an hour after it ends" rule), and shopping-list row parsing (blank/whitespace/non-string rows dropped).

## Project structure

```
cmd/
  server/        # HTTP server entrypoint
  preview/       # terminal/PNG buffer visualization CLI
  oauthsetup/    # one-time tool: OAuth desktop client JSON -> long-lived credentials file
  calendarcheck/ # dumps today's raw Calendar API events as JSON, for debugging
  sheetscheck/   # dumps the raw shopping-list sheet values as JSON, for debugging
internal/
  config/       # configuration loading (token, port, Google credentials, calendar/sheet IDs, timezone) from environment/.env
  calendar/     # Google Calendar client + today's agenda as a list of Row
  shoppinglist/ # Google Sheets client + the current shopping list as a list of items
  display/      # image generation + custom binary format codec
  server/       # router, auth middleware, and HTTP handlers
```

## Roadmap

- Time-of-day variation logic: what's shown and in what format depending on the time.
- ESP32 firmware that polls this endpoint hourly and paints the received buffer on the e-ink panel.
- VPS deployment (systemd, real environment variables).
