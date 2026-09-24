// ── DupFinder Module ─────────────────────────────────────────────────

import { showToast, formatFileSize, openPreview } from './ui.js';
import { onEvent } from './events.js';
import { openCompareModal } from './compare.js';

let currentJobId = null;
let currentResults = null;
let selectedDupFiles = new Set();
let dupTimerInterval = null;
let dupStartTimeMs = null;

function startDupTimer() {
    stopDupTimer();
    dupStartTimeMs = Date.now();
    const elapsed = document.getElementById('dupProgressElapsed');
    if (elapsed) elapsed.textContent = '0.0s';
    dupTimerInterval = setInterval(() => {
        if (!dupStartTimeMs) return;
        const elapsedSec = (Date.now() - dupStartTimeMs) / 1000;
        const el = document.getElementById('dupProgressElapsed');
        if (el) el.textContent = `${elapsedSec.toFixed(1)}s`;
    }, 100);
}

function stopDupTimer() {
    if (dupTimerInterval) {
        clearInterval(dupTimerInterval);
        dupTimerInterval = null;
    }
}

export function initDupfinder() {
    onEvent('dupfinder', (event) => {
        const payload = event.payload || event.data || event;
        if (event.type === 'progress') {
            updateDupProgress(payload);
        } else if (event.type === 'job_done') {
            finishDupScan(payload);
        } else if (event.type === 'job_cancelled') {
            cancelDupUI();
        }
    });
}

export async function dupStartScan() {
    const folder = document.getElementById('dupFolder')?.value.trim();
    if (!folder) {
        showToast('Please enter a folder path to scan', 'warning');
        return;
    }

    const type = document.getElementById('dupScanType')?.value || 'both';
    const imgThresh = parseFloat(document.getElementById('dupImgThreshold')?.value || '90') / 100;
    const vidThresh = parseFloat(document.getElementById('dupVidThreshold')?.value || '85') / 100;

    const progressCard = document.getElementById('dupProgress');
    const resultsSection = document.getElementById('dupResults');
    const scanBtn = document.getElementById('dupScanBtn');

    if (progressCard) progressCard.style.display = 'block';
    if (resultsSection) resultsSection.style.display = 'none';
    if (scanBtn) scanBtn.disabled = true;

    startDupTimer();

    try {
        const res = await fetch('/api/dupfinder/scan', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                folder: folder,
                type: type,
                image_threshold: imgThresh,
                video_threshold: vidThresh
            })
        });

        if (!res.ok) {
            const errData = await res.json();
            throw new Error(errData.error?.message || 'Failed to start dupfinder scan');
        }

        const data = await res.json();
        currentJobId = data.job_id;
        showToast('Dupfinder scan started...', 'info');
    } catch (err) {
        stopDupTimer();
        showToast(err.message, 'error');
        if (progressCard) progressCard.style.display = 'none';
        if (scanBtn) scanBtn.disabled = false;
    }
}

export async function dupCancelScan() {
    stopDupTimer();
    if (!currentJobId) return;
    try {
        await fetch(`/api/dupfinder/scan/${currentJobId}/cancel`, { method: 'POST' });
        showToast('Cancelling scan...', 'warning');
    } catch (err) {
        console.error(err);
    }
}

export async function dupApplyRule(rule) {
    if (!currentJobId || !rule) return;

    try {
        const res = await fetch('/api/dupfinder/auto-select', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ job_id: currentJobId, rule: rule })
        });

        if (!res.ok) throw new Error('Failed to auto-select rule');
        const data = await res.json();

        selectedDupFiles.clear();
        (data.selected_files || []).forEach(p => selectedDupFiles.add(p));

        document.querySelectorAll('.dup-file-cb').forEach(cb => {
            cb.checked = selectedDupFiles.has(cb.dataset.path);
        });

        updateDupBulkBar();
        showToast(`Auto-selected ${data.total_selected} files (Potential savings: ${data.freed_formatted})`, 'info');
    } catch (err) {
        showToast(err.message, 'error');
    }
}

export function dupClearSelection() {
    selectedDupFiles.clear();
    document.querySelectorAll('.dup-file-cb').forEach(cb => { cb.checked = false; });
    updateDupBulkBar();
}

export function dupExpandAll() {
    document.querySelectorAll('.dup-group-body').forEach(el => { el.style.display = 'block'; });
}

export function dupCollapseAll() {
    document.querySelectorAll('.dup-group-body').forEach(el => { el.style.display = 'none'; });
}

export function dupToggleSelect(path, checked) {
    if (checked) {
        selectedDupFiles.add(path);
    } else {
        selectedDupFiles.delete(path);
    }
    updateDupBulkBar();
}

function updateDupBulkBar() {
    const bulkBar = document.getElementById('dupBulkBar');
    const bulkCount = document.getElementById('dupBulkCount');
    const bulkSize = document.getElementById('dupBulkSize');
    if (!bulkBar || !bulkCount) return;

    const count = selectedDupFiles.size;
    bulkCount.textContent = count;

    let sizeSum = 0;
    if (currentResults) {
        const allGroups = [...(currentResults.image_groups || []), ...(currentResults.video_groups || [])];
        allGroups.forEach(grp => {
            const items = grp.items || grp;
            items.forEach(item => {
                if (selectedDupFiles.has(item.path)) {
                    sizeSum += item.file_size;
                }
            });
        });
    }
    if (bulkSize) bulkSize.textContent = formatFileSize(sizeSum);

    bulkBar.style.display = count > 0 ? 'flex' : 'none';
}

export function dupConfirmDelete() {
    if (selectedDupFiles.size === 0) return;
    const modal = document.getElementById('dupDeleteModal');
    const list = document.getElementById('dupDeleteFileList');
    if (!modal || !list) return;

    list.innerHTML = Array.from(selectedDupFiles).map(p => `<div>${p}</div>`).join('');
    modal.classList.add('active');
}

export function dupCloseModal() {
    const modal = document.getElementById('dupDeleteModal');
    if (modal) modal.classList.remove('active');
}

export async function dupExecuteDelete() {
    if (selectedDupFiles.size === 0) return;

    const filesToDelete = Array.from(selectedDupFiles);
    try {
        const res = await fetch('/api/dupfinder/delete', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ files: filesToDelete })
        });

        if (!res.ok) {
            const errData = await res.json().catch(() => ({}));
            throw new Error(errData.error?.message || 'Deletion failed');
        }

        const data = await res.json();
        dupCloseModal();

        const deleted = (data.results || []).filter(r => r.status === 'deleted').map(r => r.path);
        if (deleted.length > 0) {
            removeDeletedFromView(deleted);
            showToast(`Moved ${deleted.length} duplicate(s) to Safe Trash (Freed: ${data.total_freed_formatted || '0 B'})`, 'success');
        }

        const failed = (data.results || []).filter(r => r.status !== 'deleted');
        if (failed.length > 0) {
            const sampleReason = failed.find(r => r.error)?.error ||
                (failed.some(r => r.status === 'permission_denied') ? 'permission denied: check folder write permissions' : '');
            const msg = sampleReason
                ? `${failed.length} file(s) could not be deleted (${sampleReason})`
                : `${failed.length} file(s) could not be deleted`;
            showToast(msg, 'warning');
        }
    } catch (err) {
        showToast(err.message, 'error');
    }
}

export async function dupDeleteSingle(path) {
    if (!path) return;
    const filename = path.split('/').pop() || path;
    if (!confirm(`Move "${filename}" to Safe Trash?`)) return;

    try {
        const res = await fetch('/api/dupfinder/delete', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ files: [path] })
        });

        if (!res.ok) {
            const errData = await res.json().catch(() => ({}));
            throw new Error(errData.error?.message || 'Deletion failed');
        }

        const data = await res.json();
        const deleted = (data.results || []).filter(r => r.status === 'deleted').map(r => r.path);
        if (deleted.length > 0) {
            removeDeletedFromView(deleted);
            showToast(`Moved "${filename}" to Safe Trash (Freed: ${data.total_freed_formatted || '0 B'})`, 'success');
        } else {
            const firstFail = (data.results || []).find(r => r.status !== 'deleted');
            const errMsg = firstFail?.error ||
                (firstFail?.status === 'permission_denied' ? 'Permission denied (check folder write permissions)' : 'File could not be deleted');
            showToast(errMsg, 'error');
        }
    } catch (err) {
        showToast(err.message, 'error');
    }
}

function removeDeletedFromView(deletedPaths) {
    if (!deletedPaths || deletedPaths.length === 0 || !currentResults) return;
    const deletedSet = new Set(deletedPaths);

    deletedPaths.forEach(p => selectedDupFiles.delete(p));

    const filterGroupList = (groupList, type) => {
        const remainingGroups = [];
        groupList.forEach(grp => {
            const items = grp.items || grp;
            const remainingItems = items.filter(item => !deletedSet.has(item.path));

            if (remainingItems.length < 2) {
                // Duplicate group resolved: remove entire card from view
                items.forEach(item => {
                    const row = Array.from(document.querySelectorAll('.dup-item-row')).find(el => el.dataset.path === item.path);
                    if (row) {
                        const card = row.closest('.dup-group-card');
                        if (card && card.parentNode) {
                            card.parentNode.removeChild(card);
                        }
                    }
                });
            } else {
                grp.items = remainingItems;
                remainingGroups.push(grp);

                // Remove deleted row(s) from DOM
                deletedPaths.forEach(p => {
                    const row = Array.from(document.querySelectorAll('.dup-item-row')).find(el => el.dataset.path === p);
                    if (row && row.parentNode) {
                        row.parentNode.removeChild(row);
                    }
                });

                // Update group header file count
                const firstRow = Array.from(document.querySelectorAll('.dup-item-row')).find(el => el.dataset.path === remainingItems[0]?.path);
                if (firstRow) {
                    const card = firstRow.closest('.dup-group-card');
                    const headerSpan = card?.querySelector('.dup-group-header span');
                    if (headerSpan) {
                        headerSpan.textContent = `Group (${remainingItems.length} ${type} files)`;
                    }
                }
            }
        });
        return remainingGroups;
    };

    if (currentResults.image_groups) {
        currentResults.image_groups = filterGroupList(currentResults.image_groups, 'image');
    }
    if (currentResults.video_groups) {
        currentResults.video_groups = filterGroupList(currentResults.video_groups, 'video');
    }

    // Recalculate potential space savings
    let totalSavings = 0;
    const allGroups = [...(currentResults.image_groups || []), ...(currentResults.video_groups || [])];
    allGroups.forEach(grp => {
        const items = grp.items || grp;
        if (items.length > 1) {
            let maxSize = 0;
            let groupSum = 0;
            items.forEach(item => {
                const s = item.file_size || 0;
                groupSum += s;
                if (s > maxSize) maxSize = s;
            });
            totalSavings += (groupSum - maxSize);
        }
    });
    currentResults.space_savings = totalSavings;

    const totalGroups = (currentResults.image_groups?.length || 0) + (currentResults.video_groups?.length || 0);

    const title = document.getElementById('dupTitle');
    const summary = document.getElementById('dupSummary');
    const noResultsDiv = document.getElementById('dupNoResults');

    if (totalGroups === 0) {
        if (noResultsDiv) noResultsDiv.style.display = 'block';
        if (title) title.textContent = 'Found 0 Duplicate Groups';
        if (summary) summary.innerHTML = '';
    } else {
        if (noResultsDiv) noResultsDiv.style.display = 'none';
        if (title) title.textContent = `Found ${totalGroups} Duplicate Group(s)`;
        if (summary) {
            summary.innerHTML = `
                <div style="font-size:13px;color:var(--text-dim);margin-bottom:12px;">
                    Total Potential Space Savings: <strong style="color:var(--success);">${formatFileSize(totalSavings)}</strong>
                </div>
            `;
        }
    }

    updateDupBulkBar();
}

function updateDupProgress(payload) {
    if (!payload) return;
    const bar = document.getElementById('dupProgressBar');
    const pct = document.getElementById('dupProgressPct');
    const msg = document.getElementById('dupProgressMsg');

    const progress = Math.min(100, Math.max(0, Math.round((payload.progress || 0) * 100)));
    if (bar) bar.style.width = `${progress}%`;
    if (pct) pct.textContent = `${progress}%`;

    const total = payload.total_files_found || 0;
    const processed = payload.total_files_processed || 0;
    let text = payload.message || 'Scanning...';
    if (total > 0 && processed > 0 && !text.includes('Done') && !text.includes('(')) {
        text = `${text} (${processed} / ${total} files)`;
    }
    if (msg) msg.textContent = text;

    // Sync client timer base timestamp with server elapsed time to prevent clock drift
    if (payload.elapsed_seconds != null && typeof payload.elapsed_seconds === 'number') {
        dupStartTimeMs = Date.now() - (payload.elapsed_seconds * 1000);
    }
}

function finishDupScan(payload) {
    stopDupTimer();
    const progressCard = document.getElementById('dupProgress');
    const resultsSection = document.getElementById('dupResults');
    const scanBtn = document.getElementById('dupScanBtn');
    const elapsed = document.getElementById('dupProgressElapsed');

    if (elapsed && payload?.elapsed_seconds) {
        elapsed.textContent = `${payload.elapsed_seconds}s`;
    }

    if (progressCard) progressCard.style.display = 'none';
    if (scanBtn) scanBtn.disabled = false;

    if (!payload) return;
    currentResults = payload;
    renderDupResults(payload);
    if (resultsSection) resultsSection.style.display = 'block';
}

function cancelDupUI() {
    stopDupTimer();
    const progressCard = document.getElementById('dupProgress');
    const scanBtn = document.getElementById('dupScanBtn');
    if (progressCard) progressCard.style.display = 'none';
    if (scanBtn) scanBtn.disabled = false;
    showToast('Scan cancelled', 'warning');
}

function renderDupResults(data) {
    const groupsDiv = document.getElementById('dupGroups');
    const noResultsDiv = document.getElementById('dupNoResults');
    const title = document.getElementById('dupTitle');
    const summary = document.getElementById('dupSummary');
    if (!groupsDiv) return;

    data = data || {};

    const imgGroups = data.image_groups || [];
    const vidGroups = data.video_groups || [];
    const totalGroups = imgGroups.length + vidGroups.length;

    if (totalGroups === 0) {
        if (noResultsDiv) noResultsDiv.style.display = 'block';
        groupsDiv.innerHTML = '';
        if (summary) summary.innerHTML = '';
        return;
    }

    if (noResultsDiv) noResultsDiv.style.display = 'none';
    if (title) title.textContent = `Found ${totalGroups} Duplicate Group(s)`;
    if (summary) {
        summary.innerHTML = `
            <div style="font-size:13px;color:var(--text-dim);margin-bottom:12px;">
                Total Potential Space Savings: <strong style="color:var(--success);">${formatFileSize(data.space_savings)}</strong>
            </div>
        `;
    }

    let html = '';
    const renderGroup = (group, grpIdx, type) => {
        const items = group.items || group;
        const groupSimilarity = group.group_similarity != null ? group.group_similarity : null;

        html += `
            <div class="dup-group-card" id="dupGroup_${grpIdx}">
                <div class="dup-group-header">
                    <div style="display:flex;align-items:center;gap:10px;">
                        <span>Group #${grpIdx + 1} (${items.length} ${type} files)</span>
                        ${groupSimilarity != null ? `<span class="badge badge-accent" style="font-size:11px;font-weight:600;">${groupSimilarity}% match</span>` : ''}
                    </div>
                    <div style="display:flex;gap:8px;">
                        ${items.length >= 2 ? `
                            <button class="btn btn-ghost btn-sm" onclick="openCompareModal('${items[0].path.replace(/'/g, "\\'")}', '${items[1].path.replace(/'/g, "\\'")}')">🔍 Compare #1 vs #2</button>
                        ` : ''}
                    </div>
                </div>
                <div class="dup-group-body">
        `;

        items.forEach((item, itemIdx) => {
            const isChecked = selectedDupFiles.has(item.path) ? 'checked' : '';
            const thumbUrl = `/api/thumbnail?path=${encodeURIComponent(item.path)}`;
            const safePath = item.path.replace(/'/g, "\\'");

            html += `
                <div class="dup-item-row" data-path="${item.path}">
                    <input type="checkbox" class="dup-file-cb" data-path="${item.path}" ${isChecked} onchange="dupToggleSelect('${safePath}', this.checked)" />
                    <img class="dup-thumb" src="${thumbUrl}" alt="Thumbnail" onclick="openPreview('${safePath}')" />
                    <div class="dup-info">
                        <div class="dup-filename" title="${item.path}">${item.filename}</div>
                        <div class="dup-meta">
                            <span>📁 ${item.directory}</span>
                            <span>📏 ${item.resolution || item.file_size_formatted}</span>
                            <span>⚖️ ${item.file_size_formatted}</span>
                        </div>
                    </div>
                    <div style="display:flex;gap:6px;align-items:center;">
                        <button class="btn btn-ghost btn-sm" onclick="openPreview('${safePath}')">👁️ View</button>
                        <button class="btn btn-danger btn-sm" onclick="dupDeleteSingle('${safePath}')" title="Move this duplicate to Safe Trash">🗑️ Trash</button>
                    </div>
                </div>
            `;
        });

        html += `</div></div>`;
    };

    let globalIdx = 0;
    imgGroups.forEach(grp => { renderGroup(grp, globalIdx++, 'image'); });
    vidGroups.forEach(grp => { renderGroup(grp, globalIdx++, 'video'); });

    groupsDiv.innerHTML = html;
}
