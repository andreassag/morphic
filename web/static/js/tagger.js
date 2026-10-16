// ── Tagger Module: Bulk Metadata Editor & Privacy Stripper ────────────

import * as UI from './ui.js';

let taggerFiles = [];
let selectedFiles = new Set();
let isScanning = false;

export function initTagger() {
    // Optional initial setup
}

export function taggerInitTab() {
    const input = document.getElementById('taggerFolder');
    if (input && !input.value) {
        const convInput = document.getElementById('convFolder');
        if (convInput && convInput.value) {
            input.value = convInput.value;
        }
    }
}

export async function taggerScan() {
    const folderInput = document.getElementById('taggerFolder');
    const folder = folderInput ? folderInput.value.trim() : '';
    if (!folder) {
        UI.showToast('Please specify a folder to scan', 'warning');
        return;
    }

    const recursive = document.getElementById('taggerRecursive')?.checked ?? true;
    const imagesOnly = document.getElementById('taggerImagesOnly')?.checked ?? true;

    const progressCard = document.getElementById('taggerProgressCard');
    const progressFill = document.getElementById('taggerProgressFill');
    const progressMsg = document.getElementById('taggerProgressMsg');
    const scanBtn = document.getElementById('taggerScanBtn');

    if (progressCard) progressCard.style.display = 'block';
    if (progressFill) progressFill.style.width = '30%';
    if (progressMsg) progressMsg.textContent = 'Scanning metadata…';
    if (scanBtn) scanBtn.disabled = true;
    isScanning = true;

    try {
        const res = await fetch('/api/tagger/scan', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ folder, recursive, images_only: imagesOnly })
        });

        if (!res.ok) {
            const err = await res.json().catch(() => ({ error: 'Scan failed' }));
            throw new Error(err.error || `HTTP ${res.status}`);
        }

        const data = await res.json();
        taggerFiles = Array.isArray(data.files) ? data.files : [];
        selectedFiles.clear();

        if (progressFill) progressFill.style.width = '100%';
        if (progressMsg) progressMsg.textContent = `Found ${taggerFiles.length} media files`;

        setTimeout(() => {
            if (progressCard) progressCard.style.display = 'none';
        }, 800);

        renderTaggerTable(taggerFiles);
        UI.showToast(`Found ${taggerFiles.length} files with metadata`, 'success');
    } catch (err) {
        console.error('Tagger scan error:', err);
        UI.showToast(`Scan error: ${err.message}`, 'error');
        if (progressCard) progressCard.style.display = 'none';
    } finally {
        if (scanBtn) scanBtn.disabled = false;
        isScanning = false;
    }
}

function renderTaggerTable(files) {
    const resultsCard = document.getElementById('taggerResultsCard');
    const tableBody = document.getElementById('taggerTableBody');
    const totalFilesEl = document.getElementById('taggerTotalFiles');
    const selectAllCheck = document.getElementById('taggerSelectAll');

    if (!resultsCard || !tableBody) return;

    if (files.length === 0) {
        resultsCard.style.display = 'block';
        tableBody.innerHTML = `<tr><td colspan="9" style="text-align:center; padding:24px; color:var(--text-dim);">No files found</td></tr>`;
        if (totalFilesEl) totalFilesEl.textContent = '0';
        return;
    }

    resultsCard.style.display = 'block';
    if (totalFilesEl) totalFilesEl.textContent = files.length;
    if (selectAllCheck) selectAllCheck.checked = false;

    tableBody.innerHTML = files.map((f, idx) => {
        const isChecked = selectedFiles.has(f.path);
        const thumbUrl = `/api/thumbnail?path=${encodeURIComponent(f.path)}`;
        const dateStr = f.date_original || f.mod_time || '-';
        const cameraStr = [f.make, f.model].filter(Boolean).join(' ') || '-';
        const titleDesc = [f.title, f.description].filter(Boolean).join(' / ') || '-';
        const gpsStr = (f.gps_lat && f.gps_lon) ? `${Number(f.gps_lat).toFixed(4)}, ${Number(f.gps_lon).toFixed(4)}` : '-';
        const keywordsStr = Array.isArray(f.keywords) ? f.keywords.join(', ') : (f.keywords || '-');
        const safePath = f.path.replace(/'/g, "\\'");

        return `
            <tr data-path="${f.path}">
                <td>
                    <input type="checkbox" ${isChecked ? 'checked' : ''} onchange="taggerToggleSelectFile('${safePath}', this.checked)" aria-label="Select ${f.name}" />
                </td>
                <td>
                    <img class="trash-thumb" src="${thumbUrl}" alt="thumb" loading="lazy" onclick="openPreview('${safePath}')" onerror="this.style.opacity='0.3'" />
                </td>
                <td>
                    <div style="font-weight:600; color:var(--text);">${f.name || f.path.split('/').pop()}</div>
                    <div class="mono-text" style="font-size:11px;">${f.path}</div>
                </td>
                <td style="white-space:nowrap; font-size:12px;">${dateStr}</td>
                <td style="font-size:12px;">${cameraStr}</td>
                <td style="font-size:12px; max-width:200px; overflow:hidden; text-overflow:ellipsis;">${titleDesc}</td>
                <td style="font-size:12px; white-space:nowrap;">${gpsStr}</td>
                <td style="font-size:12px; max-width:180px; overflow:hidden; text-overflow:ellipsis;">${keywordsStr}</td>
                <td style="text-align:right; white-space:nowrap;">
                    <button class="btn btn-ghost btn-sm" onclick="taggerEditSingle('${safePath}')" title="Edit tags">✏️</button>
                    <button class="btn btn-ghost btn-sm" onclick="taggerStripSingle('${safePath}')" title="Strip metadata" style="color:var(--danger);">🛡️</button>
                </td>
            </tr>
        `;
    }).join('');

    updateBatchCardState();
}

export function taggerFilterResults() {
    const q = document.getElementById('taggerSearch')?.value.toLowerCase().trim() || '';
    if (!q) {
        renderTaggerTable(taggerFiles);
        return;
    }

    const filtered = taggerFiles.filter(f => {
        const text = [
            f.name, f.path, f.title, f.description, f.make, f.model,
            Array.isArray(f.keywords) ? f.keywords.join(' ') : f.keywords
        ].filter(Boolean).join(' ').toLowerCase();
        return text.includes(q);
    });

    renderTaggerTable(filtered);
}

export function taggerToggleSelectAll(checked) {
    if (checked) {
        taggerFiles.forEach(f => selectedFiles.add(f.path));
    } else {
        selectedFiles.clear();
    }

    document.querySelectorAll('#taggerTableBody input[type="checkbox"]').forEach(cb => {
        cb.checked = checked;
    });

    updateBatchCardState();
}

export function taggerToggleSelectFile(path, checked) {
    if (checked) {
        selectedFiles.add(path);
    } else {
        selectedFiles.delete(path);
    }

    const selectAllCheck = document.getElementById('taggerSelectAll');
    if (selectAllCheck) {
        selectAllCheck.checked = selectedFiles.size === taggerFiles.length && taggerFiles.length > 0;
    }

    updateBatchCardState();
}

export function taggerDeselectAll() {
    selectedFiles.clear();
    const selectAllCheck = document.getElementById('taggerSelectAll');
    if (selectAllCheck) selectAllCheck.checked = false;

    document.querySelectorAll('#taggerTableBody input[type="checkbox"]').forEach(cb => {
        cb.checked = false;
    });

    updateBatchCardState();
}

function updateBatchCardState() {
    const batchCard = document.getElementById('taggerBatchCard');
    const selectedCountEl = document.getElementById('taggerSelectedCount');
    const bulkBar = document.getElementById('taggerBulkBar');

    const count = selectedFiles.size;
    if (selectedCountEl) selectedCountEl.textContent = count;

    if (count > 0) {
        if (batchCard) batchCard.style.display = 'block';
        if (bulkBar) bulkBar.style.display = 'flex';
    } else {
        if (batchCard) batchCard.style.display = 'none';
        if (bulkBar) bulkBar.style.display = 'none';
    }
}

export function taggerEditSingle(path) {
    selectedFiles.clear();
    selectedFiles.add(path);

    document.querySelectorAll('#taggerTableBody input[type="checkbox"]').forEach(cb => {
        const row = cb.closest('tr');
        cb.checked = row && row.getAttribute('data-path') === path;
    });

    const file = taggerFiles.find(f => f.path === path);
    if (file) {
        if (document.getElementById('taggerBatchTitle')) document.getElementById('taggerBatchTitle').value = file.title || '';
        if (document.getElementById('taggerBatchDesc')) document.getElementById('taggerBatchDesc').value = file.description || '';
        if (document.getElementById('taggerBatchCopyright')) document.getElementById('taggerBatchCopyright').value = file.copyright || '';
        if (document.getElementById('taggerBatchLat')) document.getElementById('taggerBatchLat').value = file.gps_lat || '';
        if (document.getElementById('taggerBatchLon')) document.getElementById('taggerBatchLon').value = file.gps_lon || '';
        if (document.getElementById('taggerBatchKeywords')) {
            document.getElementById('taggerBatchKeywords').value = Array.isArray(file.keywords) ? file.keywords.join(', ') : (file.keywords || '');
        }
    }

    updateBatchCardState();
    const batchCard = document.getElementById('taggerBatchCard');
    if (batchCard) batchCard.scrollIntoView({ behavior: 'smooth' });
}

export async function taggerApplyBatch() {
    if (selectedFiles.size === 0) {
        UI.showToast('No files selected', 'warning');
        return;
    }

    const patch = {};
    const title = document.getElementById('taggerBatchTitle')?.value.trim();
    const desc = document.getElementById('taggerBatchDesc')?.value.trim();
    const copyright = document.getElementById('taggerBatchCopyright')?.value.trim();
    const lat = document.getElementById('taggerBatchLat')?.value.trim();
    const lon = document.getElementById('taggerBatchLon')?.value.trim();
    const kw = document.getElementById('taggerBatchKeywords')?.value.trim();

    if (title) patch.title = title;
    if (desc) patch.description = desc;
    if (copyright) patch.copyright = copyright;
    if (lat) patch.gps_lat = parseFloat(lat);
    if (lon) patch.gps_lon = parseFloat(lon);
    if (kw) patch.keywords = kw.split(',').map(s => s.trim()).filter(Boolean);

    try {
        const res = await fetch('/api/tagger/update', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                files: Array.from(selectedFiles),
                patch
            })
        });

        if (!res.ok) {
            const err = await res.json().catch(() => ({ error: 'Update failed' }));
            throw new Error(err.error || `HTTP ${res.status}`);
        }

        UI.showToast(`Updated tags for ${selectedFiles.size} files`, 'success');
        // Refresh local items
        taggerFiles.forEach(f => {
            if (selectedFiles.has(f.path)) {
                if (patch.title !== undefined) f.title = patch.title;
                if (patch.description !== undefined) f.description = patch.description;
                if (patch.copyright !== undefined) f.copyright = patch.copyright;
                if (patch.gps_lat !== undefined) f.gps_lat = patch.gps_lat;
                if (patch.gps_lon !== undefined) f.gps_lon = patch.gps_lon;
                if (patch.keywords !== undefined) f.keywords = patch.keywords;
            }
        });

        renderTaggerTable(taggerFiles);
    } catch (err) {
        console.error('Batch tag error:', err);
        UI.showToast(`Error updating tags: ${err.message}`, 'error');
    }
}

export async function taggerConfirmStrip() {
    if (selectedFiles.size === 0) {
        UI.showToast('No files selected', 'warning');
        return;
    }

    const confirmed = confirm(`Are you sure you want to permanently strip ALL metadata (EXIF, IPTC, XMP, GPS) from ${selectedFiles.size} selected file(s)? This cannot be undone.`);
    if (!confirmed) return;

    try {
        const res = await fetch('/api/tagger/strip', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ files: Array.from(selectedFiles) })
        });

        if (!res.ok) {
            const err = await res.json().catch(() => ({ error: 'Strip failed' }));
            throw new Error(err.error || `HTTP ${res.status}`);
        }

        UI.showToast(`Metadata stripped successfully from ${selectedFiles.size} files`, 'success');
        taggerScan(); // rescan folder to reflect cleared tags
    } catch (err) {
        console.error('Strip metadata error:', err);
        UI.showToast(`Error stripping metadata: ${err.message}`, 'error');
    }
}

export async function taggerStripSingle(path) {
    const confirmed = confirm(`Strip all metadata from ${path.split('/').pop()}?`);
    if (!confirmed) return;

    try {
        const res = await fetch('/api/tagger/strip', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ files: [path] })
        });

        if (!res.ok) {
            const err = await res.json().catch(() => ({ error: 'Strip failed' }));
            throw new Error(err.error || `HTTP ${res.status}`);
        }

        UI.showToast('Metadata stripped', 'success');
        taggerScan();
    } catch (err) {
        console.error('Strip single error:', err);
        UI.showToast(`Error: ${err.message}`, 'error');
    }
}
