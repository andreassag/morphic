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
                    <td colspan="7" style="text-align:center;padding:32px;color:var(--text-dim);">No audit entries found.</td>
                </tr>
            `;
            return;
        }

        let html = '';
        entries.forEach(item => {
            const timeStr = item.created_at ? new Date(item.created_at).toLocaleString() : '-';
            const op = item.operation || 'unknown';
            const source = item.source || '-';
            const summary = item.summary || `${op} operation`;
            const itemCount = typeof item.item_count === 'number' && item.item_count >= 0 ? item.item_count : '-';
            
            // Format size / impact
            let sizeImpact = '-';
            if (item.total_size && item.total_size > 0) {
                sizeImpact = formatFileSize(item.total_size);
            } else if (item.metadata && typeof item.metadata === 'object') {
                if (item.metadata.freed_bytes) {
                    sizeImpact = formatFileSize(item.metadata.freed_bytes);
                } else if (item.metadata.target_format) {
                    sizeImpact = `Target: ${item.metadata.target_format.toUpperCase()}`;
                }
            }

            // Operation badge styling
            let opBadgeClass = 'badge-info';
            if (op === 'delete' || op === 'purge') opBadgeClass = 'badge-danger';
            else if (op === 'convert') opBadgeClass = 'badge-warning';
            else if (op === 'sort' || op === 'rename' || op === 'restore') opBadgeClass = 'badge-success';

            // Source badge styling
            let sourceBadge = `<span class="badge badge-secondary">${source}</span>`;
            const lowerSource = source.toLowerCase();
            if (lowerSource.includes('dupfinder')) {
                sourceBadge = '<span class="badge badge-primary">Dupfinder</span>';
            } else if (lowerSource.includes('convert')) {
                sourceBadge = '<span class="badge badge-warning">Converter</span>';
            } else if (lowerSource.includes('organize') || lowerSource.includes('sort') || lowerSource.includes('rename')) {
                sourceBadge = '<span class="badge badge-info">Organizer</span>';
            } else if (lowerSource.includes('trash')) {
                sourceBadge = '<span class="badge badge-danger">Safe-Trash</span>';
            }

            // Status badge styling
            let statusBadge = '<span class="badge badge-info">Completed</span>';
            const lowerStatus = (item.status || 'completed').toLowerCase();
            if (lowerStatus === 'failed' || lowerStatus === 'error') {
                statusBadge = '<span class="badge badge-danger">Failed</span>';
            } else if (lowerStatus === 'partial') {
                statusBadge = '<span class="badge badge-warning">Partial</span>';
            } else if (lowerStatus === 'completed' || lowerStatus === 'success') {
                statusBadge = '<span class="badge badge-success">Completed</span>';
            }

            html += `
                <tr>
                    <td style="white-space:nowrap;font-size:12px;color:var(--text-dim);">${timeStr}</td>
                    <td><span class="badge ${opBadgeClass}">${op}</span></td>
                    <td>${sourceBadge}</td>
                    <td title="${summary}" style="max-width:320px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">${summary}</td>
                    <td style="font-weight:600;">${itemCount}</td>
                    <td style="color:var(--text-dim);">${sizeImpact}</td>
                    <td>${statusBadge}</td>
                </tr>
            `;
        });

        tbody.innerHTML = html;
    } catch (err) {
        showToast(err.message, 'error');
    }
}
