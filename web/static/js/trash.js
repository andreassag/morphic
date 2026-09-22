import { showToast, formatFileSize, openPreview } from './ui.js';
import { onEvent } from './events.js';

export function initTrash() {
    loadTrashHistory();

    onEvent('trash', () => {
        loadTrashHistory();
    });
}

export async function loadTrashHistory() {
    const tbody = document.getElementById('trashTableBody');
    if (!tbody) return;

    try {
        const url = `/api/trash?limit=100`;
        const res = await fetch(url);
        if (!res.ok) throw new Error('Failed to load safe trash');

        const data = await res.json();
        const entries = data.entries || [];

        if (entries.length === 0) {
            tbody.innerHTML = `
                <tr>
                    <td colspan="7" style="text-align:center;padding:32px;color:var(--text-dim);">No files in safe trash.</td>
                </tr>
            `;
            return;
        }

        let html = '';
        entries.forEach(item => {
            const timeStr = item.created_at ? new Date(item.created_at).toLocaleString() : '-';
            const isReversed = !!item.reversed_at;
            const fullPath = item.original_path || item.trash_path || 'Unknown';
            const filename = fullPath.split('/').pop() || fullPath;

            // In safe-trash, preview path is trash_path if not reversed, or original_path if restored
            const previewPath = !isReversed ? (item.trash_path || item.original_path || '') : (item.original_path || item.trash_path || '');
            const safePreviewPath = previewPath.replace(/\\/g, '\\\\').replace(/'/g, "\\'");

            // Source badge
            const rawSource = (item.source || item.metadata?.source || 'dupfinder').toLowerCase();
            let sourceBadge = '';
            if (rawSource.includes('dupfinder')) {
                sourceBadge = '<span class="badge badge-primary">Dupfinder</span>';
            } else if (rawSource.includes('convert')) {
                sourceBadge = '<span class="badge badge-warning">Converter</span>';
            } else if (rawSource.includes('organize') || rawSource.includes('sort')) {
                sourceBadge = '<span class="badge badge-info">Organizer</span>';
            } else {
                sourceBadge = `<span class="badge badge-secondary">${rawSource}</span>`;
            }

            // Preview thumbnail cell
            let previewCell = '<span style="color:var(--text-dim);">-</span>';
            if (previewPath && previewPath !== 'Unknown') {
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

            // Actions cell
            let actionsHtml = `<div style="display:flex;gap:6px;justify-content:flex-end;align-items:center;">`;
            if (previewPath && previewPath !== 'Unknown') {
                actionsHtml += `<button class="btn btn-ghost btn-sm" onclick="openPreview('${safePreviewPath}')" title="Inspect preview">👁️ View</button>`;
            }
            if (!isReversed) {
                actionsHtml += `<button class="btn btn-primary btn-sm" onclick="undoAuditAction(${item.id})" title="Restore to original location">↩️ Restore</button>`;
            }
            actionsHtml += `</div>`;

            html += `
                <tr>
                    <td>${previewCell}</td>
                    <td style="white-space:nowrap;font-size:12px;color:var(--text-dim);">${timeStr}</td>
                    <td>${sourceBadge}</td>
                    <td title="${fullPath}" style="max-width:300px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">${filename}</td>
                    <td>${item.file_size ? formatFileSize(item.file_size) : '-'}</td>
                    <td>
                        ${isReversed 
                            ? `<span class="badge badge-success">Restored</span>` 
                            : `<span class="badge badge-danger">In Safe-Trash</span>`}
                    </td>
                    <td style="text-align:right;">${actionsHtml}</td>
                </tr>
            `;
        });

        tbody.innerHTML = html;
    } catch (err) {
        showToast(err.message, 'error');
    }
}

export async function undoAuditAction(id) {
    try {
        const res = await fetch(`/api/history/${id}/undo`, { method: 'POST' });
        if (!res.ok) {
            const errData = await res.json();
            throw new Error(errData.error?.message || 'Restore failed');
        }

        showToast('File successfully restored to original location!', 'success');
        loadTrashHistory();
    } catch (err) {
        showToast(err.message, 'error');
    }
}

export async function purgeExpiredTrash() {
    if (!confirm('Purge all safe-trash items older than 30 days? This action cannot be undone.')) return;

    try {
        const res = await fetch('/api/trash/purge?days=30', { method: 'POST' });
        if (!res.ok) throw new Error('Purge failed');

        const data = await res.json();
        showToast(`Purged ${data.purged_count} items (Freed: ${data.freed_formatted})`, 'success');
        loadTrashHistory();
    } catch (err) {
        showToast(err.message, 'error');
    }
}
