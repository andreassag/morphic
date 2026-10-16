// ── Hasher Module: Checksum Tool & Bit-Rot Integrity Verifier ─────────

import * as UI from './ui.js';

let currentJobId = null;
let pollTimer = null;
let hasherResults = [];

export function initHasher() {
    // Optional setup
}

export function hasherInitTab() {
    const input = document.getElementById('hasherFolder');
    if (input && !input.value) {
        const convInput = document.getElementById('convFolder');
        if (convInput && convInput.value) {
            input.value = convInput.value;
        }
    }
}

export function hasherSwitchMode(mode) {
    const genSec = document.getElementById('hasherSectionGenerate');
    const verSec = document.getElementById('hasherSectionVerify');
    const btnGen = document.getElementById('btnHasherModeGen');
    const btnVer = document.getElementById('btnHasherModeVerify');

    if (mode === 'generate') {
        if (genSec) genSec.style.display = 'block';
        if (verSec) verSec.style.display = 'none';
        btnGen?.classList.add('active');
        btnVer?.classList.remove('active');
    } else {
        if (genSec) genSec.style.display = 'none';
        if (verSec) verSec.style.display = 'block';
        btnGen?.classList.remove('active');
        btnVer?.classList.add('active');
    }
}

// ── Checksum Generation ────────────────────────────────────────────────

export async function hasherStartGenerate() {
    const folderInput = document.getElementById('hasherFolder');
    const folder = folderInput ? folderInput.value.trim() : '';
    if (!folder) {
        UI.showToast('Please specify a directory to hash', 'warning');
        return;
    }

    const algorithm = document.getElementById('hasherAlgorithm')?.value || 'sha256';
    const recursive = document.getElementById('hasherRecursive')?.checked ?? true;

    const progressCard = document.getElementById('hasherProgressCard');
    const progressFill = document.getElementById('hasherProgressFill');
    const progressMsg = document.getElementById('hasherProgressMsg');
    const progressPct = document.getElementById('hasherProgressPct');
    const startBtn = document.getElementById('hasherStartBtn');

    if (progressCard) progressCard.style.display = 'block';
    if (progressFill) progressFill.style.width = '10%';
    if (progressMsg) progressMsg.textContent = 'Computing checksums…';
    if (progressPct) progressPct.textContent = '0%';
    if (startBtn) startBtn.disabled = true;

    try {
        const res = await fetch('/api/hasher/generate', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ folder, algorithm, recursive })
        });

        if (!res.ok) {
            const err = await res.json().catch(() => ({ error: 'Hashing failed' }));
            throw new Error(err.error || `HTTP ${res.status}`);
        }

        const data = await res.json();
        if (data.job_id) {
            currentJobId = data.job_id;
            pollJobStatus(data.job_id);
        } else if (Array.isArray(data.results)) {
            hasherResults = data.results;
            renderResultsTable(hasherResults);
            if (progressCard) progressCard.style.display = 'none';
            UI.showToast(`Hashed ${hasherResults.length} files successfully`, 'success');
        }
    } catch (err) {
        console.error('Hasher error:', err);
        UI.showToast(`Hash generation error: ${err.message}`, 'error');
        if (progressCard) progressCard.style.display = 'none';
    } finally {
        if (startBtn) startBtn.disabled = false;
    }
}

function pollJobStatus(jobId) {
    clearInterval(pollTimer);
    const progressFill = document.getElementById('hasherProgressFill');
    const progressMsg = document.getElementById('hasherProgressMsg');
    const progressPct = document.getElementById('hasherProgressPct');
    const progressCard = document.getElementById('hasherProgressCard');

    pollTimer = setInterval(async () => {
        try {
            const res = await fetch(`/api/hasher/status/${jobId}`);
            if (!res.ok) return;

            const job = await res.json();
            const pct = job.total ? Math.round((job.processed / job.total) * 100) : 0;
            if (progressFill) progressFill.style.width = `${pct}%`;
            if (progressPct) progressPct.textContent = `${pct}%`;
            if (progressMsg) progressMsg.textContent = `Hashed ${job.processed || 0} / ${job.total || 0} files`;

            if (job.status === 'completed' || job.status === 'failed' || job.status === 'cancelled') {
                clearInterval(pollTimer);
                if (progressCard) progressCard.style.display = 'none';
                if (job.status === 'completed') {
                    hasherResults = Array.isArray(job.results) ? job.results : [];
                    renderResultsTable(hasherResults);
                    UI.showToast(`Completed hashing ${hasherResults.length} files`, 'success');
                } else if (job.status === 'cancelled') {
                    UI.showToast('Hashing cancelled', 'info');
                } else {
                    UI.showToast(`Hashing failed: ${job.error || 'Unknown error'}`, 'error');
                }
            }
        } catch (_) {}
    }, 500);
}

export async function hasherCancelJob() {
    if (!currentJobId) return;
    try {
        await fetch(`/api/hasher/cancel/${currentJobId}`, { method: 'POST' });
        clearInterval(pollTimer);
        const progressCard = document.getElementById('hasherProgressCard');
        if (progressCard) progressCard.style.display = 'none';
        UI.showToast('Cancelling hash job…', 'info');
    } catch (err) {
        console.error('Cancel error:', err);
    }
}

function renderResultsTable(results) {
    const card = document.getElementById('hasherResultsCard');
    const body = document.getElementById('hasherTableBody');
    const countEl = document.getElementById('hasherResultCount');

    if (!card || !body) return;
    card.style.display = 'block';
    if (countEl) countEl.textContent = results.length;

    if (results.length === 0) {
        body.innerHTML = '<tr><td colspan="4" style="text-align:center; padding:20px; color:var(--text-dim);">No files processed</td></tr>';
        return;
    }

    body.innerHTML = results.map(r => {
        const hash = r.sha256 || r.md5 || r.xxhash64 || r.hash || '-';
        const safeHash = hash.replace(/'/g, "\\'");
        return `
            <tr>
                <td>
                    <div style="font-weight:600;">${r.name || r.path.split('/').pop()}</div>
                    <div class="mono-text" style="font-size:11px;">${r.path}</div>
                </td>
                <td>${UI.formatFileSize(r.size)}</td>
                <td>
                    <span class="hash-pill">
                        ${hash}
                        <button class="copy-btn" onclick="hasherCopyHash('${safeHash}')" title="Copy checksum">📋</button>
                    </span>
                </td>
                <td style="text-align:right;">
                    <button class="btn btn-ghost btn-sm" onclick="openPreview('${r.path.replace(/'/g, "\\'")}')">👁️</button>
                </td>
            </tr>
        `;
    }).join('');
}

export function hasherCopyHash(hash) {
    navigator.clipboard.writeText(hash).then(() => {
        UI.showToast('Checksum copied to clipboard', 'info');
    }).catch(() => {
        UI.showToast('Failed to copy checksum', 'warning');
    });
}

export function hasherExportManifest(format) {
    if (hasherResults.length === 0) {
        UI.showToast('No checksums available to export', 'warning');
        return;
    }

    let content = '';
    let filename = `manifest.${format}`;
    let mimeType = 'text/plain';

    if (format === 'json') {
        content = JSON.stringify(hasherResults, null, 2);
        mimeType = 'application/json';
    } else {
        // Standard unix checksum format: "<hash>  <relative_path>"
        content = hasherResults.map(r => {
            const h = r[format] || r.hash || '';
            return `${h}  ${r.path}`;
        }).join('\n');
    }

    const blob = new Blob([content], { type: mimeType });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);

    UI.showToast(`Exported ${filename}`, 'success');
}

// ── Bit-Rot Integrity Verification ─────────────────────────────────────

export async function hasherStartVerify() {
    const folder = document.getElementById('hasherVerifyFolder')?.value.trim() || '';
    const manifestPath = document.getElementById('hasherManifestPath')?.value.trim() || '';

    if (!manifestPath) {
        UI.showToast('Please specify a manifest file path', 'warning');
        return;
    }

    const verifyBtn = document.getElementById('hasherVerifyBtn');
    if (verifyBtn) verifyBtn.disabled = true;

    try {
        const res = await fetch('/api/hasher/verify', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ folder, manifest_path: manifestPath })
        });

        if (!res.ok) {
            const err = await res.json().catch(() => ({ error: 'Verification failed' }));
            throw new Error(err.error || `HTTP ${res.status}`);
        }

        const data = await res.json();
        renderVerificationReport(data);
        UI.showToast('Integrity verification complete', 'success');
    } catch (err) {
        console.error('Verify error:', err);
        UI.showToast(`Verification error: ${err.message}`, 'error');
    } finally {
        if (verifyBtn) verifyBtn.disabled = false;
    }
}

function renderVerificationReport(data) {
    const kpis = document.getElementById('hasherVerifyKpis');
    const resultsCard = document.getElementById('hasherVerifyResultsCard');
    const tableBody = document.getElementById('hasherVerifyTableBody');

    if (!kpis || !resultsCard || !tableBody) return;

    kpis.style.display = 'grid';
    resultsCard.style.display = 'block';

    const okCount = data.ok_count || 0;
    const corruptCount = data.corrupted_count || 0;
    const missingCount = data.missing_count || 0;
    const newCount = data.new_count || 0;

    document.getElementById('hasherKpiOk').textContent = okCount;
    document.getElementById('hasherKpiCorrupt').textContent = corruptCount;
    document.getElementById('hasherKpiMissing').textContent = missingCount;
    document.getElementById('hasherKpiNew').textContent = newCount;

    const files = Array.isArray(data.files) ? data.files : [];
    if (files.length === 0) {
        tableBody.innerHTML = '<tr><td colspan="4" style="text-align:center; padding:16px; color:var(--text-dim);">All verified files matched cleanly</td></tr>';
        return;
    }

    tableBody.innerHTML = files.map(f => {
        let badgeClass = 'badge-hash-ok';
        let statusText = 'OK';
        if (f.status === 'corrupted') { badgeClass = 'badge-hash-corrupt'; statusText = 'CORRUPTED'; }
        else if (f.status === 'missing') { badgeClass = 'badge-hash-missing'; statusText = 'MISSING'; }
        else if (f.status === 'new') { badgeClass = 'badge-hash-new'; statusText = 'NEW'; }

        return `
            <tr>
                <td><span class="badge ${badgeClass}">${statusText}</span></td>
                <td><div class="mono-text">${f.path}</div></td>
                <td><span class="mono-text" style="font-size:11px;">${f.expected_hash || '-'}</span></td>
                <td><span class="mono-text" style="font-size:11px;">${f.actual_hash || '-'}</span></td>
            </tr>
        `;
    }).join('');
}
