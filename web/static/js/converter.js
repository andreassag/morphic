// ── Converter Module ─────────────────────────────────────────────────

import { showToast, formatFileSize, openPreview } from './ui.js';
import { onEvent } from './events.js';
import { openCompareModal } from './compare.js';

let scanResults = [];
let selectedFiles = new Set();
let selectedExts = new Set(); // Multi-select filter for extensions
let formatsData = null;
let currentConvertJobId = null;
let showFullPaths = false;
let currentSortBy = 'name';
let conversionResults = new Map(); // path -> ConversionResult

export function initConverter() {
    loadFormats();

    onEvent('converter', (event) => {
        const payload = event.payload || event.data || event;
        if (event.type === 'progress') {
            updateConvertProgress(payload);
        } else if (event.type === 'job_done') {
            finishConversion(payload);
        } else if (event.type === 'job_cancelled') {
            cancelConversionUI();
        }
    });
}

async function loadFormats() {
    try {
        const res = await fetch('/api/converter/formats');
        if (res.ok) {
            formatsData = await res.json();
        }
    } catch (err) {
        console.error('Failed to load formats', err);
    }
}

export async function convScan() {
    const folder = document.getElementById('convFolder')?.value.trim();
    if (!folder) {
        showToast('Please enter a folder path to scan', 'warning');
        return;
    }

    const filterType = document.getElementById('convFilterType')?.value || 'both';
    const includeSub = document.getElementById('convSubfolders')?.checked ?? true;
    const excludeFoldersRaw = document.getElementById('convExcludeFolders')?.value.trim() || '';
    const excludeFolders = excludeFoldersRaw ? excludeFoldersRaw.split(',').map(s => s.trim()).filter(Boolean) : [];

    const scanProgress = document.getElementById('convScanProgress');
    const scanResultsDiv = document.getElementById('convResults');
    const scanBtn = document.getElementById('convScanBtn');

    if (scanProgress) scanProgress.style.display = 'block';
    if (scanResultsDiv) scanResultsDiv.style.display = 'none';
    if (scanBtn) scanBtn.disabled = true;

    try {
        const res = await fetch('/api/converter/scan', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                folder: folder,
                include_subfolders: includeSub,
                filter_type: filterType,
                exclude_folders: excludeFolders
            })
        });

        if (!res.ok) {
            const errData = await res.json();
            throw new Error(errData.error?.message || 'Scan failed');
        }

        const data = await res.json();
        scanResults = data.files || [];
        selectedExts.clear(); // Reset to show all by default
        renderScanResults(data);
        if (scanResultsDiv) scanResultsDiv.style.display = 'block';
    } catch (err) {
        showToast(err.message, 'error');
    } finally {
        if (scanProgress) scanProgress.style.display = 'none';
        if (scanBtn) scanBtn.disabled = false;
    }
}

export function convSortResults(sortBy) {
    currentSortBy = sortBy;
    if (!scanResults || scanResults.length === 0) {
        renderTableRows();
        return;
    }

    scanResults.sort((a, b) => {
        const nameA = (a.name || a.filename || '').toLowerCase();
        const nameB = (b.name || b.filename || '').toLowerCase();
        if (sortBy === 'name') {
            return nameA.localeCompare(nameB);
        } else if (sortBy === 'type') {
            const extA = (a.ext || '').toLowerCase();
            const extB = (b.ext || '').toLowerCase();
            return extA.localeCompare(extB);
        } else if (sortBy === 'size') {
            return (b.size || 0) - (a.size || 0);
        }
        return 0;
    });

    renderTableRows();
}

function renderScanResults(data) {
    const summary = document.getElementById('convSummary');
    const totalCount = data.total_count || data.total || scanResults.length;
    const totalSize = data.total_size_formatted || (data.total_size ? formatFileSize(data.total_size) : '0 B');

    if (summary) {
        summary.innerHTML = `
            <div style="font-size:13px;color:var(--text-dim);margin-bottom:8px;">
                Found <strong>${totalCount}</strong> file(s) (<strong>${totalSize}</strong>). 
                Filter by extension or select files below to batch convert.
            </div>
        `;
    }

    conversionResults.clear();
    const batchSummary = document.getElementById('convBatchSummary');
    if (batchSummary) {
        batchSummary.style.display = 'none';
        batchSummary.innerHTML = '';
    }

    renderExtFilterChips();
    selectedFiles.clear();
    convSortResults(currentSortBy);
    updateBulkBar();
}

function renderExtFilterChips() {
    const filterBar = document.getElementById('convExtFilters');
    if (!filterBar) return;

    const extCounts = {};
    scanResults.forEach(f => {
        const ext = (f.ext || '').toLowerCase();
        if (ext) extCounts[ext] = (extCounts[ext] || 0) + 1;
    });

    const exts = Object.keys(extCounts).sort();
    if (exts.length <= 1) {
        filterBar.innerHTML = '';
        return;
    }

    const isAll = selectedExts.size === 0;
    let chipsHtml = `
        <span style="font-size:12px;color:var(--text-dim);margin-right:4px;">Filter Ext:</span>
        <button class="badge ${isAll ? 'badge-primary' : 'badge-ghost'}" style="cursor:pointer;padding:4px 8px;" onclick="convToggleExtFilter('ALL')">
            All (${scanResults.length})
        </button>
    `;

    exts.forEach(ext => {
        const isActive = selectedExts.has(ext);
        chipsHtml += `
            <button class="badge ${isActive ? 'badge-primary' : 'badge-ghost'}" style="cursor:pointer;padding:4px 8px;" onclick="convToggleExtFilter('${ext}')">
                ${ext} (${extCounts[ext]})
            </button>
        `;
    });

    filterBar.innerHTML = chipsHtml;
}

export function convToggleExtFilter(ext) {
    if (ext === 'ALL') {
        selectedExts.clear();
    } else {
        if (selectedExts.has(ext)) {
            selectedExts.delete(ext);
        } else {
            selectedExts.add(ext);
        }
    }
    renderExtFilterChips();
    renderTableRows();
}

function getVisibleFiles() {
    if (selectedExts.size === 0) return scanResults;
    return scanResults.filter(f => selectedExts.has((f.ext || '').toLowerCase()));
}

function renderTableRows() {
    const tableDiv = document.getElementById('convFileTable');
    if (!tableDiv) return;

    const visibleFiles = getVisibleFiles();
    if (!visibleFiles || visibleFiles.length === 0) {
        tableDiv.innerHTML = `
            <div style="padding:32px;text-align:center;color:var(--text-dim);font-size:14px;">
                No convertible media files found in this folder.
            </div>
        `;
        const selectAll = document.getElementById('convSelectAll');
        if (selectAll) selectAll.checked = false;
        return;
    }

    let html = `
        <table class="data-table">
            <thead>
                <tr>
                    <th style="width:36px;"><input type="checkbox" id="convSelectAll" onchange="convToggleSelectAll(this.checked)" /></th>
                    <th style="width:48px;">Preview</th>
                    <th>File</th>
                    <th>Extension</th>
                    <th>Size</th>
                    <th>Status</th>
                    <th>Actions</th>
                </tr>
            </thead>
            <tbody>
    `;

    visibleFiles.forEach((file) => {
        const isChecked = selectedFiles.has(file.path) ? 'checked' : '';
        const rawName = file.name || file.filename || file.path.split('/').pop();
        const displayName = showFullPaths ? file.path : rawName;
        const initialSizeStr = file.size_formatted || formatFileSize(file.size || 0);

        let sizeHtml = `<span>${initialSizeStr}</span>`;
        let statusHtml = `<span class="badge" style="background:var(--surface3);color:var(--text-dim);">Ready</span>`;
        let actionsHtml = `<button class="btn btn-ghost btn-sm" onclick="openPreview('${file.path.replace(/'/g, "\\'")}')">👁️ View</button>`;

        if (conversionResults.has(file.path)) {
            const res = conversionResults.get(file.path);
            if (res.status === 'ok') {
                const origBytes = res.original_size || file.size || 0;
                const newBytes = res.new_size || 0;
                const origFmt = res.original_size_fmt || formatFileSize(origBytes);
                const newFmt = res.new_size_fmt || formatFileSize(newBytes);
                let pctBadge = '';
                if (origBytes > 0 && newBytes > 0) {
                    const diff = origBytes - newBytes;
                    const pct = Math.round((diff / origBytes) * 100);
                    if (diff > 0) {
                        pctBadge = `<span class="badge badge-success" style="font-size:11px;margin-left:4px;">-${pct}%</span>`;
                    } else if (diff < 0) {
                        pctBadge = `<span class="badge badge-warning" style="font-size:11px;margin-left:4px;">+${Math.abs(pct)}%</span>`;
                    } else {
                        pctBadge = `<span class="badge badge-ghost" style="font-size:11px;margin-left:4px;">0%</span>`;
                    }
                }
                sizeHtml = `<span>${origFmt} → <strong>${newFmt}</strong> ${pctBadge}</span>`;
                statusHtml = `<span class="badge badge-success">✓ Done</span>`;
                actionsHtml = `
                    <button class="btn btn-ghost btn-sm" onclick="openPreview('${(res.destination || file.path).replace(/'/g, "\\'")}')" title="Preview converted file">👁️ Converted</button>
                    ${res.destination ? `<button class="btn btn-ghost btn-sm" onclick="openCompareModal('${file.path.replace(/'/g, "\\'")}', '${res.destination.replace(/'/g, "\\'")}')" title="Compare original vs converted">🔍 Compare</button>` : ''}
                `;
            } else if (res.status === 'error') {
                statusHtml = `<span class="badge badge-danger">✕ Failed</span>`;
                sizeHtml = `
                    <span>${initialSizeStr}</span>
                    <div style="font-size:11px;color:var(--danger);margin-top:2px;max-width:240px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;" title="${(res.error || 'Conversion error').replace(/"/g, '&quot;')}">
                        ${res.error || 'Conversion failed'}
                    </div>
                `;
            }
        }

        let previewPath = file.path;
        if (conversionResults.has(file.path)) {
            const res = conversionResults.get(file.path);
            if (res.status === 'ok' && res.destination) {
                previewPath = res.destination;
            }
        }
        const safePreviewAttrPath = previewPath.replace(/'/g, "\\'");

        const safePath = file.path.replace(/"/g, '&quot;');
        const safeAttrPath = file.path.replace(/'/g, "\\'");

        html += `
            <tr data-path="${safePath}">
                <td><input type="checkbox" class="conv-file-cb" data-path="${safePath}" ${isChecked} onchange="convToggleSelectFile('${safeAttrPath}', this.checked)" /></td>
                <td>
                    <img class="trash-thumb"
                         src="/api/thumbnail?path=${encodeURIComponent(previewPath)}"
                         alt="Thumbnail"
                         loading="lazy"
                         onclick="openPreview('${safePreviewAttrPath}')"
                         onerror="this.onerror=null;this.src='data:image/svg+xml;utf8,<svg xmlns=\\'http://www.w3.org/2000/svg\\' width=\\'44\\' height=\\'44\\' viewBox=\\'0 0 24 24\\' fill=\\'none\\' stroke=\\'%238b949e\\' stroke-width=\\'2\\' stroke-linecap=\\'round\\' stroke-linejoin=\\'round\\'><rect width=\\'18\\' height=\\'18\\' x=\\'3\\' y=\\'3\\' rx=\\'2\\'/><circle cx=\\'9\\' cy=\\'9\\' r=\\'2\\'/><path d=\\'m21 15-3.086-3.086a2 2 0 0 0-2.828 0L6 21\\'/></svg>';"
                         style="cursor:pointer;"
                         title="${previewPath !== file.path ? 'Preview converted file' : 'Preview original'}" />
                </td>
                <td>
                    <span style="cursor:pointer;font-weight:600;" onclick="openPreview('${safePreviewAttrPath}')" title="${safePath}">
                        ${displayName}
                    </span>
                </td>
                <td><span class="badge badge-info">${file.ext}</span></td>
                <td class="conv-size-cell">${sizeHtml}</td>
                <td class="conv-status-cell">${statusHtml}</td>
                <td class="conv-actions-cell">${actionsHtml}</td>
            </tr>
        `;
    });

    html += `</tbody></table>`;
    tableDiv.innerHTML = html;

    const selectAll = document.getElementById('convSelectAll');
    if (selectAll) {
        selectAll.checked = visibleFiles.length > 0 && visibleFiles.every(f => selectedFiles.has(f.path));
    }
}

export function convToggleSelectAll(checked) {
    const visibleFiles = getVisibleFiles();
    visibleFiles.forEach(f => {
        if (checked) {
            selectedFiles.add(f.path);
        } else {
            selectedFiles.delete(f.path);
        }
    });
    document.querySelectorAll('.conv-file-cb').forEach(cb => { cb.checked = checked; });
    updateBulkBar();
}

export function convToggleSelectFile(path, checked) {
    if (checked) {
        selectedFiles.add(path);
    } else {
        selectedFiles.delete(path);
    }
    const visibleFiles = getVisibleFiles();
    const selectAll = document.getElementById('convSelectAll');
    if (selectAll) selectAll.checked = visibleFiles.length > 0 && visibleFiles.every(f => selectedFiles.has(f.path));
    updateBulkBar();
}

export function toggleFullPaths() {
    showFullPaths = !showFullPaths;
    renderTableRows();
}

function updateBulkBar() {
    const bulkBar = document.getElementById('convBulkBar');
    const bulkCount = document.getElementById('convBulkCount');
    if (!bulkBar || !bulkCount) return;

    const count = selectedFiles.size;
    bulkCount.textContent = count;
    bulkBar.style.display = count > 0 ? 'flex' : 'none';

    if (count > 0 && formatsData) {
        populateBatchDropdowns();
    }
}

function populateBatchDropdowns() {
    const videoDropdowns = document.getElementById('convVideoDropdowns');
    const imageDropdown = document.getElementById('convImageDropdown');
    const containerSel = document.getElementById('convBatchContainer');
    const codecSel = document.getElementById('convBatchCodec');
    const extSel = document.getElementById('convBatchExt');
    const imageTargetSel = document.getElementById('convBatchTarget');

    let hasVideo = false;
    let hasImage = false;

    selectedFiles.forEach(path => {
        const ext = '.' + path.split('.').pop().toLowerCase();
        if (['.mp4', '.mov', '.avi', '.mkv', '.webm', '.m4v'].includes(ext)) {
            hasVideo = true;
        } else {
            hasImage = true;
        }
    });

    if (videoDropdowns) videoDropdowns.style.display = hasVideo ? 'flex' : 'none';
    if (imageDropdown) imageDropdown.style.display = hasImage && !hasVideo ? 'flex' : 'none';

    if (hasVideo && containerSel && formatsData.video?.containers) {
        containerSel.innerHTML = formatsData.video.containers.map(c => `<option value="${c.name}">${c.name}</option>`).join('');
        convOnBatchContainerChange();
    }

    if (hasImage && imageTargetSel) {
        const imgExts = ['.webp', '.avif', '.jpg', '.png', '.gif', '.bmp', '.tif'];
        imageTargetSel.innerHTML = imgExts.map(e => `<option value="${e}">${e.toUpperCase().replace('.', '')}</option>`).join('');
    }
}

export function convOnBatchContainerChange() {
    const containerSel = document.getElementById('convBatchContainer');
    const codecSel = document.getElementById('convBatchCodec');
    const extSel = document.getElementById('convBatchExt');
    if (!containerSel || !codecSel || !extSel || !formatsData) return;

    const containerName = containerSel.value;
    const container = formatsData.video?.containers?.find(c => c.name === containerName);
    if (!container) return;

    codecSel.innerHTML = container.codecs.map(cd => `<option value="${cd}">${cd.toUpperCase()}</option>`).join('');
    extSel.innerHTML = container.extensions.map(ext => `<option value="${ext}">${ext}</option>`).join('');
}

export async function convConvertBatch() {
    if (selectedFiles.size === 0) return;

    conversionResults.clear();
    const batchSummary = document.getElementById('convBatchSummary');
    if (batchSummary) {
        batchSummary.style.display = 'none';
        batchSummary.innerHTML = '';
    }

    let targetExt = '';
    let codec = '';
    const hwaccel = document.getElementById('convHWAccel')?.value || 'auto';
    const deleteOriginal = document.getElementById('convDeleteOrig')?.checked || false;

    const codecSel = document.getElementById('convBatchCodec');
    const extSel = document.getElementById('convBatchExt');
    const imageTargetSel = document.getElementById('convBatchTarget');

    let hasVideo = false;
    selectedFiles.forEach(path => {
        const ext = '.' + path.split('.').pop().toLowerCase();
        if (['.mp4', '.mov', '.avi', '.mkv', '.webm', '.m4v'].includes(ext)) hasVideo = true;
    });

    if (hasVideo) {
        targetExt = extSel ? extSel.value : '.mp4';
        codec = codecSel ? codecSel.value : 'h264';
    } else {
        targetExt = imageTargetSel ? imageTargetSel.value : '.webp';
    }

    const progressCard = document.getElementById('convProgress');
    if (progressCard) progressCard.style.display = 'block';

    try {
        const res = await fetch('/api/converter/convert', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                files: Array.from(selectedFiles),
                target_ext: targetExt,
                codec: codec,
                hwaccel: hwaccel,
                delete_original: deleteOriginal
            })
        });

        if (!res.ok) {
            const errData = await res.json();
            throw new Error(errData.error?.message || 'Conversion start failed');
        }

        const data = await res.json();
        currentConvertJobId = data.job_id;
        showToast('Batch conversion running in background...', 'info');
    } catch (err) {
        showToast(err.message, 'error');
        if (progressCard) progressCard.style.display = 'none';
    }
}

export async function convStopConvert() {
    if (!currentConvertJobId) return;
    try {
        await fetch(`/api/converter/progress/${currentConvertJobId}/cancel`, { method: 'POST' });
        showToast('Cancelling conversion...', 'warning');
    } catch (err) {
        console.error(err);
    }
}

export async function convDeleteBatch() {
    if (selectedFiles.size === 0) return;
    if (!confirm(`Are you sure you want to move ${selectedFiles.size} selected file(s) to Safe Trash?`)) return;

    try {
        const res = await fetch('/api/converter/delete', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ files: Array.from(selectedFiles) })
        });

        if (!res.ok) throw new Error('Deletion failed');
        const data = await res.json();
        showToast(`Moved files to Safe Trash (Freed: ${data.total_freed_formatted})`, 'success');
        convScan();
    } catch (err) {
        showToast(err.message, 'error');
    }
}

function findRowByPath(tableDiv, filePath) {
    if (!tableDiv) return null;
    const rows = tableDiv.querySelectorAll('tr[data-path]');
    for (const r of rows) {
        if (r.getAttribute('data-path') === filePath) return r;
    }
    return null;
}

function updateConvertProgress(payload) {
    const bar = document.getElementById('convProgressBar');
    const pct = document.getElementById('convProgressPct');
    const msg = document.getElementById('convProgressMsg');

    const progress = Math.round((payload.progress || 0) * 100);
    if (bar) bar.style.width = `${progress}%`;
    if (pct) pct.textContent = `${progress}%`;
    if (msg) msg.textContent = payload.current_file ? `Converting: ${payload.current_file.split('/').pop()}` : 'Converting...';

    const tableDiv = document.getElementById('convFileTable');

    // Update table rows dynamically
    if (payload.results && Array.isArray(payload.results)) {
        payload.results.forEach(res => {
            conversionResults.set(res.source, res);

            const row = findRowByPath(tableDiv, res.source);
            if (row) {
                const statusCell = row.querySelector('.conv-status-cell');
                const sizeCell = row.querySelector('.conv-size-cell');
                const actionCell = row.querySelector('.conv-actions-cell');

                if (res.status === 'ok') {
                    const origBytes = res.original_size || 0;
                    const newBytes = res.new_size || 0;
                    const origFmt = res.original_size_fmt || formatFileSize(origBytes);
                    const newFmt = res.new_size_fmt || formatFileSize(newBytes);
                    let pctBadge = '';
                    if (origBytes > 0 && newBytes > 0) {
                        const diff = origBytes - newBytes;
                        const pctVal = Math.round((diff / origBytes) * 100);
                        if (diff > 0) {
                            pctBadge = `<span class="badge badge-success" style="font-size:11px;margin-left:4px;">-${pctVal}%</span>`;
                        } else if (diff < 0) {
                            pctBadge = `<span class="badge badge-warning" style="font-size:11px;margin-left:4px;">+${Math.abs(pctVal)}%</span>`;
                        } else {
                            pctBadge = `<span class="badge badge-ghost" style="font-size:11px;margin-left:4px;">0%</span>`;
                        }
                    }

                    if (statusCell) statusCell.innerHTML = `<span class="badge badge-success">✓ Done</span>`;
                    if (sizeCell) sizeCell.innerHTML = `<span>${origFmt} → <strong>${newFmt}</strong> ${pctBadge}</span>`;
                    if (actionCell) {
                        const safeDest = (res.destination || res.source).replace(/'/g, "\\'");
                        const safeSrc = res.source.replace(/'/g, "\\'");
                        actionCell.innerHTML = `
                            <button class="btn btn-ghost btn-sm" onclick="openPreview('${safeDest}')" title="Preview converted file">👁️ Converted</button>
                            ${res.destination ? `<button class="btn btn-ghost btn-sm" onclick="openCompareModal('${safeSrc}', '${safeDest}')" title="Compare original vs converted">🔍 Compare</button>` : ''}
                        `;
                    }

                    // Update thumbnail image and preview triggers to point to the newly converted file
                    if (res.destination) {
                        const safeDest = res.destination.replace(/'/g, "\\'");
                        const thumbImg = row.querySelector('img.trash-thumb');
                        if (thumbImg) {
                            thumbImg.src = `/api/thumbnail?path=${encodeURIComponent(res.destination)}`;
                            thumbImg.setAttribute('onclick', `openPreview('${safeDest}')`);
                            thumbImg.title = 'Preview converted file';
                        }
                    }
                } else if (res.status === 'error') {
                    if (statusCell) statusCell.innerHTML = `<span class="badge badge-danger">✕ Failed</span>`;
                    if (sizeCell) {
                        const origText = sizeCell.querySelector('span')?.textContent || sizeCell.textContent;
                        sizeCell.innerHTML = `
                            <span>${origText}</span>
                            <div style="font-size:11px;color:var(--danger);margin-top:2px;max-width:240px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;" title="${(res.error || 'Conversion error').replace(/"/g, '&quot;')}">
                                ${res.error || 'Conversion failed'}
                            </div>
                        `;
                    }
                }
            }
        });
    }
}

function finishConversion(payload) {
    const progressCard = document.getElementById('convProgress');
    if (progressCard) {
        setTimeout(() => { progressCard.style.display = 'none'; }, 1000);
    }
    updateConvertProgress(payload);

    // Calculate batch summary stats
    const allResults = Array.from(conversionResults.values());
    if (allResults.length > 0) {
        let okCount = 0;
        let errCount = 0;
        let totalOrigBytes = 0;
        let totalNewBytes = 0;

        allResults.forEach(r => {
            if (r.status === 'ok') {
                okCount++;
                totalOrigBytes += (r.original_size || 0);
                totalNewBytes += (r.new_size || 0);
            } else if (r.status === 'error') {
                errCount++;
            }
        });

        const batchSummary = document.getElementById('convBatchSummary');
        if (batchSummary) {
            batchSummary.style.display = 'block';
            const freedBytes = totalOrigBytes - totalNewBytes;
            const freedFmt = formatFileSize(Math.abs(freedBytes));
            const pct = totalOrigBytes > 0 ? Math.round((freedBytes / totalOrigBytes) * 100) : 0;
            const isSaved = freedBytes >= 0;

            batchSummary.innerHTML = `
                <div class="card" style="background:var(--surface2);border:1px solid var(--border);padding:14px 18px;border-radius:var(--radius-md);">
                    <div style="display:flex;align-items:center;justify-content:space-between;flex-wrap:wrap;gap:14px;">
                        <div style="display:flex;align-items:center;gap:12px;">
                            <span style="font-size:24px;">${errCount === 0 ? '🎉' : '⚠️'}</span>
                            <div>
                                <div style="font-weight:700;font-size:15px;color:var(--text-primary);">
                                    Conversion Complete
                                </div>
                                <div style="font-size:13px;color:var(--text-dim);margin-top:2px;">
                                    <span style="color:var(--accent-success);font-weight:600;">${okCount} succeeded</span>${errCount > 0 ? `, <span style="color:var(--danger);font-weight:600;">${errCount} failed</span>` : ''} of ${allResults.length} file(s)
                                </div>
                            </div>
                        </div>
                        <div style="display:flex;align-items:center;gap:18px;flex-wrap:wrap;">
                            <div style="text-align:right;">
                                <div style="font-size:11px;text-transform:uppercase;color:var(--text-dim);letter-spacing:0.5px;font-weight:600;">${isSaved ? 'Freed Space' : 'Size Change'}</div>
                                <div style="font-size:16px;font-weight:700;color:${isSaved ? 'var(--accent-success)' : 'var(--accent-warning)'};">
                                    ${isSaved ? '-' : '+'}${freedFmt}
                                    <span class="badge ${isSaved ? 'badge-success' : 'badge-warning'}" style="font-size:12px;vertical-align:middle;margin-left:4px;">
                                        ${isSaved ? '-' : '+'}${Math.abs(pct)}%
                                    </span>
                                </div>
                            </div>
                            <div style="border-left:1px solid var(--border);height:32px;"></div>
                            <div style="text-align:right;">
                                <div style="font-size:11px;text-transform:uppercase;color:var(--text-dim);letter-spacing:0.5px;font-weight:600;">Original → Converted</div>
                                <div style="font-size:13px;color:var(--text-secondary);font-weight:500;">
                                    ${formatFileSize(totalOrigBytes)} → ${formatFileSize(totalNewBytes)}
                                </div>
                            </div>
                        </div>
                    </div>
                </div>
            `;
        }
    }

    showToast('Batch conversion complete!', 'success');
}

function cancelConversionUI() {
    const progressCard = document.getElementById('convProgress');
    if (progressCard) progressCard.style.display = 'none';
    showToast('Conversion cancelled', 'warning');
}
