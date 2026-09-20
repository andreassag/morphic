// ── Audit History Module ──────────────────────────────────────────────

import { showToast, formatFileSize, openPreview } from './ui.js';
import { onEvent } from './events.js';

export function initHistory() {
    loadAuditHistory();

    onEvent('history', () => {
        loadAuditHistory();
    });
    onEvent('trash', () => {
        loadAuditHistory();
    });
    onEvent('converter', (e) => {
        if (e.type === 'job_done') loadAuditHistory();
    });
    onEvent('organizer', (e) => {
        if (e.type === 'job_done') loadAuditHistory();
    });
}

export async function loadAuditHistory() {
    const filterOp = document.getElementById('historyFilterOp')?.value || '';
    const tbody = document.getElementById('historyTableBody');
    if (!tbody) return;

    try {
        const url = `/api/history?limit=100${filterOp ? `&operation=${encodeURIComponent(filterOp)}` : ''}`;
        const res = await fetch(url);
        if (!res.ok) throw new Error('Failed to load audit history');

        const data = await res.json();
        const entries = data.entries || [];

        if (entries.length === 0) {
            tbody.innerHTML = `
                <tr>
                    <td colspan="9" style="text-align:center;padding:32px;color:var(--text-dim);">No audit entries found.</td>
                </tr>
            `;
            return;
        }

        let html = '';
        entries.forEach(item => {
            const timeStr = item.created_at ? new Date(item.created_at).toLocaleString() : '-';
            const isReversed = !!item.reversed_at;
            const origPath = item.original_path || '-';
            const destPath = item.destination_path || item.trash_path || '-';
            const origFilename = origPath.split('/').pop() || origPath;
            const destFilename = destPath.split('/').pop() || destPath;

            // Determine preview path
            const previewPath = (item.operation === 'delete' && !isReversed)
                ? (item.trash_path || item.original_path || '')
                : (item.destination_path || item.original_path || item.trash_path || '');
            const safePreviewPath = previewPath.replace(/\\/g, '\\\\').replace(/'/g, "\\'");

            // Source badge
            const rawSource = (item.source || (item.operation === 'delete' ? 'dupfinder' : (item.operation === 'convert' ? 'converter' : 'organizer'))).toLowerCase();
            let sourceBadge = '';
            if (rawSource.includes('dupfinder')) {
                sourceBadge = '<span class="badge badge-primary">Dupfinder</span>';
            } else if (rawSource.includes('convert')) {
                sourceBadge = '<span class="badge badge-warning">Converter</span>';
            } else if (rawSource.includes('organize') || rawSource.includes('sort') || rawSource.includes('rename')) {
                sourceBadge = '<span class="badge badge-info">Organizer</span>';
            } else {
                sourceBadge = `<span class="badge badge-secondary">${rawSource}</span>`;
            }

            // Operation badge
            let opBadgeClass = 'badge-info';
            if (item.operation === 'delete') opBadgeClass = 'badge-danger';
            else if (item.operation === 'convert') opBadgeClass = 'badge-warning';
            else if (item.operation === 'sort' || item.operation === 'rename') opBadgeClass = 'badge-success';

            // Preview cell
            let previewCell = '<span style="color:var(--text-dim);">-</span>';
            if (previewPath && previewPath !== 'Unknown' && previewPath !== '-') {
                const thumbUrl = `/api/thumbnail?path=${encodeURIComponent(previewPath)}`;
                previewCell = `
                    <div style="display:flex;align-items:center;justify-content:center;width:44px;height:44px;">
                        <img class="trash-thumb" src="${thumbUrl}" alt="Thumb" loading="lazy"
                             onclick="openPreview('${safePreviewPath}')"
                             onerror="this.style.display='none';this.nextElementSibling.style.display='flex';" />
                        <div style="display:none;width:44px;height:44px;border-radius:var(--radius-sm);background:var(--surface3);align-items:center;justify-content:center;font-size:18px;cursor:pointer;border:1px solid var(--border);"
                             onclick="openPreview('${safePreviewPath}')" title="Inspect">🖼️</div>
                    </div>
                `;
            }

            // Status badge
            let statusBadge = '<span class="badge badge-info">Completed</span>';
            if (item.operation === 'delete') {
                statusBadge = isReversed 
                    ? '<span class="badge badge-success">Restored</span>' 
                    : '<span class="badge badge-danger">In Safe-Trash</span>';
            }

            // Actions cell
            let actionsHtml = `<div style="display:flex;gap:6px;justify-content:flex-end;align-items:center;">`;
            if (previewPath && previewPath !== 'Unknown' && previewPath !== '-') {
                actionsHtml += `<button class="btn btn-ghost btn-sm" onclick="openPreview('${safePreviewPath}')" title="Inspect preview">👁️ View</button>`;
            }
            if (item.operation === 'delete' && !isReversed) {
                actionsHtml += `<button class="btn btn-primary btn-sm" onclick="undoHistoryAction(${item.id})" title="Restore to original location">↩️ Restore</button>`;
            }
            actionsHtml += `</div>`;

            html += `
                <tr>
                    <td>${previewCell}</td>
                    <td style="white-space:nowrap;font-size:12px;color:var(--text-dim);">${timeStr}</td>
                    <td>${sourceBadge}</td>
                    <td><span class="badge ${opBadgeClass}">${item.operation}</span></td>
                    <td title="${origPath}" style="max-width:200px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">${origFilename}</td>
                    <td title="${destPath}" style="max-width:200px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:var(--text-dim);">${destFilename}</td>
                    <td>${item.file_size ? formatFileSize(item.file_size) : '-'}</td>
                    <td>${statusBadge}</td>
                    <td style="text-align:right;">${actionsHtml}</td>
                </tr>
            `;
        });

        tbody.innerHTML = html;
    } catch (err) {
        showToast(err.message, 'error');
    }
}

export async function undoHistoryAction(id) {
    try {
        const res = await fetch(`/api/history/${id}/undo`, { method: 'POST' });
        if (!res.ok) {
            const errData = await res.json();
            throw new Error(errData.error?.message || 'Restore failed');
        }

        showToast('File successfully restored to original location!', 'success');
        loadAuditHistory();
        if (typeof window.loadTrashHistory === 'function') {
            window.loadTrashHistory();
        }
    } catch (err) {
        showToast(err.message, 'error');
    }
}
