---
layout: default
title: morphic — Self-hosted Media Toolkit
---

# morphic

A self-hosted media toolkit written in Go. Convert formats, find duplicates, and organise your media library — all from a single dark-themed web UI with no cloud dependencies.

---

## Getting Started

```bash
git clone https://github.com/exterex/morphic
cd morphic
make build
./bin/morphic          # opens http://127.0.0.1:8000
```

**Prerequisites**

| Dependency | Required for |
|---|---|
| Go 1.22+ | Building |
| ffmpeg *(optional)* | Video conversion, video duplicate detection |

Install ffmpeg on Ubuntu/Debian:

```bash
sudo apt install ffmpeg
```

---

## Modules

### 🔄 Converter

Scan a folder and convert images or videos to any supported format.

- **Images** — jpg, png, tif, bmp, webp, gif, ico, avif
- **Videos** — mp4, mov, avi, mkv, webm, flv, wmv, m4v, mpeg, 3gp, ts
- Filter by type, click a format pill, batch-select files, pick a target
- Live progress bar, per-file success/error feedback
- Stop an in-flight conversion at any time

### 🔍 Dupfinder

Find visually similar media using perceptual hashing — catches re-encoded and resized duplicates that byte-comparison misses.

- Combines pHash, aHash, and dHash
- Configurable similarity thresholds
- Video support: hashes sampled frames across each clip
- Grouped results with thumbnail preview and space-savings estimate
- Auto-select duplicates (keeps the largest file per group)
- Cancel scan mid-flight

### 📂 Organizer

Restructure and rename a media collection.

**Date sort** — moves or copies files into `{year}/{month}/{day}` trees:
- Reads EXIF `DateTimeOriginal` first; falls back to file modification time
- Preview the full plan before executing

**Rename** — template tokens available:

| Token | Output |
|---|---|
| `{date}` | `YYYYMMDD` |
| `{datetime}` | `YYYYMMDD_HHMMSS` |
| `{original}` | Original filename without extension |
| `{ext}` | File extension without dot |
| `{seq}` | Sequential integer |
| `{seq:N}` | Sequential integer zero-padded to N digits |

---

## API Reference

All endpoints are under `/api/`.

### Converter

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/converter/scan` | Scan folder for media |
| `GET`  | `/api/converter/formats` | List supported conversion formats |
| `POST` | `/api/converter/convert` | Start batch conversion job |
| `GET`  | `/api/converter/progress/:id` | Fetch job progress page |
| `GET`  | `/api/converter/progress/:id/poll` | Poll job progress (JSON) |
| `POST` | `/api/converter/progress/:id/cancel` | Cancel running job |
| `POST` | `/api/converter/delete` | Delete listed files |

### Dupfinder

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/dupfinder/scan` | Start duplicate scan |
| `GET`  | `/api/dupfinder/scan/:id/status` | Poll scan status |
| `GET`  | `/api/dupfinder/scan/:id/results` | Fetch grouped results |
| `POST` | `/api/dupfinder/scan/:id/cancel` | Cancel running scan |
| `POST` | `/api/dupfinder/delete` | Delete selected files |

### Organizer

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/organizer/plan` | Create organisation plan |
| `GET`  | `/api/organizer/plan/:id` | Get plan status & preview |
| `POST` | `/api/organizer/execute/:id` | Execute a previewed plan |
| `GET`  | `/api/organizer/execute/:id/status` | Poll execution status |
| `POST` | `/api/organizer/cancel/:id` | Cancel plan/execute job |

### Shared

| Method | Path | Description |
|--------|------|-------------|
| `GET`  | `/api/browse` | List directories |
| `POST` | `/api/browse/native` | Open OS native folder picker |
| `GET`  | `/api/thumbnail` | Generate JPEG thumbnail by path |
| `GET`  | `/api/system_info` | Go/ffmpeg build info |
| `GET`  | `/api/media` | Serve a media file for preview |

---

## Architecture

```
cmd/morphic/       CLI entry-point and server startup
internal/
  converter/       Folder scanner, image/video conversion
  dupfinder/       Perceptual hashing, grouping, async job runner
  organizer/       Date sorter, batch renamer, plan executor
  shared/          Generic job store, file utilities, thumbnails
web/
  server.go        Gin router with embedded static assets
  routes_*.go      HTTP handlers per module
  templates/       index.html — single-page UI
  static/          js/, css/
```

---

## Development

```bash
make tidy    # tidy go.mod / go.sum
make build   # compile → ./bin/morphic
make test    # run unit tests
make vet     # go vet
make run     # build + start server
```

---

## License

[MIT License](https://github.com/exterex/morphic/blob/main/LICENSE)
