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
window.deleteTrashItem = Trash.deleteTrashItem;
window.emptyAllTrash = Trash.emptyAllTrash;
window.purgeExpiredTrash = Trash.purgeExpiredTrash;

window.loadAuditHistory = History.loadAuditHistory;

window.loadWatchFolders = Settings.loadWatchFolders;
window.addWatchFolder = Settings.addWatchFolder;
window.deleteWatchFolder = Settings.deleteWatchFolder;

// Folder Browser Helpers
//
// The "Browse" button opens the browser's own native folder dialog (showDirectoryPicker), so the
// client OS file explorer is used (Windows Explorer, Finder, ...) regardless of the OS Morphic runs
// on. Browsers never reveal the real path of the picked folder, so the server locates it by name +
// content among everything it can access (/api/browse/resolve). The in-page server browser is the
// fallback for unsupported browsers or folders the server cannot see.
const FOLDER_PREFIXES = ['converter', 'dupfinder', 'organizer', 'watch'];

function getFolderDomIds(prefix) {
    if (prefix === 'converter') return { inputId: 'convFolder', browserId: 'convBrowser' };
    if (prefix === 'dupfinder') return { inputId: 'dupFolder', browserId: 'dupBrowser' };
    if (prefix === 'organizer') return { inputId: 'orgFolder', browserId: 'orgBrowser' };
    if (prefix === 'watch') return { inputId: 'watchNewPath', browserId: 'watchBrowser' };
    return { inputId: `${prefix}Folder`, browserId: `${prefix}Browser` };
}

function withTrailingSlash(p) {
    return p === '/' || p.endsWith('/') ? p : `${p}/`;
}

const currentBrowserPath = { converter: '', dupfinder: '', organizer: '', watch: '' };
const showHiddenFolders = { converter: false, dupfinder: false, organizer: false, watch: false };
const browseDebounceTimers = {};
const browseRequestSeq = {};

// Lists up to `limit` entry names of a picked directory handle (used to disambiguate same-named folders).
async function listPickedChildren(dirHandle, limit = 60) {
    const names = [];
    try {
        for await (const entry of dirHandle.values()) {
            if (entry && entry.name) names.push(entry.name);
            if (names.length >= limit) break;
        }
    } catch (_) { /* unreadable: resolve by name only */ }
    return names;
}

async function resolveFolderOnServer(name, children, hint) {
    try {
        const res = await fetch('/api/browse/resolve', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ name, children, hint })
        });
        if (res.ok) return await res.json();
    } catch (err) {
        console.warn('Folder resolve failed:', err);
    }
    return { resolved: false, path: '', candidates: [] };
}

function setFolderValue(prefix, path) {
    const { inputId } = getFolderDomIds(prefix);
    const input = document.getElementById(inputId);
    if (input) input.value = withTrailingSlash(path);
    window.closeBrowser(prefix);
    UI.showToast(`Selected: ${withTrailingSlash(path)}`, 'info');
}

// Shows several possible server paths for a picked folder so the user can confirm the right one.
function showResolveCandidates(prefix, name, candidates) {
    const { browserId } = getFolderDomIds(prefix);
    const browser = document.getElementById(browserId);
    if (!browser) return;
    browser.style.display = 'block';

    const pathHeader = browser.querySelector('.fb-path');
    const parentBtn = browser.querySelector('.fb-parent');
    const entriesDiv = browser.querySelector('.fb-entries');
    if (pathHeader) pathHeader.textContent = `Matches for "${name}"`;
    if (parentBtn) parentBtn.style.display = 'none';
    if (!entriesDiv) return;

    entriesDiv.replaceChildren();
    const intro = document.createElement('div');
    intro.style.cssText = 'padding:10px 12px;color:var(--text-dim);font-size:12px;';
    intro.textContent = 'Several folders match your selection. Pick the correct one:';
    entriesDiv.appendChild(intro);

    candidates.forEach(c => {
        const item = makeBrowserItem('📁', c.path, () => setFolderValue(prefix, c.path));
        item.title = `${c.matched_children} matching item(s)`;
        entriesDiv.appendChild(item);
    });
    entriesDiv.appendChild(makeBrowserItem('🔎', 'None of these – browse manually…',
        () => window.loadBrowserPath(prefix, '/', true)));
}

async function tryServerNativeDialog(prefix) {
    try {
        const res = await fetch('/api/browse/native', { method: 'POST' });
        const data = await res.json();
        if (data.available && data.folder) {
            setFolderValue(prefix, data.folder);
            return true;
        }
        if (data.available && data.cancelled) return true; // user cancelled the server dialog
    } catch (_) { /* not available */ }
    return false;
}

window.openNativeFolderExplorer = async function(prefix) {
    const { inputId } = getFolderDomIds(prefix);
    const input = document.getElementById(inputId);

    // 1. Browser-native folder dialog (client OS file explorer)
    if (typeof window.showDirectoryPicker === 'function') {
        let dirHandle;
        try {
            dirHandle = await window.showDirectoryPicker({ mode: 'read' });
        } catch (err) {
            if (err && err.name === 'AbortError') return; // user cancelled
            console.warn('showDirectoryPicker failed, using in-page folder browser:', err);
            await window.openBrowser(prefix);
            return;
        }

        UI.showToast(`Locating "${dirHandle.name}" on the server…`, 'info');
        const children = await listPickedChildren(dirHandle);
        const result = await resolveFolderOnServer(dirHandle.name, children, input?.value.trim() || '');

        if (result.resolved && result.path) {
            setFolderValue(prefix, result.path);
            return;
        }
        if (result.candidates && result.candidates.length > 0) {
            showResolveCandidates(prefix, dirHandle.name, result.candidates);
            return;
        }
        let hintMsg = `"${dirHandle.name}" was not found in the folders Morphic can access.`;
        if (result.empty && result.empty.length > 0) {
            hintMsg += ` These mount points are empty (drive not mounted into the container?): ${result.empty.join(', ')}.`;
        } else if (result.timed_out) {
            hintMsg += ' The search timed out; type the path manually or pick it below.';
        } else {
            hintMsg += ` Searched: ${(result.searched || []).slice(0, 6).join(', ')}.`;
        }
        UI.showToast(hintMsg, 'warning');
        await window.openBrowser(prefix, '/');
        return;
    }

    // 2. Browser has no folder dialog (Firefox/Safari, or not https/localhost): try a server-side dialog
    if (await tryServerNativeDialog(prefix)) return;

    // 3. In-page server folder browser
    UI.showToast('Your browser cannot open a native folder dialog here (use Chrome/Edge on localhost or https). Using the built-in browser.', 'info');
    await window.openBrowser(prefix);
};

window.toggleBrowser = async function(prefix) {
    const { browserId } = getFolderDomIds(prefix);
    const browser = document.getElementById(browserId);
    if (!browser) return;
    if (browser.style.display === 'block') {
        window.closeBrowser(prefix);
    } else {
        await window.openBrowser(prefix);
    }
};

window.openBrowser = async function(prefix, overridePath = null) {
    const { inputId, browserId } = getFolderDomIds(prefix);
    const browser = document.getElementById(browserId);
    if (!browser) return;
    browser.style.display = 'block';

    const input = document.getElementById(inputId);
    const initialPath = overridePath !== null ? overridePath : (input?.value.trim() || currentBrowserPath[prefix] || '');
    await window.loadBrowserPath(prefix, initialPath, true);
};

window.closeBrowser = function(prefix) {
    const { browserId } = getFolderDomIds(prefix);
    const browser = document.getElementById(browserId);
    if (browser) browser.style.display = 'none';
};

function makeBrowserItem(icon, label, onClick, className = 'fb-item') {
    const el = document.createElement('div');
    el.className = className;
    const iconEl = document.createElement('span');
    iconEl.className = 'icon';
    iconEl.textContent = icon;
    el.appendChild(iconEl);
    el.appendChild(document.createTextNode(` ${label}`));
    el.addEventListener('click', onClick);
    return el;
}

function renderBrowserRoots(prefix, browser, data) {
    let rootsDiv = browser.querySelector('.fb-roots');
    if (!rootsDiv) {
        rootsDiv = document.createElement('div');
        rootsDiv.className = 'fb-roots';
        const headerEl = browser.querySelector('.fb-header');
        if (headerEl) headerEl.after(rootsDiv); else browser.prepend(rootsDiv);
    }
    rootsDiv.replaceChildren();

    const label = document.createElement('span');
    label.style.cssText = 'font-size:11px;color:var(--text-dim);margin-right:2px;';
    label.textContent = 'Jump to:';
    rootsDiv.appendChild(label);

    (data.roots || []).forEach(r => {
        const pill = document.createElement('button');
        pill.type = 'button';
        const isActive = data.current === r.path || (r.path !== '/' && data.current.startsWith(`${r.path}/`));
        pill.className = `fb-root-pill${isActive ? ' active' : ''}`;
        pill.textContent = `📁 ${r.label}`;
        pill.title = r.path;
        pill.addEventListener('click', () => window.selectEntryPath(prefix, r.path));
        rootsDiv.appendChild(pill);
    });

    const hiddenLabel = document.createElement('label');
    hiddenLabel.className = 'fb-hidden-toggle';
    const cb = document.createElement('input');
    cb.type = 'checkbox';
    cb.checked = !!showHiddenFolders[prefix];
    cb.addEventListener('change', () => {
        showHiddenFolders[prefix] = cb.checked;
        window.loadBrowserPath(prefix, currentBrowserPath[prefix] || '');
    });
    hiddenLabel.appendChild(cb);
    hiddenLabel.appendChild(document.createTextNode(' Show hidden'));
    rootsDiv.appendChild(hiddenLabel);
}

window.loadBrowserPath = async function(prefix, path, explicitOpen = false) {
    const { inputId, browserId } = getFolderDomIds(prefix);
    const browser = document.getElementById(browserId);
    if (!browser) return;

    const seq = (browseRequestSeq[prefix] = (browseRequestSeq[prefix] || 0) + 1);
    const hiddenParam = showHiddenFolders[prefix] ? '&hidden=1' : '';
    const entriesDiv = browser.querySelector('.fb-entries');

    try {
        let res = await fetch(`/api/browse?path=${encodeURIComponent(path)}${hiddenParam}`);
        if (!res.ok && explicitOpen && path !== '') {
            // Typed/previous path is not usable: fall back to the server's default start location
            res = await fetch(`/api/browse?path=${hiddenParam}`);
        }
        if (seq !== browseRequestSeq[prefix]) return; // a newer request superseded this one
        if (!res.ok) {
            if (explicitOpen && entriesDiv) {
                let msg = 'Could not list this folder';
                try { msg = (await res.json()).error?.message || msg; } catch (_) {}
                entriesDiv.replaceChildren();
                const err = document.createElement('div');
                err.style.cssText = 'padding:12px;color:var(--danger);font-size:12px;';
                err.textContent = msg;
                entriesDiv.appendChild(err);
            }
            // Otherwise the user is probably still typing a path
            return;
        }
        const data = await res.json();
        currentBrowserPath[prefix] = data.current;

        // A pasted Windows path (D:\Photos, \\wsl$\...) was translated by the server: show the real path
        const winLike = /^([A-Za-z]:[\\/]|[\\/]{2}wsl)/i;
        const inputEl = document.getElementById(inputId);
        if (inputEl && data.exists !== false && winLike.test(path) && !/^[A-Za-z]:[\\/]/.test(data.current)) {
            inputEl.value = withTrailingSlash(data.current);
        }

        const pathHeader = browser.querySelector('.fb-path');
        const parentBtn = browser.querySelector('.fb-parent');

        if (pathHeader) pathHeader.textContent = data.current;
        renderBrowserRoots(prefix, browser, data);

        if (parentBtn) {
            parentBtn.style.display = data.parent ? 'flex' : 'none';
            parentBtn.onclick = () => {
                if (data.parent) window.selectEntryPath(prefix, data.parent);
            };
        }

        if (entriesDiv) {
            entriesDiv.replaceChildren();
            if (data.error) {
                const err = document.createElement('div');
                err.style.cssText = 'padding:12px;color:var(--danger);font-size:12px;';
                err.textContent = data.error;
                entriesDiv.appendChild(err);
            }
            if (!data.entries || data.entries.length === 0) {
                if (!data.error) {
                    const empty = document.createElement('div');
                    empty.style.cssText = 'padding:12px;color:var(--text-dim);font-size:12px;';
                    empty.textContent = 'No subdirectories found';
                    entriesDiv.appendChild(empty);
                }
            } else {
                data.entries.forEach(e => {
                    entriesDiv.appendChild(makeBrowserItem('📁', e.name, () => window.selectEntryPath(prefix, e.path)));
                });
            }
        }
    } catch (err) {
        console.error('Folder browser error:', err);
    }
};

window.selectEntryPath = function(prefix, path) {
    const { inputId } = getFolderDomIds(prefix);
    const input = document.getElementById(inputId);
    // "/" is only for navigating; it is not a valid scan/watch target
    if (input && path !== '/') {
        input.value = withTrailingSlash(path);
    }
    window.loadBrowserPath(prefix, path);
};

window.selectBrowserFolder = function(prefix) {
    const { inputId } = getFolderDomIds(prefix);
    const input = document.getElementById(inputId);
    const p = currentBrowserPath[prefix];
    if (input && p) {
        if (p === '/') {
            UI.showToast('Please open a subfolder first (e.g. /media or /mnt/d)', 'warning');
            return;
        }
        input.value = withTrailingSlash(p);
        UI.showToast(`Selected: ${input.value}`, 'info');
    }
    window.closeBrowser(prefix);
};

function initFolderInputEvents() {
    FOLDER_PREFIXES.forEach(prefix => {
        const { inputId, browserId } = getFolderDomIds(prefix);
        const input = document.getElementById(inputId);
        const browser = document.getElementById(browserId);
        if (!input || !browser) return;

        input.addEventListener('focus', () => {
            if (browser.style.display !== 'block') window.openBrowser(prefix);
        });

        input.addEventListener('click', () => {
            if (browser.style.display !== 'block') window.openBrowser(prefix);
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
        FOLDER_PREFIXES.forEach(prefix => {
            const { inputId, browserId } = getFolderDomIds(prefix);
            const input = document.getElementById(inputId);
            const browser = document.getElementById(browserId);
            if (!browser || browser.style.display !== 'block') return;
            // Ignore clicks inside the browser or on the input/Browse button row. Re-rendered
            // entries are detached from the DOM by the time this runs, so also check isConnected.
            const inputRow = input?.closest('.folder-input-row');
            const inside = browser.contains(e.target) || !e.target.isConnected ||
                (inputRow && inputRow.contains(e.target));
            if (!inside) window.closeBrowser(prefix);
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
