// ── Morphic UI Utilities, Grouped Nav & State ─────────────────────────

// Map tabs to workflow groups
export const tabToGroupMap = {
    converter: 'transform',
    trimmer: 'transform',
    resizer: 'transform',
    dupfinder: 'analyze',
    inspector: 'analyze',
    gallery: 'analyze',
    hasher: 'analyze',
    organizer: 'organize',
    tagger: 'organize',
    syncer: 'organize',
    importer: 'ingest',
    archiver: 'ingest',
    reporter: 'ingest',
    trash: 'system',
    history: 'system',
    settings: 'system'
};

export const tabLabels = {
    converter: 'Converter',
    trimmer: 'Trimmer',
    resizer: 'Resizer',
    dupfinder: 'Dupfinder',
    inspector: 'Inspector',
    gallery: 'Gallery',
    hasher: 'Hasher',
    organizer: 'Organizer',
    tagger: 'Tagger',
    syncer: 'Syncer',
    importer: 'Importer',
    archiver: 'Archiver',
    reporter: 'Reporter',
    trash: 'Safe Trash',
    history: 'Audit History',
    settings: 'Watchers'
};

export function showToast(message, type = 'info') {
    const container = document.getElementById('toastContainer');
    if (!container) return;

    const toast = document.createElement('div');
    toast.className = `toast ${type}`;
    toast.setAttribute('role', 'alert');
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

export function closeAllNavMenus() {
    document.querySelectorAll('.nav-group').forEach(group => {
        group.classList.remove('open');
        const btn = group.querySelector('.nav-group-btn');
        if (btn) btn.setAttribute('aria-expanded', 'false');
    });
}

export function switchTab(tabId) {
    if (!tabId) return;
    const groupName = tabToGroupMap[tabId] || 'transform';

    // 1. Update tab buttons
    document.querySelectorAll('.tab-btn').forEach(btn => {
        const isActive = btn.dataset.tab === tabId;
        btn.classList.toggle('active', isActive);
        btn.setAttribute('aria-selected', isActive ? 'true' : 'false');
    });

    // 2. Update group containers & group buttons
    document.querySelectorAll('.nav-group').forEach(group => {
        const isGroupActive = group.dataset.group === groupName;
        group.classList.toggle('active-group', isGroupActive);
        const btn = group.querySelector('.nav-group-btn');
        if (btn) btn.classList.toggle('active', isGroupActive);

        // Update active indicator badge in group button
        const ind = group.querySelector('.nav-group-active-indicator');
        if (ind) {
            if (isGroupActive) {
                ind.textContent = tabLabels[tabId] || tabId;
                ind.style.display = 'inline-block';
            } else {
                ind.textContent = '';
                ind.style.display = 'none';
            }
        }
    });

    // 3. Switch tab content
    document.querySelectorAll('.tab-content').forEach(content => {
        const isActive = content.id === `tab-${tabId}`;
        content.classList.toggle('active', isActive);
    });

    // 4. Close any open dropdowns
    closeAllNavMenus();

    // 5. Inactive tab bulk bar management: ensure inactive bulk bars are hidden
    const convBulkBar = document.getElementById('convBulkBar');
    const dupBulkBar = document.getElementById('dupBulkBar');
    const taggerBulkBar = document.getElementById('taggerBulkBar');
    if (tabId !== 'converter' && convBulkBar) convBulkBar.style.display = 'none';
    if (tabId !== 'dupfinder' && dupBulkBar) dupBulkBar.style.display = 'none';
    if (tabId !== 'tagger' && taggerBulkBar) taggerBulkBar.style.display = 'none';

    // 6. Invoke module tab hooks if present
    if (tabId === 'trash' && typeof window.loadTrashHistory === 'function') {
        window.loadTrashHistory();
    }
    if (tabId === 'history' && typeof window.loadAuditHistory === 'function') {
        window.loadAuditHistory();
    }
    if (tabId === 'tagger' && typeof window.taggerInitTab === 'function') {
        window.taggerInitTab();
    }
    if (tabId === 'inspector' && typeof window.inspectorInitTab === 'function') {
        window.inspectorInitTab();
    }
    if (tabId === 'gallery' && typeof window.galleryInitTab === 'function') {
        window.galleryInitTab();
    }
    if (tabId === 'trimmer' && typeof window.trimmerInitTab === 'function') {
        window.trimmerInitTab();
    }
    if (tabId === 'hasher' && typeof window.hasherInitTab === 'function') {
        window.hasherInitTab();
    }
    if (tabId === 'archiver' && typeof window.archiverInitTab === 'function') {
        window.archiverInitTab();
    }
    if (tabId === 'importer' && typeof window.importerInitTab === 'function') {
        window.importerInitTab();
    }
    if (tabId === 'resizer' && typeof window.resizerInitTab === 'function') {
        window.resizerInitTab();
    }
    if (tabId === 'reporter' && typeof window.reporterInitTab === 'function') {
        window.reporterInitTab();
    }
    if (tabId === 'syncer' && typeof window.syncerInitTab === 'function') {
        window.syncerInitTab();
    }

    localStorage.setItem('morphic_active_tab', tabId);
}

export function initGroupedNavigation() {
    const groups = document.querySelectorAll('.nav-group');

    groups.forEach(group => {
        const btn = group.querySelector('.nav-group-btn');
        const menu = group.querySelector('.nav-group-menu');
        if (!btn || !menu) return;

        // Button click toggles menu
        btn.addEventListener('click', (e) => {
            e.stopPropagation();
            const isOpen = group.classList.contains('open');
            closeAllNavMenus();
            if (!isOpen) {
                group.classList.add('open');
                btn.setAttribute('aria-expanded', 'true');
            }
        });

        // Keyboard navigation on button
        btn.addEventListener('keydown', (e) => {
            if (e.key === 'ArrowDown' || e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                group.classList.add('open');
                btn.setAttribute('aria-expanded', 'true');
                const firstItem = menu.querySelector('button[role="menuitem"]');
                if (firstItem) firstItem.focus();
            } else if (e.key === 'Escape') {
                group.classList.remove('open');
                btn.setAttribute('aria-expanded', 'false');
            }
        });

        // Keyboard navigation inside dropdown menu
        const items = menu.querySelectorAll('button[role="menuitem"]');
        items.forEach((item, index) => {
            item.addEventListener('keydown', (e) => {
                if (e.key === 'ArrowDown') {
                    e.preventDefault();
                    const next = items[(index + 1) % items.length];
                    if (next) next.focus();
                } else if (e.key === 'ArrowUp') {
                    e.preventDefault();
                    const prev = items[(index - 1 + items.length) % items.length];
                    if (prev) prev.focus();
                } else if (e.key === 'Escape') {
                    e.preventDefault();
                    group.classList.remove('open');
                    btn.setAttribute('aria-expanded', 'false');
                    btn.focus();
                } else if (e.key === 'Tab') {
                    group.classList.remove('open');
                    btn.setAttribute('aria-expanded', 'false');
                }
            });
        });
    });

    // Click outside closes any open dropdown
    document.addEventListener('click', (e) => {
        if (!e.target.closest('.nav-group')) {
            closeAllNavMenus();
        }
    });

    // Escape closes any open dropdown
    document.addEventListener('keydown', (e) => {
        if (e.key === 'Escape') {
            closeAllNavMenus();
        }
    });
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

    const savedTab = localStorage.getItem('morphic_active_tab') || 'converter';
    if (document.getElementById(`tab-${savedTab}`)) {
        switchTab(savedTab);
    } else {
        switchTab('converter');
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
