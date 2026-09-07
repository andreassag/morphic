// ── Settings & Watch Folders Module ──────────────────────────────────

import { showToast } from './ui.js';
import { onEvent } from './events.js';

export function initSettings() {
    loadWatchFolders();
    loadSystemInfo();

    onEvent('watcher', () => {
        loadWatchFolders();
    });
}

export async function loadWatchFolders() {
    const tbody = document.getElementById('watchersTableBody');
    if (!tbody) return;

    try {
        const res = await fetch('/api/watch');
        if (!res.ok) throw new Error('Failed to load watch folders');

        const folders = await res.json();
        if (!folders || folders.length === 0) {
            tbody.innerHTML = `
                <tr>
                    <td colspan="5" style="text-align:center;padding:24px;color:var(--text-dim);">No active watch folders.</td>
                </tr>
            `;
            return;
        }

        let html = '';
        folders.forEach(item => {
            const timeStr = new Date(item.created_at).toLocaleDateString();
            html += `
                <tr>
                    <td><code>${item.path}</code></td>
                    <td><span class="badge badge-info">${item.action}</span></td>
                    <td><span class="badge ${item.enabled ? 'badge-success' : 'badge-danger'}">${item.enabled ? 'Active' : 'Paused'}</span></td>
                    <td>${timeStr}</td>
                    <td>
                        <button class="btn btn-danger btn-sm" onclick="deleteWatchFolder(${item.id})">🗑️ Remove</button>
                    </td>
                </tr>
            `;
        });
        tbody.innerHTML = html;
    } catch (err) {
        showToast(err.message, 'error');
    }
}

export async function addWatchFolder() {
    const path = document.getElementById('watchNewPath')?.value.trim();
    const action = document.getElementById('watchNewAction')?.value || 'organize';

    if (!path) {
        showToast('Please enter an absolute directory path to watch', 'warning');
        return;
    }

    try {
        const res = await fetch('/api/watch', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                path: path,
                action: action,
                config: {}
            })
        });

        if (!res.ok) {
            const errData = await res.json();
            throw new Error(errData.error?.message || 'Failed to add watch folder');
        }

        showToast('Watch folder added and monitoring in daemon background!', 'success');
        document.getElementById('watchNewPath').value = '';
        loadWatchFolders();
    } catch (err) {
        showToast(err.message, 'error');
    }
}

export async function deleteWatchFolder(id) {
    if (!confirm('Remove this directory from background monitoring?')) return;

    try {
        const res = await fetch(`/api/watch/${id}`, { method: 'DELETE' });
        if (!res.ok) throw new Error('Failed to delete watch folder');

        showToast('Watch folder removed', 'info');
        loadWatchFolders();
    } catch (err) {
        showToast(err.message, 'error');
    }
}

async function loadSystemInfo() {
    const div = document.getElementById('systemInfoDetails');
    if (!div) return;

    try {
        const res = await fetch('/api/system_info');
        if (!res.ok) return;

        const info = await res.json();
        const profiles = info.ffmpeg?.profiles || [];

        let profilesHtml = '';
        if (profiles.length > 0) {
            profilesHtml = `
                <div style="margin-top:8px;">
                    <strong>Detected GPU Hardware Accelerators:</strong>
                    <div style="display:flex;gap:8px;flex-wrap:wrap;margin-top:4px;">
                        ${profiles.map(p => `<span class="badge badge-success">${p.name}: ${(p.encoders || []).join(', ')}</span>`).join('')}
                    </div>
                </div>
            `;
        } else {
            profilesHtml = `<div style="margin-top:8px;color:var(--warning);">No dedicated hardware GPU encoders found; CPU fallback enabled.</div>`;
        }

        div.innerHTML = `
            <div><strong>OS / Arch:</strong> ${info.platform} (${info.arch}) | <strong>CPUs:</strong> ${info.cpus}</div>
            <div><strong>Go Version:</strong> ${info.go} | <strong>Morphic:</strong> v${info.version}</div>
            ${profilesHtml}
        `;
    } catch (err) {
        console.error(err);
    }
}
