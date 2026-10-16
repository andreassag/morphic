// ── Reporter Module: Standalone HTML / PDF Report Generator ────────────

import * as UI from './ui.js';

let latestReportHTML = '';
let latestReportId = null;

export function initReporter() {
    // Optional setup
}

export function reporterInitTab() {
    const input = document.getElementById('reporterFolder');
    if (input && !input.value) {
        const convInput = document.getElementById('convFolder');
        if (convInput && convInput.value) {
            input.value = convInput.value;
        }
    }
}

export async function reporterGenerate() {
    const folder = document.getElementById('reporterFolder')?.value.trim() || '';
    if (!folder) {
        UI.showToast('Please specify a library folder to report on', 'warning');
        return;
    }

    const sections = {
        storage_stats: document.getElementById('repSecStats')?.checked ?? true,
        format_breakdown: document.getElementById('repSecFormats')?.checked ?? true,
        dup_summary: document.getElementById('repSecDups')?.checked ?? true,
        audit_history: document.getElementById('repSecAudit')?.checked ?? true,
        highlights: document.getElementById('repSecHighlights')?.checked ?? true
    };

    const generateBtn = document.getElementById('reporterGenerateBtn');
    if (generateBtn) generateBtn.disabled = true;

    try {
        const res = await fetch('/api/reporter/generate', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ folder, sections })
        });

        if (!res.ok) {
            const err = await res.json().catch(() => ({ error: 'Report generation failed' }));
            throw new Error(err.error || `HTTP ${res.status}`);
        }

        const data = await res.json();
        latestReportId = data.report_id || null;
        latestReportHTML = data.html || buildClientFallbackReport(folder, data);

        const outCard = document.getElementById('reporterOutputCard');
        const previewContainer = document.getElementById('reporterPreviewContainer');

        if (outCard) outCard.style.display = 'block';
        if (previewContainer) {
            previewContainer.innerHTML = latestReportHTML;
        }

        UI.showToast('Report generated successfully', 'success');
    } catch (err) {
        console.error('Reporter error:', err);
        UI.showToast(`Error generating report: ${err.message}`, 'error');
    } finally {
        if (generateBtn) generateBtn.disabled = false;
    }
}

function buildClientFallbackReport(folder, data) {
    const dateStr = new Date().toLocaleString();
    const stats = data.stats || {};
    return `
        <div class="report-doc">
            <h2>📊 Morphic Media Library Audit Report</h2>
            <div style="font-size:13px; color:var(--text-dim); margin-bottom:20px;">
                Generated for <strong>${folder}</strong> on ${dateStr}
            </div>

            <div class="kpi-grid">
                <div class="kpi-card accent">
                    <span class="kpi-label">Total Volume</span>
                    <span class="kpi-value">${UI.formatFileSize(stats.total_size || 0)}</span>
                </div>
                <div class="kpi-card">
                    <span class="kpi-label">Total Files</span>
                    <span class="kpi-value">${(stats.total_files || 0).toLocaleString()}</span>
                </div>
                <div class="kpi-card success">
                    <span class="kpi-label">Reclaimable Duplicates</span>
                    <span class="kpi-value">${UI.formatFileSize(stats.dup_bytes || 0)}</span>
                </div>
            </div>

            <h3>Library Overview</h3>
            <p style="font-size:13px; line-height:1.6; color:var(--text-dim);">
                This self-contained diagnostic report provides storage metrics, format distribution, duplicate file clusters, and conversion activity for the specified media library.
            </p>
        </div>
    `;
}

export function reporterDownloadHTML() {
    if (!latestReportHTML) {
        UI.showToast('No report generated yet', 'warning');
        return;
    }

    const fullDocument = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Morphic Media Library Report</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #0f1117; color: #e1e4ed; padding: 32px; }
        .kpi-grid { display: flex; gap: 16px; margin: 20px 0; }
        .kpi-card { background: #1a1d27; border: 1px solid #2e3345; border-radius: 8px; padding: 16px; flex: 1; }
        .kpi-label { font-size: 11px; color: #8b8fa3; text-transform: uppercase; font-weight: 700; }
        .kpi-value { font-size: 24px; font-weight: 800; color: #6c7bf7; margin-top: 4px; }
        table { width: 100%; border-collapse: collapse; margin-top: 16px; font-size: 13px; }
        th, td { padding: 8px 12px; border-bottom: 1px solid #2e3345; text-align: left; }
        th { background: #1a1d27; color: #8b8fa3; }
        @media print {
            body { background: #fff !important; color: #000 !important; }
            .kpi-card { border-color: #ddd !important; background: #f9f9f9 !important; }
            .kpi-value { color: #000 !important; }
            th, td { border-color: #ddd !important; color: #000 !important; }
        }
    </style>
</head>
<body>
    ${latestReportHTML}
</body>
</html>`;

    const blob = new Blob([fullDocument], { type: 'text/html' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `morphic_report_${new Date().toISOString().slice(0, 10)}.html`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);

    UI.showToast('HTML report downloaded', 'success');
}

export function reporterPrintPDF() {
    if (!latestReportHTML) {
        UI.showToast('Please generate a report first', 'warning');
        return;
    }
    window.print();
}
