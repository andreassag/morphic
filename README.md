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

### 🏷️ Tagger

Bulk metadata reader, editor, and one-click privacy cleaner.

- Read EXIF, IPTC, and XMP metadata across photos and videos
- Batch-edit title, description, GPS coordinates, copyright, and keywords
- One-click metadata stripping for privacy protection
- Native Go JPEG/PNG segment stripper with optional `exiftool` support

### 📊 Inspector

Storage analytics dashboard with interactive visualization.

- Recursive directory analyzer with squarified treemap and sunburst diagrams
- Color-code by media type, file size, or age
- Identify largest files and oldest untouched media
- Format distribution summary and KPI metrics

### 🖼️ Gallery

Fast media browser and full-screen lightbox.

- Responsive grid view with lazy-loaded thumbnails
- Sort and filter by date, size, type, or search term
- Full-screen lightbox preview with keyboard shortcuts and video playback

### ✂️ Trimmer

Lightweight non-destructive image and video editing.

- Crop, rotate, flip, and resize images with live before/after comparison
- Trim video segments via ffmpeg with stream-copy or transcode
- Non-destructive export to new files

### 🔐 Hasher

Cryptographic checksum generator and bit-rot detection.

- High-throughput MD5, SHA-256, and xxHash64 streaming hashing
- Export manifests (`.sha256`, `.md5`, `.json`)
- Bit-rot verification mode to detect corrupt, missing, or changed files

### 📦 Archiver

Batch compression and packaging utility.

- Bundle files and folders into `.zip`, `.tar.gz`, or `.7z` archives
- Configurable compression levels (store to best)
- Multi-part archive splitting for size-limited transfers

### 🔗 Importer

Ingest media from URLs, S3 cloud storage, and connected devices.

- Download from HTTP/HTTPS URLs with SSRF protection
- Pull from S3-compatible object storage
- Scan mounted camera, phone, or removable media paths

### 🏗️ Resizer

Batch image resizing and thumbnail preset generator.

- Generate multiple size variants using web, social, and thumbnail presets
- Replicate directory hierarchies in parallel destination folders

### 📋 Reporter

Shareable summary and audit report generator.

- Create standalone HTML reports summarizing storage, duplicates, and activity
- Print-friendly layout for PDF export

### 🔄 Syncer

Folder mirroring and synchronization tool.

- One-way mirror and two-way sync between local directories
- Previewable diff plan showing new, modified, deleted, and conflicting files

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
  archiver/        Batch archive compression (.zip, .tar.gz, .7z) & volume splitting
  converter/       Folder scanner, image/video conversion logic
  dupfinder/       Perceptual hashing, duplicate grouping, job runner
  gallery/         Media collection indexing, filtering, and pagination
  hasher/          Concurrent cryptographic/fast hashing and manifest verification
  importer/        External media ingestion (URL, S3, devices)
  inspector/       Hierarchical storage tree analysis and metrics
  organizer/       Date sorter, batch renamer, plan executor
  reporter/        HTML and PDF report synthesis
  resizer/         Batch preset image resizing into parallel trees
  shared/          Job store, file browser, thumbnail generator, constants
  syncer/          Directory mirroring, diff planning, and sync execution
  tagger/          EXIF/IPTC/XMP metadata editing and privacy stripping
  trimmer/         Image crop/rotate/resize and video trimming
web/
  server.go        Gin router, embedded static assets
  routes_*.go      HTTP handlers per module
  templates/       index.html (single-page UI with grouped navigation)
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
| `POST` | `/api/organizer/execute` | Execute a previewed plan |
| `POST` | `/api/organizer/cancel/:id` | Cancel a running plan job |
| `POST` | `/api/tagger/scan` | Scan a folder for image/video metadata |
| `POST` | `/api/tagger/update` | Batch-update metadata fields |
| `POST` | `/api/tagger/strip` | Strip metadata for privacy |
| `POST` | `/api/inspector/analyze` | Generate storage hierarchy and breakdown |
| `POST` | `/api/gallery/list` | Query paginated, filtered media items |
| `POST` | `/api/trimmer/preview` | Generate image transform preview |
| `POST` | `/api/trimmer/export` | Export cropped/resized image or trimmed video |
| `POST` | `/api/hasher/generate` | Start concurrent hashing job |
| `POST` | `/api/hasher/verify` | Verify directory files against manifest |
| `POST` | `/api/archiver/create` | Package files into archive |
| `POST` | `/api/importer/url` | Ingest media from HTTP/HTTPS URLs |
| `POST` | `/api/importer/s3` | Ingest media from S3 bucket |
| `GET`  | `/api/importer/devices` | List attached media devices/mounts |
| `POST` | `/api/resizer/batch` | Batch resize images using presets |
| `POST` | `/api/reporter/generate` | Generate standalone HTML audit report |
| `POST` | `/api/syncer/plan` | Preview directory sync diff |
| `POST` | `/api/syncer/execute` | Execute planned directory sync |
| `GET`  | `/api/browse` | List directories (in-page browser) |
| `GET`  | `/api/thumbnail` | Generate a JPEG thumbnail |
| `GET`  | `/api/system_info` | Report Go, ffmpeg, exiftool, 7z capability info |

---

## License

Released under the [MIT License](LICENSE).
