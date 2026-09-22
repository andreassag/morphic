// ── Main Morphic Application Entrypoint (ES Module) ─────────────────

import * as UI from './ui.js';
import * as Events from './events.js';
import * as Compare from './compare.js';
import * as Converter from './converter.js';
import * as Dupfinder from './dupfinder.js';
import * as Organizer from './organizer.js';
import * as Trash from './trash.js';
import * as History from './history.js';
import * as Settings from './settings.js';

// Export functions to global scope for HTML inline handlers
window.switchTab = UI.switchTab;
window.toggleCardCollapse = UI.toggleCardCollapse;
window.openPreview = UI.openPreview;
window.closePreview = UI.closePreview;

window.openCompareModal = Compare.openCompareModal;
window.closeCompareModal = Compare.closeCompareModal;
window.setCompareMode = Compare.setCompareMode;

window.convScan = Converter.convScan;
window.convSortResults = Converter.convSortResults;
window.convToggleExtFilter = Converter.convToggleExtFilter;
window.convToggleSelectAll = Converter.convToggleSelectAll;
window.convToggleSelectFile = Converter.convToggleSelectFile;
window.toggleFullPaths = Converter.toggleFullPaths;
window.convOnBatchContainerChange = Converter.convOnBatchContainerChange;
window.convConvertBatch = Converter.convConvertBatch;
window.convStopConvert = Converter.convStopConvert;
window.convDeleteBatch = Converter.convDeleteBatch;

window.dupStartScan = Dupfinder.dupStartScan;
window.dupCancelScan = Dupfinder.dupCancelScan;
window.dupApplyRule = Dupfinder.dupApplyRule;
window.dupClearSelection = Dupfinder.dupClearSelection;
window.dupExpandAll = Dupfinder.dupExpandAll;
window.dupCollapseAll = Dupfinder.dupCollapseAll;
window.dupToggleSelect = Dupfinder.dupToggleSelect;
window.dupConfirmDelete = Dupfinder.dupConfirmDelete;
window.dupCloseModal = Dupfinder.dupCloseModal;
window.dupExecuteDelete = Dupfinder.dupExecuteDelete;
window.dupDeleteSingle = Dupfinder.dupDeleteSingle;

window.orgModeChanged = Organizer.orgModeChanged;
window.orgStartPlan = Organizer.orgStartPlan;
window.orgExecute = Organizer.orgExecute;
window.orgCancelJob = Organizer.orgCancelJob;

window.loadTrashHistory = Trash.loadTrashHistory;
window.undoAuditAction = Trash.undoAuditAction;
window.purgeExpiredTrash = Trash.purgeExpiredTrash;

window.loadAuditHistory = History.loadAuditHistory;

window.loadWatchFolders = Settings.loadWatchFolders;
window.addWatchFolder = Settings.addWatchFolder;
window.deleteWatchFolder = Settings.deleteWatchFolder;

// Folder Browser Helpers
async function resolveSmartMountPath(currentVal, folderName) {
    if (!folderName) return '';

    // If current input has an absolute container path, check if it's already a parent directory
    if (currentVal && (currentVal.startsWith('/mnt/') || currentVal.startsWith('/media') || currentVal.startsWith('/app/'))) {
        const base = currentVal.endsWith('/') ? currentVal.slice(0, -1) : currentVal;
        const subPath = `${base}/${folderName}`;
        try {
            const res = await fetch(`/api/browse?path=${encodeURIComponent(subPath)}`);
            if (res.ok) {
                const data = await res.json();
                if (data.exists !== false && data.current === subPath) return subPath;
            }
        } catch (_) {}
    }

    // Candidate container mount points to probe
    const candidates = [
        `/mnt/d/storage/${folderName}`,
        `/mnt/d/${folderName}`,
        `/media/${folderName}`
    ];

    for (const cand of candidates) {
        try {
            const res = await fetch(`/api/browse?path=${encodeURIComponent(cand)}`);
            if (res.ok) {
                const data = await res.json();
                if (data.exists !== false && data.current === cand) return cand;
            }
        } catch (_) {}
    }

    return '';
}

window.openNativeFolderExplorer = async function(prefix) {
    const inputId = `${prefix === 'converter' ? 'conv' : prefix === 'dupfinder' ? 'dup' : 'org'}Folder`;
    const input = document.getElementById(inputId);

    // 1. Browser native directory picker (Approach 1)
    if ('showDirectoryPicker' in window) {
        try {
            const dirHandle = await window.showDirectoryPicker();
            if (dirHandle && dirHandle.name) {
                const resolved = await resolveSmartMountPath(input?.value.trim(), dirHandle.name);
                if (resolved && input) {
                    input.value = resolved;
                    UI.showToast(`Selected: ${resolved}`, 'info');
                    return;
                }
                UI.showToast(`Browsing for folder: ${dirHandle.name}`, 'info');
                await window.openBrowser(prefix);
                return;
            }
        } catch (err) {
            if (err.name === 'AbortError') {
                return; // User cancelled picker
            }
            console.warn('showDirectoryPicker failed or cancelled, falling back to server browser:', err);
        }
    }

    // 2. Server-side native dialog fallback (e.g. desktop mode)
    try {
        const res = await fetch('/api/browse/native', { method: 'POST' });
        const data = await res.json();
        if (data.available && data.folder) {
            if (input) input.value = data.folder;
            UI.showToast(`Selected folder: ${data.folder}`, 'info');
            return;
        }
    } catch (_) {}

    // 3. Fallback to in-page server folder browser
    await window.openBrowser(prefix);
};

let currentBrowserPath = { converter: '', dupfinder: '', organizer: '' };
const browseDebounceTimers = {};

window.toggleBrowser = async function(prefix) {
    const idPrefix = prefix === 'converter' ? 'conv' : prefix === 'dupfinder' ? 'dup' : 'org';
    const browser = document.getElementById(`${idPrefix}Browser`);
    if (!browser) return;
    if (browser.style.display === 'block') {
        window.closeBrowser(prefix);
    } else {
        await window.openBrowser(prefix);
    }
};

window.openBrowser = async function(prefix) {
    const idPrefix = prefix === 'converter' ? 'conv' : prefix === 'dupfinder' ? 'dup' : 'org';
    const browser = document.getElementById(`${idPrefix}Browser`);
    if (!browser) return;
    browser.style.display = 'block';

    const input = document.getElementById(`${idPrefix}Folder`);
    const initialPath = input?.value.trim() || currentBrowserPath[prefix] || '';
    await window.loadBrowserPath(prefix, initialPath);
};

window.closeBrowser = function(prefix) {
    const idPrefix = prefix === 'converter' ? 'conv' : prefix === 'dupfinder' ? 'dup' : 'org';
    const browser = document.getElementById(`${idPrefix}Browser`);
    if (browser) browser.style.display = 'none';
};

window.loadBrowserPath = async function(prefix, path) {
    const idPrefix = prefix === 'converter' ? 'conv' : prefix === 'dupfinder' ? 'dup' : 'org';
    const browser = document.getElementById(`${idPrefix}Browser`);
    if (!browser) return;

    try {
        let res = await fetch(`/api/browse?path=${encodeURIComponent(path)}`);
        if (!res.ok) {
            // Path doesn't resolve or isn't a valid directory yet (user still typing)
            return;
        }
        const data = await res.json();
        currentBrowserPath[prefix] = data.current;

        const pathHeader = browser.querySelector('.fb-path');
        const parentBtn = browser.querySelector('.fb-parent');
        const entriesDiv = browser.querySelector('.fb-entries');

        if (pathHeader) pathHeader.textContent = data.current;
        if (parentBtn) {
            parentBtn.style.display = data.parent ? 'flex' : 'none';
            parentBtn.onclick = () => {
                const newPath = data.parent.endsWith('/') ? data.parent : (data.parent + '/');
                const input = document.getElementById(`${idPrefix}Folder`);
                if (input) input.value = newPath;
                window.loadBrowserPath(prefix, newPath);
            };
        }

        if (entriesDiv) {
            if (!data.entries || data.entries.length === 0) {
                entriesDiv.innerHTML = '<div style="padding:12px;color:var(--text-dim);font-size:12px;">No subdirectories found</div>';
            } else {
                entriesDiv.innerHTML = data.entries.map(e => {
                    const safePath = e.path.replace(/\\/g, '\\\\').replace(/'/g, "\\'");
                    return `
                        <div class="fb-item" onclick="window.selectEntryPath('${prefix}', '${safePath}')">
                            <span class="icon">📁</span> ${e.name}
                        </div>
                    `;
                }).join('');
            }
        }
    } catch (err) {
        console.error('Folder browser error:', err);
    }
};

window.selectEntryPath = function(prefix, path) {
    const idPrefix = prefix === 'converter' ? 'conv' : prefix === 'dupfinder' ? 'dup' : 'org';
    const input = document.getElementById(`${idPrefix}Folder`);
    const formattedPath = path.endsWith('/') ? path : (path + '/');
    if (input) input.value = formattedPath;
    window.loadBrowserPath(prefix, formattedPath);
};

window.selectBrowserFolder = function(prefix) {
    const idPrefix = prefix === 'converter' ? 'conv' : prefix === 'dupfinder' ? 'dup' : 'org';
    const input = document.getElementById(`${idPrefix}Folder`);
    if (input && currentBrowserPath[prefix]) {
        const p = currentBrowserPath[prefix];
        input.value = p.endsWith('/') ? p : (p + '/');
        UI.showToast(`Selected: ${input.value}`, 'info');
    }
    window.closeBrowser(prefix);
};

function initFolderInputEvents() {
    const tabs = ['converter', 'dupfinder', 'organizer'];
    tabs.forEach(prefix => {
        const idPrefix = prefix === 'converter' ? 'conv' : prefix === 'dupfinder' ? 'dup' : 'org';
        const input = document.getElementById(`${idPrefix}Folder`);
        const browser = document.getElementById(`${idPrefix}Browser`);
        if (!input || !browser) return;

        input.addEventListener('focus', () => {
            window.openBrowser(prefix);
        });

        input.addEventListener('click', () => {
            if (browser.style.display !== 'block') {
                window.openBrowser(prefix);
            }
        });

        input.addEventListener('input', () => {
            if (browser.style.display !== 'block') {
                browser.style.display = 'block';
            }
            clearTimeout(browseDebounceTimers[prefix]);
            browseDebounceTimers[prefix] = setTimeout(() => {
                window.loadBrowserPath(prefix, input.value.trim());
            }, 180);
        });

        input.addEventListener('keydown', (e) => {
            if (e.key === 'Escape') {
                window.closeBrowser(prefix);
            }
        });
    });

    document.addEventListener('click', (e) => {
        tabs.forEach(prefix => {
            const idPrefix = prefix === 'converter' ? 'conv' : prefix === 'dupfinder' ? 'dup' : 'org';
            const input = document.getElementById(`${idPrefix}Folder`);
            const browser = document.getElementById(`${idPrefix}Browser`);
            if (browser && browser.style.display === 'block') {
                if (!browser.contains(e.target) && e.target !== input) {
                    window.closeBrowser(prefix);
                }
            }
        });
    });
}

// Initialize Modules on DOMContentLoaded
document.addEventListener('DOMContentLoaded', () => {
    UI.restoreCollapsedState();
    Events.initEventSource();
    initFolderInputEvents();
    Converter.initConverter();
    Dupfinder.initDupfinder();
    Organizer.initOrganizer();
    Trash.initTrash();
    History.initHistory();
    Settings.initSettings();
});
