// ── Safe Trash & Audit History Module ────────────────────────────────

import { showToast, formatFileSize } from './ui.js';
import { onEvent } from './events.js';

export function initTrash() {
    loadTrashHistory();

    onEvent('trash', () => {
        loadTrashHistory();
    });
}

export async function loadTrashHistory() {
    const filterOp = document.getElementById('trashFilterOp')?.value || '';
    const tbody = document.getElementById('trashTableBody');
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
                    <td colspan="6" style="text-align:center;padding:32px;color:var(--text-dim);">No history entries found.</td>
                </tr>
            `;
            return;
        }

        let html = '';
        entries.forEach(item => {
            const timeStr = item.created_at ? new Date(item.created_at).toLocaleString() : '-';
            const badgeClass = item.operation === 'delete' ? 'badge-danger' : 'badge-info';
            const isReversed = !!item.reversed_at;
            const fullPath = item.original_path || item.destination_path || item.trash_path || 'Unknown';
            const filename = fullPath.split('/').pop() || fullPath;

            html += `
                <tr>
                    <td style="white-space:nowrap;font-size:12px;color:var(--text-dim);">${timeStr}</td>
                    <td><span class="badge ${badgeClass}">${item.operation}</span></td>
                    <td title="${fullPath}">${filename}</td>
                    <td>${item.file_size ? formatFileSize(item.file_size) : '-'}</td>
                    <td>
                        ${isReversed 
                            ? `<span class="badge badge-success">Restored</span>` 
                            : item.operation === 'delete' 
                                ? `<span class="badge badge-danger">In Safe-Trash</span>` 
                                : `<span class="badge badge-info">Completed</span>`}
                    </td>
                    <td>
                        ${item.operation === 'delete' && !isReversed ? `
                            <button class="btn btn-primary btn-sm" onclick="undoAuditAction(${item.id})">↩️ Restore</button>
                        ` : '-'}
                    </td>
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
