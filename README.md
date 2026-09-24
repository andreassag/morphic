# morphic

A self-hosted media toolkit — format conversion, duplicate detection, and file organisation in a single dark-themed web UI, built entirely in Go.

[![Build](https://github.com/andreassag/morphic/actions/workflows/documentation.yml/badge.svg)](https://github.com/andreassag/morphic/actions/workflows/documentation.yml)

---

## Features

### 🔄 Converter

Convert images and videos between popular formats directly in the browser.

- Scan any folder (with optional subfolder recursion) filtered by images, videos, or both
- Per-extension summary with badge counts and filter pills
- Select individual files or batch-select by extension
- Pick a target format and convert in one click — live progress bar with per-file feedback
- **Images** — jpg, png, tif, bmp, webp, gif, ico, avif (via the `imaging` library)
- **Videos** — mp4, mov, avi, mkv, webm, flv, wmv, m4v, mpeg, 3gp, ts (via ffmpeg)
- Optionally delete originals after a successful conversion
- Stop an in-flight conversion at any time

### 🔍 Dupfinder

Find visually similar media using perceptual hashing — catches re-encoded or resized duplicates that byte-comparison misses.

- Three hash types combined (pHash, aHash, dHash) for higher accuracy
- Configurable similarity threshold per type (images: 90 %, videos: 85 % default)
- Video analysis by comparing frame hashes across a clip
- Multi-worker concurrent hashing with a configurable worker count
- Grouped results with thumbnail previews, file sizes, and space-savings estimate
- One-click auto-select of duplicates (keeps the largest file in each group)
- Stop scan mid-flight and discard partial results

### 📂 Organizer

Restructure a media collection into clean date-based folders or rename files in bulk.

- **Date sort** — moves/copies files into `{year}/{month}/{day}` trees using EXIF date or mtime fallback
- **Rename** — template tokens: `{date}`, `{datetime}`, `{original}`, `{ext}`, `{seq}`, `{seq:N}` (zero-padded)
- Preview the full plan before executing — see every source → destination path
- Move or copy mode
- Conflict detection — skips renames that would overwrite another file

---

## Quick Start

```bash
git clone https://github.com/andreassag/morphic
cd morphic
make build          # compiles to ./bin/morphic
./bin/morphic       # opens http://127.0.0.1:8000 in the browser
```

```bash
# Custom options
./bin/morphic --port 9000 --folder ~/Pictures --no-browser
```

---

## Prerequisites

| Dependency | Required for | Install |
|---|---|---|
| Go 1.22+ | Building | https://go.dev/dl/ |
| ffmpeg | Video conversion & video duplicate detection | `sudo apt install ffmpeg` |

ffmpeg is optional — the app starts without it and greys out video features automatically.

---

## Development

```bash
make tidy    # go mod tidy
make build   # build binary → ./bin/morphic
make test    # go test ./...
make vet     # go vet ./...
make run     # build + start dev server
```

---

## Architecture

```
cmd/morphic/       CLI entry-point (flags, server startup)
internal/
  converter/       Folder scanner, image/video conversion logic
  dupfinder/       Perceptual hashing, duplicate grouping, job runner
  organizer/       Date sorter, batch renamer, plan executor
  shared/          Job store, file browser, thumbnail generator, constants
web/
  server.go        Gin router, embedded static assets
  routes_*.go      HTTP handlers per module
  templates/       index.html (single-page UI)
  static/          js/, css/
```

---

## API Overview

All endpoints are under `/api/`.

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/converter/scan` | Scan a folder for convertible media |
| `POST` | `/api/converter/convert` | Start a batch conversion job |
| `GET`  | `/api/converter/progress/:id/poll` | Poll conversion progress |
| `POST` | `/api/converter/progress/:id/cancel` | Cancel a running conversion |
| `POST` | `/api/dupfinder/scan` | Start a duplicate scan job |
| `GET`  | `/api/dupfinder/scan/:id/status` | Poll scan status |
| `GET`  | `/api/dupfinder/scan/:id/results` | Fetch scan results |
| `POST` | `/api/dupfinder/scan/:id/cancel` | Cancel a running scan |
| `POST` | `/api/organizer/plan` | Create an organisation plan |
| `POST` | `/api/organizer/execute/:id` | Execute a previewed plan |
| `POST` | `/api/organizer/cancel/:id` | Cancel a running plan job |
| `GET`  | `/api/browse` | List directories (in-page browser) |
| `GET`  | `/api/thumbnail` | Generate a JPEG thumbnail |
| `GET`  | `/api/system_info` | Report Go/ffmpeg version info |

---

## License

Released under the [MIT License](LICENSE).
