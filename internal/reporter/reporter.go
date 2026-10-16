package reporter

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"html/template"
	"sync"
	"time"

	"github.com/andreassag/morphic/internal/database"
	"github.com/andreassag/morphic/internal/inspector"
	"github.com/andreassag/morphic/internal/shared"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ReportRequest defines sections to include in the generated report.
type ReportRequest struct {
	Folder            string `json:"folder"`
	IncludeStorage    bool   `json:"include_storage"`
	IncludeFormats    bool   `json:"include_formats"`
	IncludeDuplicates bool   `json:"include_duplicates"`
	IncludeAudit      bool   `json:"include_audit"`
}

// ReportResult holds the generated report metadata and rendered HTML.
type ReportResult struct {
	ID          string                    `json:"id"`
	Title       string                    `json:"title"`
	GeneratedAt time.Time                 `json:"generated_at"`
	Folder      string                    `json:"folder"`
	Stats       *inspector.InspectResult  `json:"stats,omitempty"`
	AuditOps    []database.AuditOperation `json:"audit_ops,omitempty"`
	HTMLContent string                    `json:"html_content"`
}

var (
	reportsMu sync.RWMutex
	reports   = make(map[string]*ReportResult)
)

// GetReport retrieves a stored report by ID.
func GetReport(id string) (*ReportResult, bool) {
	reportsMu.RLock()
	defer reportsMu.RUnlock()
	r, ok := reports[id]
	return r, ok
}

// StoreReport saves a report in memory.
func StoreReport(r *ReportResult) {
	reportsMu.Lock()
	defer reportsMu.Unlock()
	reports[r.ID] = r
}

// GenerateReport creates storage analytics and renders a self-contained HTML document.
func GenerateReport(ctx context.Context, pool *pgxpool.Pool, req ReportRequest) (*ReportResult, error) {
	safeDir, err := shared.ValidateSafeDirPath(req.Folder)
	if err != nil {
		return nil, fmt.Errorf("validate directory: %w", err)
	}

	result := &ReportResult{
		ID:          uuid.New().String(),
		Title:       fmt.Sprintf("Morphic Storage & Media Report: %s", safeDir),
		GeneratedAt: time.Now(),
		Folder:      safeDir,
	}

	if req.IncludeStorage || req.IncludeFormats {
		stats, err := inspector.Analyze(ctx, safeDir, 15)
		if err == nil {
			result.Stats = stats
		}
	}

	if req.IncludeAudit && pool != nil {
		ops, _, err := database.ListAuditOperations(ctx, pool, "", 25, 0)
		if err == nil {
			result.AuditOps = ops
		}
	}

	htmlDoc, err := renderHTMLReport(result, req)
	if err != nil {
		return nil, fmt.Errorf("rendering HTML report: %w", err)
	}

	result.HTMLContent = htmlDoc
	StoreReport(result)

	return result, nil
}

func renderHTMLReport(r *ReportResult, req ReportRequest) (string, error) {
	tmplStr := `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>{{.Title}}</title>
<style>
  :root {
    --bg-base: #0f1117;
    --bg-card: #1a1d27;
    --border: #2a2f42;
    --text-main: #e2e8f0;
    --text-muted: #94a3b8;
    --accent: #6c7bf7;
    --success: #4ade80;
  }
  @media print {
    body { background: #fff !important; color: #000 !important; }
    .card { border: 1px solid #ccc !important; background: #fff !important; color: #000 !important; }
    .no-print { display: none !important; }
  }
  body {
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
    background: var(--bg-base);
    color: var(--text-main);
    margin: 0;
    padding: 2rem;
  }
  .container { max-width: 1000px; margin: 0 auto; }
  header { margin-bottom: 2rem; border-bottom: 1px solid var(--border); padding-bottom: 1rem; }
  h1 { font-size: 1.75rem; margin: 0 0 0.5rem 0; color: var(--accent); }
  .meta { color: var(--text-muted); font-size: 0.9rem; }
  .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 1rem; margin-bottom: 2rem; }
  .card { background: var(--bg-card); border: 1px solid var(--border); border-radius: 8px; padding: 1.25rem; }
  .card-title { color: var(--text-muted); font-size: 0.85rem; text-transform: uppercase; letter-spacing: 0.05em; margin-bottom: 0.5rem; }
  .card-value { font-size: 1.5rem; font-weight: 600; color: var(--text-main); }
  table { width: 100%; border-collapse: collapse; margin-top: 1rem; }
  th, td { text-align: left; padding: 0.75rem; border-bottom: 1px solid var(--border); font-size: 0.9rem; }
  th { color: var(--text-muted); font-weight: 500; }
  .btn-print { background: var(--accent); color: #fff; border: none; padding: 0.5rem 1rem; border-radius: 4px; cursor: pointer; float: right; }
</style>
</head>
<body>
<div class="container">
  <header>
    <button class="btn-print no-print" onclick="window.print()">Print / Export PDF</button>
    <h1>{{.Title}}</h1>
    <div class="meta">Folder: <code>{{.Folder}}</code> | Generated: {{.GeneratedAt.Format "2006-01-02 15:04:05 UTC"}}</div>
  </header>

  {{if .Stats}}
  <section>
    <h2>Storage Overview</h2>
    <div class="grid">
      <div class="card">
        <div class="card-title">Total Size</div>
        <div class="card-value">{{formatSize .Stats.TotalSize}}</div>
      </div>
      <div class="card">
        <div class="card-title">Total Files</div>
        <div class="card-value">{{.Stats.FileCount}}</div>
      </div>
      <div class="card">
        <div class="card-title">Media Files</div>
        <div class="card-value">{{.Stats.MediaCount}}</div>
      </div>
    </div>
  </section>

  <section class="card" style="margin-bottom: 2rem;">
    <h2>Format & Extension Breakdown</h2>
    <table>
      <thead><tr><th>Extension</th><th>File Count</th><th>Total Size</th></tr></thead>
      <tbody>
        {{range $ext, $cnt := .Stats.ExtensionCounts}}
        <tr>
          <td><code>{{$ext}}</code></td>
          <td>{{$cnt}}</td>
          <td>{{formatSize (index $.Stats.ExtensionSizes $ext)}}</td>
        </tr>
        {{end}}
      </tbody>
    </table>
  </section>

  <section class="card" style="margin-bottom: 2rem;">
    <h2>Largest Files</h2>
    <table>
      <thead><tr><th>Filename</th><th>Extension</th><th>Size</th><th>Modified</th></tr></thead>
      <tbody>
        {{range .Stats.LargestFiles}}
        <tr>
          <td>{{.Name}}</td>
          <td><code>{{.Ext}}</code></td>
          <td>{{formatSize .Size}}</td>
          <td>{{.ModTime.Format "2006-01-02 15:04"}}</td>
        </tr>
        {{end}}
      </tbody>
    </table>
  </section>
  {{end}}

  {{if .AuditOps}}
  <section class="card" style="margin-bottom: 2rem;">
    <h2>Recent Audit History</h2>
    <table>
      <thead><tr><th>Operation</th><th>Source</th><th>Summary</th><th>Items</th><th>Date</th></tr></thead>
      <tbody>
        {{range .AuditOps}}
        <tr>
          <td><code>{{.Operation}}</code></td>
          <td>{{.Source}}</td>
          <td>{{.Summary}}</td>
          <td>{{.ItemCount}}</td>
          <td>{{.CreatedAt.Format "2006-01-02 15:04"}}</td>
        </tr>
        {{end}}
      </tbody>
    </table>
  </section>
  {{end}}
</div>
</body>
</html>`

	funcMap := template.FuncMap{
		"formatSize": shared.FormatFileSize,
		"html":       html.EscapeString,
	}

	t, err := template.New("report").Funcs(funcMap).Parse(tmplStr)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, r); err != nil {
		return "", err
	}
	return buf.String(), nil
}
