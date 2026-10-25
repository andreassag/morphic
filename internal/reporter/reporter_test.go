package reporter_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/andreassag/morphic/internal/reporter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func assetsDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "assets", "test"))
	require.NoError(t, err)
	return dir
}

func TestGenerateReport(t *testing.T) {
	ctx := context.Background()
	assets := assetsDir(t)

	tests := []struct {
		name    string
		req     reporter.ReportRequest
		wantErr bool
	}{
		{
			name: "Generate report with storage and formats",
			req: reporter.ReportRequest{
				Folder:         assets,
				IncludeStorage: true,
				IncludeFormats: true,
			},
			wantErr: false,
		},
		{
			name: "Generate report for invalid folder",
			req: reporter.ReportRequest{
				Folder: "/nonexistent/invalid/path",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := reporter.GenerateReport(ctx, nil, tt.req)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, res)
				assert.NotEmpty(t, res.ID)
				assert.Contains(t, res.HTMLContent, "<!DOCTYPE html>")
				assert.Contains(t, res.HTMLContent, "Storage Overview")

				retrieved, ok := reporter.GetReport(res.ID)
				assert.True(t, ok)
				assert.Equal(t, res.ID, retrieved.ID)
			}
		})
	}
}
