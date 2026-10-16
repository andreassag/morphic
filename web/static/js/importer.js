// ── Importer Module: Media Ingestion Hub (URL, S3, Devices) ───────────

import * as UI from './ui.js';

let currentSource = 'url'; // 'url', 's3', 'device'
let importerJobId = null;
let importerPollTimer = null;

export function initImporter() {
    // Optional setup
}

export function importerInitTab() {
    const input = document.getElementById('importerFolder');
    if (input && !input.value) {
        const convInput = document.getElementById('convFolder');
        if (convInput && convInput.value) {
            input.value = convInput.value;
        }
    }
}

export function importerSetSource(source) {
    currentSource = source;
    const formUrl = document.getElementById('importerSourceUrl');
    const formS3 = document.getElementById('importerSourceS3');
    const formDev = document.getElementById('importerSourceDevice');

    const btnUrl = document.getElementById('btnImportUrl');
    const btnS3 = document.getElementById('btnImportS3');
    const btnDev = document.getElementById('btnImportDevice');

    if (formUrl) formUrl.style.display = source === 'url' ? 'block' : 'none';
    if (formS3) formS3.style.display = source === 's3' ? 'block' : 'none';
    if (formDev) formDev.style.display = source === 'device' ? 'block' : 'none';

    btnUrl?.classList.toggle('active', source === 'url');
    btnS3?.classList.toggle('active', source === 's3');
    btnDev?.classList.toggle('active', source === 'device');

    if (source === 'device') {
        importerScanDevices();
    }
}

export async function importerScanDevices() {
    const listEl = document.getElementById('importerDeviceList');
    if (!listEl) return;
    listEl.innerHTML = '<span class="spinner"></span> Scanning system mount points…';

    try {
        const res = await fetch('/api/importer/devices');
        if (!res.ok) throw new Error(`HTTP ${res.status}`);

        const data = await res.json();
        const devices = Array.isArray(data.devices) ? data.devices : [];

        if (devices.length === 0) {
            listEl.innerHTML = '<div style="padding:12px; color:var(--text-dim); font-size:13px;">No external devices or camera mounts detected. Specify a mount path below.</div>';
            return;
        }

        listEl.innerHTML = devices.map(d => {
            const safePath = d.path.replace(/'/g, "\\'");
            return `
                <div class="fb-item" style="justify-content:space-between; background:var(--surface2); margin-bottom:6px; border-radius:var(--radius-sm);" onclick="importerSelectDevice('${safePath}')">
                    <div style="display:flex; align-items:center; gap:8px;">
                        <span>💾</span>
                        <div>
                            <div style="font-weight:600;">${d.name || d.path}</div>
                            <div class="mono-text" style="font-size:11px;">${d.path} ${d.size ? `(${UI.formatFileSize(d.size)})` : ''}</div>
                        </div>
                    </div>
                    <button class="btn btn-ghost btn-sm">Select</button>
                </div>
            `;
        }).join('');
    } catch (_) {
        listEl.innerHTML = '<div style="padding:12px; color:var(--text-dim); font-size:13px;">No external devices detected via auto-probe. Enter a custom mount path below.</div>';
    }
}

export function importerSelectDevice(path) {
    const input = document.getElementById('importerDevicePath');
    if (input) input.value = path;
    UI.showToast(`Selected device: ${path}`, 'info');
}

export async function importerStart() {
    const targetFolder = document.getElementById('importerFolder')?.value.trim() || '';
    if (!targetFolder) {
        UI.showToast('Please specify a target local directory', 'warning');
        return;
    }

    const chainOrganize = document.getElementById('importerChainOrganize')?.checked || false;
    const chainConvert = document.getElementById('importerChainConvert')?.checked || false;

    const payload = {
        source_type: currentSource,
        target_folder: targetFolder,
        chain_workflow: {
            organize: chainOrganize,
            convert: chainConvert
        }
    };

    if (currentSource === 'url') {
        const rawUrls = document.getElementById('importerUrls')?.value || '';
        const urls = rawUrls.split('\n').map(u => u.trim()).filter(u => u.startsWith('http://') || u.startsWith('https://'));
        if (urls.length === 0) {
            UI.showToast('Please enter at least one valid HTTP/HTTPS URL', 'warning');
            return;
        }
        payload.url_config = { urls };
    } else if (currentSource === 's3') {
        payload.s3_config = {
            endpoint: document.getElementById('importerS3Endpoint')?.value.trim() || '',
            region: document.getElementById('importerS3Region')?.value.trim() || 'us-east-1',
            bucket: document.getElementById('importerS3Bucket')?.value.trim() || '',
            prefix: document.getElementById('importerS3Prefix')?.value.trim() || '',
            access_key: document.getElementById('importerS3AccessKey')?.value.trim() || '',
            secret_key: document.getElementById('importerS3SecretKey')?.value.trim() || ''
        };
        if (!payload.s3_config.bucket) {
            UI.showToast('Please provide an S3 bucket name', 'warning');
            return;
        }
    } else if (currentSource === 'device') {
        const devicePath = document.getElementById('importerDevicePath')?.value.trim() || '';
        if (!devicePath) {
            UI.showToast('Please specify a device mount path', 'warning');
            return;
        }
        payload.device_config = { device_path: devicePath };
    }

    const progressCard = document.getElementById('importerProgressCard');
    const progressFill = document.getElementById('importerProgressFill');
    const progressMsg = document.getElementById('importerProgressMsg');
    const progressPct = document.getElementById('importerProgressPct');
    const startBtn = document.getElementById('importerStartBtn');

    if (progressCard) progressCard.style.display = 'block';
    if (progressFill) progressFill.style.width = '10%';
    if (progressMsg) progressMsg.textContent = 'Connecting to source…';
    if (progressPct) progressPct.textContent = '0%';
    if (startBtn) startBtn.disabled = true;

    try {
        const res = await fetch('/api/importer/import', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload)
        });

        if (!res.ok) {
            const err = await res.json().catch(() => ({ error: 'Import failed' }));
            throw new Error(err.error || `HTTP ${res.status}`);
        }

        const data = await res.json();
        if (data.job_id) {
            importerJobId = data.job_id;
            pollImporterJob(data.job_id);
        } else {
            renderImporterResults(data.items || []);
            if (progressCard) progressCard.style.display = 'none';
        }
    } catch (err) {
        console.error('Import error:', err);
        UI.showToast(`Import error: ${err.message}`, 'error');
        if (progressCard) progressCard.style.display = 'none';
    } finally {
        if (startBtn) startBtn.disabled = false;
    }
}

function pollImporterJob(jobId) {
    clearInterval(importerPollTimer);
    const progressFill = document.getElementById('importerProgressFill');
    const progressMsg = document.getElementById('importerProgressMsg');
    const progressPct = document.getElementById('importerProgressPct');
    const progressCard = document.getElementById('importerProgressCard');

    importerPollTimer = setInterval(async () => {
        try {
            const res = await fetch(`/api/importer/status/${jobId}`);
            if (!res.ok) return;

            const job = await res.json();
            const pct = job.total ? Math.round((job.processed / job.total) * 100) : 0;
            if (progressFill) progressFill.style.width = `${pct}%`;
            if (progressPct) progressPct.textContent = `${pct}%`;
            if (progressMsg) progressMsg.textContent = `Imported ${job.processed || 0} / ${job.total || 0} items`;

            if (job.status === 'completed' || job.status === 'failed' || job.status === 'cancelled') {
                clearInterval(importerPollTimer);
                if (progressCard) progressCard.style.display = 'none';

                if (job.status === 'completed') {
                    renderImporterResults(job.items || []);
                    UI.showToast('Media imported successfully', 'success');
                } else if (job.status === 'cancelled') {
                    UI.showToast('Import cancelled', 'info');
                } else {
                    UI.showToast(`Import failed: ${job.error || 'Unknown error'}`, 'error');
                }
            }
        } catch (_) {}
    }, 600);
}

function renderImporterResults(items) {
    const card = document.getElementById('importerResultsCard');
    const body = document.getElementById('importerLogTableBody');
    if (!card || !body) return;

    card.style.display = 'block';

    if (items.length === 0) {
        body.innerHTML = '<tr><td colspan="4" style="text-align:center; padding:16px; color:var(--text-dim);">No files recorded</td></tr>';
        return;
    }

    body.innerHTML = items.map(item => `
        <tr>
            <td><span class="badge badge-success">IMPORTED</span></td>
            <td><div class="mono-text">${item.source || '-'}</div></td>
            <td><div style="font-weight:600;">${item.target_path || item.path || '-'}</div></td>
            <td>${UI.formatFileSize(item.size)}</td>
        </tr>
    `).join('');
}

export async function importerCancel() {
    if (!importerJobId) return;
    try {
        await fetch(`/api/importer/cancel/${importerJobId}`, { method: 'POST' });
        clearInterval(importerPollTimer);
        const progressCard = document.getElementById('importerProgressCard');
        if (progressCard) progressCard.style.display = 'none';
        UI.showToast('Cancelling import…', 'info');
    } catch (err) {
        console.error('Cancel importer error:', err);
    }
}
