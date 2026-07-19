# my-assistant

A Go REST service that tells an ESP32 with an e-ink display what to show. The ESP32 polls the endpoint every hour; content will change with the time of day (and, in future iterations, will come from Google Calendar and Google Sheets). Designed to run autonomously on a VPS.

**Current status (first iteration)**: just the base structure — a token-protected endpoint that returns a placeholder image ("Hello World" + current time). No Google integration yet.

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

## Running the server

```bash
go run ./cmd/server
```

Listens on `:8080` by default (configurable via `PORT`).

## Endpoint

`GET /api/v1/display`

Requires an `Authorization: Bearer <AUTH_TOKEN>` header. The same token must be set in the ESP32 firmware.

```bash
curl -H "Authorization: Bearer $AUTH_TOKEN" http://localhost:8080/api/v1/display -o buffer.bin
```

- No token or wrong token → `401 Unauthorized`.
- Correct token → `200 OK`, `Content-Type: application/octet-stream`, body = image in the binary format described above.

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

## Tests

```bash
go test ./...
```

Covers: token validation (auth middleware), round-trip encoding/decoding of the custom binary format, and the endpoint handler via `httptest`.

## Project structure

```
cmd/
  server/     # HTTP server entrypoint
  preview/    # terminal/PNG buffer visualization CLI
internal/
  config/     # configuration loading (token, port) from environment/.env
  display/    # image generation + custom binary format codec
  server/     # router, auth middleware, and HTTP handlers
```

## Roadmap

- Google Calendar and Google Sheets integration as the real content source (will replace the "Hello World" placeholder).
- Time-of-day variation logic: what's shown and in what format depending on the time.
- ESP32 firmware that polls this endpoint hourly and paints the received buffer on the e-ink panel.
- VPS deployment (systemd, real environment variables).
