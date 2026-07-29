# my-assistant

A Go REST service that tells an ESP32 with an e-ink display what to show. The ESP32 polls the endpoint every hour; content will change with the time of day (and, in future iterations, will come from Google Calendar and Google Sheets). Meant to run unattended on a small always-on machine on your own LAN (an old PC, a Raspberry Pi) rather than a public VPS — see [Deployment recommendations](#deployment-recommendations) below.

**Current status**: `/api/v1/display` renders today's agenda from a single reference Google Calendar — one row per event/reminder, dropped an hour after it ends — next to the current shopping list, both read from a Google Sheet, with the next few days of the weekly menu (lunch/dinner) below, read from that same sheet's second tab.

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

- Go 1.25+

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

The service reads the current shopping list from a single reference spreadsheet — its first tab, one product per row starting at row 2 (row 1 is a header for your own use, e.g. "Producto"; it's never read). Reuses the same OAuth credentials file as Calendar, scoped to **only this one spreadsheet** (`drive.file`, not account-wide `spreadsheets.readonly`) via a one-time Google Picker selection:

1. In the same [Google Cloud Console](https://console.cloud.google.com/) project, enable the **Google Picker API** (`APIs & Services → Library`).
2. **APIs & Services → Credentials → Create credentials → API key.** Restrict it to the Picker API (no HTTP referrer restriction needed — it's only ever used from a page served on `localhost` during setup, never a public site). Set `GOOGLE_PICKER_API_KEY` to it (see `.env.example`) — this is only ever read by `cmd/oauthsetup`, never by the server at runtime.
3. On the **OAuth consent screen**, add the `.../auth/drive.file` scope alongside the existing calendar one.
4. Create the spreadsheet (or reuse one you already have), with a header in row 1 and products starting at row 2 of the first tab.
5. Run (or **re-run**, if `secrets/credentials.json` already exists from a Calendar-only setup — Google requires re-consent whenever the requested scope set changes) `go run ./cmd/oauthsetup`. After signing in, a browser tab opens the Google Picker widget — pick this spreadsheet there; that selection *is* what grants the app access to it. The tool then prints the picked file's ID and name.
6. Set `GOOGLE_SHEET_ID` (see `.env.example`) to that printed ID — it should match the ID in the sheet's URL (`https://docs.google.com/spreadsheets/d/<GOOGLE_SHEET_ID>/edit`) since it's the same file; the print is just a confirmation.

The **second tab** of that same spreadsheet holds the weekly menu, with a fixed layout the service assumes (rather than asking you to name the tab — it's found by position, the second one, whatever it's called):

- Row 1: one column per day of the week, Monday-first (A=Monday, B=Tuesday, ... G=Sunday). The text you put in each header cell is used verbatim as that day's display label — write it in whatever language/format you like.
- Rows 2–6: up to 5 lunch ("comida") entries for that day, one per row.
- Row 7: left blank (a spacer, never read).
- Rows 8–12: up to 5 dinner ("cena") entries for that day, one per row.

Blank cells and rows are skipped silently, so a day doesn't need all 5 lunch/dinner rows filled in.

See [`examples/example-spreadsheet.xlsx`](examples/example-spreadsheet.xlsx) for a filled-in example of both tabs.

## Running the server

```bash
go run ./cmd/server
```

Listens on `:8080` by default (configurable via `PORT`). Fails immediately at startup (before serving anything) if `AUTH_TOKEN`, `GOOGLE_CREDENTIALS_FILE`, `CALENDAR_ID`, `GOOGLE_SHEET_ID`, `TZ`, `WEATHER_LATITUDE`, or `WEATHER_LONGITUDE` is missing or invalid — see [Configuration](#configuration), [Google Calendar setup](#google-calendar-setup), and [Google Sheets setup](#google-sheets-setup) above to get them all in place first. `WEATHER_LATITUDE`/`WEATHER_LONGITUDE` need no setup step of their own — just the coordinates of whatever fixed location you want the weather panel's forecast for (see `.env.example`).

By default the server listens over HTTPS, using a self-signed certificate. Pass `--insecure` to serve plain HTTP instead — see [HTTPS (default) and --insecure mode](#https-default-and---insecure-mode) below.

**First-time setup, end to end:**

This assumes you've already completed the Google Cloud Console side of [Google Calendar setup](#google-calendar-setup) (steps 1-4: enable the API, configure the OAuth consent screen, create the OAuth client, save it as `secrets/oauth-client.json`) and [Google Sheets setup](#google-sheets-setup) (steps 1-3: enable the Picker API, create the Picker API key, add the `drive.file` scope) — the commands below are everything that's left once those one-time Cloud Console steps are done:

```bash
cp .env.example .env               # then edit it: AUTH_TOKEN, TZ, GOOGLE_PICKER_API_KEY (see "Google Sheets setup")
go run ./cmd/oauthsetup             # one-time OAuth login + spreadsheet picker; writes secrets/credentials.json; prints calendar IDs + picked spreadsheet ID
# edit .env: set GOOGLE_CREDENTIALS_FILE=secrets/credentials.json
# edit .env: set CALENDAR_ID to one of the printed IDs
# edit .env: set GOOGLE_SHEET_ID to the printed spreadsheet ID
go run ./cmd/server
```

## HTTPS (default) and --insecure mode

By default the server listens with TLS using a self-signed certificate. Pass `--insecure` to serve plain HTTP instead, on the same port either way (`PORT`/`cfg.Port` is unchanged — this is a mode switch, not a second listener):

```bash
go run ./cmd/server            # HTTPS (default)
go run ./cmd/server --insecure # plain HTTP
```

- **Certificate persistence**: generated once at `secrets/tls-cert.pem` / `secrets/tls-key.pem` (the same `secrets/` directory the Google OAuth credentials live in, already gitignored) and reused as-is on every later restart — regenerating would change the certificate's fingerprint and break any ESP32 client that has already pinned the old one.
- **Certificate shape**: self-signed, ECDSA P-256, 10-year validity, SANs covering loopback (`127.0.0.1`, `::1`, `localhost`) plus any other local IPs detected via the network interfaces present at generation time.
- **Using your own certificate instead**: the reuse logic doesn't care whether the cert/key pair at those two paths was generated by this program or came from a real CA — stop the server, replace `secrets/tls-cert.pem` and `secrets/tls-key.pem` with your own PEM files (same filenames), and restart. No flag or code change needed.
- **Checking the certificate**: whenever the server is running over HTTPS (the default), `GET /api/v1/tls-cert` (see [Endpoint](#endpoint) below) serves a small page with the certificate's SHA-256 fingerprint and full PEM text, each with a "copy" button — deliberately unauthenticated (no `AUTH_TOKEN` needed) so it's easy to open from an ordinary phone browser on the same network and copy from there. This endpoint doesn't exist at all when the server is running with `--insecure`.
- **Trusting it from an ESP32**: since it's self-signed, any browser visiting an HTTPS URL on this server will show a security warning — expected, since the intended client is an ESP32, not a browser. The simplest and most standard way for an ESP32 to trust this one specific certificate is to embed the PEM text from `/api/v1/tls-cert` as its own trusted CA: `WiFiClientSecure::setCACert(...)` on Arduino-ESP32, or `esp_tls_cfg_t.cacert_buf`/`cacert_pem` on ESP-IDF — mbedTLS then validates the server's certificate normally against exactly this one, no manual hash comparison needed in firmware. (Arduino-ESP32 also has a `setFingerprint()` method, but it only supports SHA-1 and is more fragile if the certificate ever changes.)

## Endpoint

`GET /api/v1/display`

Requires an `Authorization: Bearer <AUTH_TOKEN>` header. The same token must be set in the ESP32 firmware. Also requires a `battery` query parameter — the device's battery percentage, 1-100 — since the ESP32 reports its own battery level on every poll.

```bash
curl -k -H "Authorization: Bearer $AUTH_TOKEN" "https://localhost:8080/api/v1/display?battery=87" -o buffer.bin
```

(`-k` skips certificate verification, since the server's certificate is self-signed — see [HTTPS (default) and --insecure mode](#https-default-and---insecure-mode) above. If the server was started with `--insecure`, drop `-k` and use `http://` instead.)

- No token or wrong token → `401 Unauthorized`.
- Missing, non-numeric, or out-of-range (must be 1-100) `battery` query parameter → `400 Bad Request`.
- Correct token and valid battery → `200 OK`, `Content-Type: application/octet-stream`, body = image in the binary format described above.

The image shows a header with the date, then today's agenda ("Eventos") in a left column (one row per event/reminder, each dropped from the list an hour after it ends, so only what's upcoming, ongoing, or just finished stays visible — a reminder shows just its start time, a regular event shows start-end, an all-day item is marked "All day") next to the current shopping list ("Lista de la compra") in a right column. Below both, a "Menú semanal" section shows the next few days' planned lunch/dinner, starting from today. A small footer in the bottom-right corner shows the image's generation time and the reported battery percentage (e.g. `15:04:05 - 87%`). If the calendar can't be fetched, the endpoint still returns `200` with a rendered error message instead of the whole layout, so a broken integration is visible on the panel itself; if only the shopping list or the weekly menu can't be fetched, the rest of the display stays visible and just that section shows an error line instead.

`GET /api/v1/tls-cert`

Registered whenever the server is running over HTTPS (the default — see [HTTPS (default) and --insecure mode](#https-default-and---insecure-mode) above) — a `404` on this path when started with `--insecure` is expected, not a bug. No `Authorization` header required. Returns `200`, `Content-Type: text/html`, a page showing the server's TLS certificate's SHA-256 fingerprint and full PEM text, each with a "copy to clipboard" button.

## Visualization tool (`cmd/preview`)

Since no standard image format is used, `cmd/preview` lets you inspect what's being sent to the ESP32 without owning the physical panel, either in the terminal or as a native-resolution (800×480) PNG image.

**Image mode (recommended for a sharp view of the content):**

```bash
# generate a PNG and open it with the system's default viewer/browser (add --insecure if the server is running its default HTTPS mode, see below)
go run ./cmd/preview --url http://localhost:8080/api/v1/display --token "$AUTH_TOKEN" --battery 87 --open

# or against an already downloaded buffer
go run ./cmd/preview --file buffer.bin --open

# --png saves to a specific path instead of a temp file
go run ./cmd/preview --file buffer.bin --png output.png
```

`--battery` (default `100`) is only meaningful in `--url` mode — it's sent as the endpoint's required `battery` query parameter, so you can e.g. pass `--battery 5` to see how a low-battery footer reads.

Against the server's default HTTPS mode (see [HTTPS (default) and --insecure mode](#https-default-and---insecure-mode)), add `--insecure` to skip TLS certificate verification — like `curl -k` — since the server's self-signed certificate isn't trusted by default:

```bash
go run ./cmd/preview --url https://localhost:8080/api/v1/display --token "$AUTH_TOKEN" --battery 87 --insecure --open
```

Note this is the same flag name as `cmd/server --insecure`, but with a different meaning in each binary: here it means "skip TLS certificate verification when connecting" (like `curl -k`); on the server it means "serve plain HTTP instead of HTTPS." If the server itself was started with `--insecure` (plain HTTP), connect `cmd/preview` with a plain `http://` URL and omit `cmd/preview`'s own `--insecure`.

`--open` uses `xdg-open` (Linux), `open` (macOS), or `start` (Windows) to open the PNG with the default application.

**Terminal mode:**

```bash
go run ./cmd/preview --file buffer.bin

# --cols controls the output width in terminal columns (default 120)
go run ./cmd/preview --file buffer.bin --cols 160
```

Renders the image using Unicode block characters and ANSI grayscale colors (232-255), using half-blocks (`▀`) to double the apparent vertical resolution. Both the terminal mode and the PNG generation start from the same fully decoded buffer, so the PNG always shows the real detail without downsampling.

## OAuth setup tool (`cmd/oauthsetup`)

Turns the OAuth desktop client downloaded from Google Cloud Console into the long-lived `authorized_user` credentials file described in [Google Calendar setup](#google-calendar-setup) above. After signing in, it opens a Google Picker widget (served locally, restricted to spreadsheets, authorized with `GOOGLE_PICKER_API_KEY` from `.env`) so you can authorize exactly one reference spreadsheet — `drive.file` scope grants access only to whatever gets picked there, not every spreadsheet in the account. Then it prints the calendars the authorized account can see (name + ID, for `CALENDAR_ID`) and the picked spreadsheet's ID + name (for `GOOGLE_SHEET_ID`). Run once per Google account:

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

## Diagnostic tool (`cmd/menucheck`)

Dumps the raw values read from the weekly-menu spreadsheet's second tab as JSON (plus which tab title was picked, since the tab is found by position rather than by name), straight from the Google Sheets API, bypassing the app's own column-extraction/rotation parsing.

```bash
go run ./cmd/menucheck
```

Requires the same env vars as the server (`GOOGLE_CREDENTIALS_FILE`, `GOOGLE_SHEET_ID`).

## Diagnostic tool (`cmd/weathercheck`)

Dumps the raw Open-Meteo hourly forecast response for the configured location as JSON, straight from the API (bypassing `internal/weather`'s parsing). Useful to check fields `internal/weather` doesn't read, or to debug the data feeding the weather panel.

```bash
go run ./cmd/weathercheck
```

Requires the same env vars as the server (`WEATHER_LATITUDE`, `WEATHER_LONGITUDE`, `TZ`).

## Deployment recommendations

This service is designed for a small always-on machine on your own LAN — an old PC, a Raspberry Pi — not a public VPS. It holds long-lived OAuth credentials with read (Calendar) and read/write (the picked spreadsheet) access to your Google account, and its self-signed HTTPS is meant for ESP32 certificate pinning, not for standing up to internet-facing traffic (no rate limiting, no intrusion protection, etc.).

If you deploy it on a VPS anyway:

- **Cap your Google Cloud billing.** Set a budget alert (Cloud Console → Billing → Budgets & alerts), and consider quotas on the Calendar/Sheets/Drive APIs, so a leaked `AUTH_TOKEN` or a bug can't run up unexpected API usage while facing the public internet.
- **Don't authorize a Google account whose calendar or picked spreadsheet contains anything confidential.** The OAuth credentials in `secrets/` are what stand between the internet and that data — treat the account you run `cmd/oauthsetup` with as dedicated to this app, not your main personal account.

**Cross-compiling for low-power hardware** (e.g. a Raspberry Pi Zero W, single-core ARMv6):

```bash
GOOS=linux GOARCH=arm GOARM=6 go build -o my-assistant ./cmd/server
```

Copy the resulting `my-assistant` binary and the `secrets/` directory to the device, then run it directly or under systemd (see below). For a Raspberry Pi 2/3/4 (ARMv7/ARM64), use `GOARCH=arm GOARM=7` or `GOARCH=arm64` instead.

## Running as a systemd service

A minimal unit for running the server unattended, e.g. at `/etc/systemd/system/my-assistant.service`:

```ini
[Unit]
Description=my-assistant
After=network.target

[Service]
WorkingDirectory=/opt/my-assistant
EnvironmentFile=/opt/my-assistant/my-assistant.env
ExecStart=/opt/my-assistant/my-assistant
Restart=on-failure
User=my-assistant

[Install]
WantedBy=multi-user.target
```

`WorkingDirectory` is what the server's relative `secrets/` path resolves against, so the `secrets/` directory produced by `cmd/oauthsetup` (and `secrets/tls-cert.pem`/`tls-key.pem`, generated on first run) must live there too. `EnvironmentFile` holds the same keys as `.env.example`, as real environment variables — not `.env` itself, which is a development-only convenience (see [Configuration](#configuration) above).

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now my-assistant
journalctl -u my-assistant -f   # follow logs
```

## Tests

```bash
go test ./...
```

Covers: token and battery-parameter validation, round-trip encoding/decoding of the custom binary format, the endpoint handler via `httptest` (including calendar, shopping list, and weekly menu fetch success/failure, using fake fetchers — no real network calls), config validation, calendar event classification (reminder vs. event vs. all-day, and the "hide an hour after it ends" rule), shopping-list row parsing (blank/whitespace/non-string rows dropped), weekly-menu column parsing/rotation (blank rows dropped, days rotated to start at a given weekday and wrap around the week), the bottom-right footer's right-alignment, and self-signed TLS certificate generation/reuse (idempotency, reusing an externally-provided cert/key pair unchanged, SAN filtering, fingerprint formatting) plus the `/api/v1/tls-cert` endpoint's behavior with and without `--insecure`.

## Project structure

```
examples/
  example-spreadsheet.xlsx # reference spreadsheet: shopping list (first tab) + weekly menu (second tab)
cmd/
  server/        # HTTP(S) server entrypoint; HTTPS by default (generates/reuses a self-signed cert, cmd/server/tls.go), --insecure for plain HTTP
  preview/       # terminal/PNG buffer visualization CLI
  oauthsetup/    # one-time tool: OAuth desktop client JSON -> long-lived credentials file
  calendarcheck/ # dumps today's raw Calendar API events as JSON, for debugging
  sheetscheck/   # dumps the raw shopping-list sheet values as JSON, for debugging
  menucheck/     # dumps the raw weekly-menu sheet values as JSON, for debugging
  weathercheck/  # dumps the raw Open-Meteo forecast response as JSON, for debugging
internal/
  config/       # configuration loading (token, port, Google credentials, calendar/sheet IDs, timezone) from environment/.env
  calendar/     # Google Calendar client + today's agenda as a list of Row
  shoppinglist/ # Google Sheets client + the current shopping list as a list of items
  weeklymenu/   # Google Sheets client + the week's planned menu as a rotated list of Day
  display/      # image generation + custom binary format codec
  server/       # router, auth middleware, and HTTP handlers
```

## Roadmap

- Time-of-day variation logic: what's shown and in what format depending on the time (including revisiting the weekly menu's 3-day cap and layout once other phases are in place).
- ESP32 firmware that polls this endpoint hourly and paints the received buffer on the e-ink panel.
