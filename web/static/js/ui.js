// ── Morphic UI Utilities & State ─────────────────────────────────────

export function showToast(message, type = 'info') {
    const container = document.getElementById('toastContainer');
    if (!container) return;

    const toast = document.createElement('div');
    toast.className = `toast ${type}`;
    toast.textContent = message;
    container.appendChild(toast);

    setTimeout(() => {
        toast.style.opacity = '0';
        toast.style.transition = 'opacity 0.3s ease';
        setTimeout(() => toast.remove(), 300);
    }, 4000);
}

export function formatFileSize(bytes) {
    if (!bytes || bytes <= 0) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    let size = parseFloat(bytes);
    let unitIdx = 0;
    while (size >= 1024 && unitIdx < units.length - 1) {
        size /= 1024;
        unitIdx++;
    }
    return `${size.toFixed(2)} ${units[unitIdx]}`;
}

export function formatDuration(seconds) {
    if (!seconds || seconds <= 0) return '0s';
    const h = Math.floor(seconds / 3600);
    const m = Math.floor((seconds % 3600) / 60);
    const s = Math.floor(seconds % 60);
    if (h > 0) return `${h}h ${m}m ${s}s`;
    if (m > 0) return `${m}m ${s}s`;
    return `${s}s`;
}

export function switchTab(tabId) {
    document.querySelectorAll('.tab-btn').forEach(btn => {
        btn.classList.toggle('active', btn.dataset.tab === tabId);
    });
    document.querySelectorAll('.tab-content').forEach(content => {
        content.classList.toggle('active', content.id === `tab-${tabId}`);
    });

    // Inactive tab bulk bar management: ensure inactive bulk bars are hidden
    const convBulkBar = document.getElementById('convBulkBar');
    const dupBulkBar = document.getElementById('dupBulkBar');
    if (tabId !== 'converter' && convBulkBar) convBulkBar.style.display = 'none';
    if (tabId !== 'dupfinder' && dupBulkBar) dupBulkBar.style.display = 'none';

    if (tabId === 'trash' && typeof window.loadTrashHistory === 'function') {
        window.loadTrashHistory();
    }
    if (tabId === 'history' && typeof window.loadAuditHistory === 'function') {
        window.loadAuditHistory();
    }

    localStorage.setItem('morphic_active_tab', tabId);
}

export function toggleCardCollapse(cardId) {
    const card = document.getElementById(cardId);
    if (!card) return;
    card.classList.toggle('collapsed');
    const isCollapsed = card.classList.contains('collapsed');
    localStorage.setItem(`morphic_collapsed_${cardId}`, isCollapsed ? '1' : '0');
}

export function restoreCollapsedState() {
    document.querySelectorAll('.collapsible-card').forEach(card => {
        const saved = localStorage.getItem(`morphic_collapsed_${card.id}`);
        if (saved === '1') {
            card.classList.add('collapsed');
        }
    });

    const savedTab = localStorage.getItem('morphic_active_tab');
    if (savedTab && document.getElementById(`tab-${savedTab}`)) {
        switchTab(savedTab);
    }
}

export function openPreview(mediaPath) {
    const overlay = document.getElementById('previewOverlay');
    const content = document.getElementById('previewContent');
    if (!overlay || !content) return;

    content.innerHTML = '';
    const ext = mediaPath.split('.').pop().toLowerCase();
    const isVid = ['mp4', 'mov', 'avi', 'mkv', 'webm', 'm4v'].includes(ext);

    if (isVid) {
        const video = document.createElement('video');
        video.src = `/api/media?path=${encodeURIComponent(mediaPath)}`;
        video.controls = true;
        video.autoplay = true;
        content.appendChild(video);
    } else {
        const img = document.createElement('img');
        img.src = `/api/media?path=${encodeURIComponent(mediaPath)}`;
        content.appendChild(img);
    }

    overlay.classList.add('active');
}

export function closePreview() {
    const overlay = document.getElementById('previewOverlay');
    const content = document.getElementById('previewContent');
    if (overlay) overlay.classList.remove('active');
    if (content) content.innerHTML = '';
}
