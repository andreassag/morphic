// ── Archiver Module: Batch Compression & Multi-Part Splitting ─────────

import * as UI from './ui.js';

let archiverJobId = null;
let archiverPollTimer = null;

export function initArchiver() {
    // Optional setup
}

export function archiverInitTab() {
    const input = document.getElementById('archiverFolder');
    if (input && !input.value) {
        const convInput = document.getElementById('convFolder');
        if (convInput && convInput.value) {
            input.value = convInput.value;
            const outInput = document.getElementById('archiverOutputPath');
            if (outInput && !outInput.value) {
                outInput.value = `${convInput.value.replace(/\/+$/, '')}_archive.zip`;
            }
        }
    }
}

export function archiverToggleSplit(checked) {
    const opts = document.getElementById('archiverSplitOptions');
    if (opts) opts.style.display = checked ? 'flex' : 'none';
}

export function archiverSetSplitPreset(mb) {
    const input = document.getElementById('archiverSplitSize');
    if (input) input.value = mb;
}

export function archiverOnFormatChange(format) {
    const outInput = document.getElementById('archiverOutputPath');
    if (outInput && outInput.value) {
        let val = outInput.value;
        val = val.replace(/\.(zip|tar\.gz|7z)$/i, '');
        if (format === 'zip') outInput.value = `${val}.zip`;
        else if (format === 'tar.gz') outInput.value = `${val}.tar.gz`;
        else if (format === '7z') outInput.value = `${val}.7z`;
    }
}

export async function archiverCreate() {
    const folder = document.getElementById('archiverFolder')?.value.trim() || '';
    const outputPath = document.getElementById('archiverOutputPath')?.value.trim() || '';

    if (!folder) {
        UI.showToast('Please specify a source directory to archive', 'warning');
        return;
    }
    if (!outputPath) {
        UI.showToast('Please specify an output archive path', 'warning');
        return;
    }

    const format = document.getElementById('archiverFormat')?.value || 'zip';
    const level = document.getElementById('archiverLevel')?.value || 'normal';
    const splitEnabled = document.getElementById('archiverSplitToggle')?.checked || false;
    const splitSizeMb = splitEnabled ? (parseInt(document.getElementById('archiverSplitSize')?.value) || 0) : 0;

    const progressCard = document.getElementById('archiverProgressCard');
    const progressFill = document.getElementById('archiverProgressFill');
    const progressMsg = document.getElementById('archiverProgressMsg');
    const progressPct = document.getElementById('archiverProgressPct');
    const startBtn = document.getElementById('archiverStartBtn');
    const summaryCard = document.getElementById('archiverSummaryCard');

    if (summaryCard) summaryCard.style.display = 'none';
    if (progressCard) progressCard.style.display = 'block';
    if (progressFill) progressFill.style.width = '10%';
    if (progressMsg) progressMsg.textContent = 'Preparing archive…';
    if (progressPct) progressPct.textContent = '0%';
    if (startBtn) startBtn.disabled = true;

    try {
        const res = await fetch('/api/archiver/create', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                folder,
                output_path: outputPath,
                format,
                compression: level,
                split_size_mb: splitSizeMb
            })
        });

        if (!res.ok) {
            const err = await res.json().catch(() => ({ error: 'Archive creation failed' }));
            throw new Error(err.error || `HTTP ${res.status}`);
        }

        const data = await res.json();
        if (data.job_id) {
            archiverJobId = data.job_id;
            pollArchiverJob(data.job_id, outputPath);
        } else {
            showArchiveSummary(data, outputPath);
            if (progressCard) progressCard.style.display = 'none';
        }
    } catch (err) {
        console.error('Archiver error:', err);
        UI.showToast(`Error creating archive: ${err.message}`, 'error');
        if (progressCard) progressCard.style.display = 'none';
    } finally {
        if (startBtn) startBtn.disabled = false;
    }
}

function pollArchiverJob(jobId, outputPath) {
    clearInterval(archiverPollTimer);
    const progressFill = document.getElementById('archiverProgressFill');
    const progressMsg = document.getElementById('archiverProgressMsg');
    const progressPct = document.getElementById('archiverProgressPct');
    const progressCard = document.getElementById('archiverProgressCard');

    archiverPollTimer = setInterval(async () => {
        try {
            const res = await fetch(`/api/archiver/status/${jobId}`);
            if (!res.ok) return;

            const job = await res.json();
            const pct = job.total ? Math.round((job.processed / job.total) * 100) : 0;
            if (progressFill) progressFill.style.width = `${pct}%`;
            if (progressPct) progressPct.textContent = `${pct}%`;
            if (progressMsg) progressMsg.textContent = `Archiving: ${job.processed || 0} / ${job.total || 0} files`;

            if (job.status === 'completed' || job.status === 'failed' || job.status === 'cancelled') {
                clearInterval(archiverPollTimer);
                if (progressCard) progressCard.style.display = 'none';

                if (job.status === 'completed') {
                    showArchiveSummary(job, outputPath);
                    UI.showToast('Archive created successfully', 'success');
                } else if (job.status === 'cancelled') {
                    UI.showToast('Archiving cancelled', 'info');
                } else {
                    UI.showToast(`Archiving failed: ${job.error || 'Unknown error'}`, 'error');
                }
            }
        } catch (_) {}
    }, 600);
}

function showArchiveSummary(job, outputPath) {
    const summaryCard = document.getElementById('archiverSummaryCard');
    const resultPath = document.getElementById('archiverResultPath');
    const resultSize = document.getElementById('archiverResultSize');
    const resultCount = document.getElementById('archiverResultCount');

    if (!summaryCard) return;
    summaryCard.style.display = 'block';

    if (resultPath) resultPath.textContent = job.output_path || outputPath;
    if (resultSize) resultSize.textContent = UI.formatFileSize(job.size || job.total_bytes || 0);
    if (resultCount) resultCount.textContent = (job.total || job.processed || 0).toLocaleString();
}

export async function archiverCancel() {
    if (!archiverJobId) return;
    try {
        await fetch(`/api/archiver/cancel/${archiverJobId}`, { method: 'POST' });
        clearInterval(archiverPollTimer);
        const progressCard = document.getElementById('archiverProgressCard');
        if (progressCard) progressCard.style.display = 'none';
        UI.showToast('Cancelling archive job…', 'info');
    } catch (err) {
        console.error('Cancel archiver error:', err);
    }
}
