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

// New Modules
import * as Tagger from './tagger.js';
import * as Inspector from './inspector.js';
import * as Gallery from './gallery.js';
import * as Trimmer from './trimmer.js';
import * as Hasher from './hasher.js';
import * as Archiver from './archiver.js';
import * as Importer from './importer.js';
import * as Resizer from './resizer.js';
import * as Reporter from './reporter.js';
import * as Syncer from './syncer.js';

// ── Export Core UI functions to global scope for HTML handlers ────────
window.switchTab = UI.switchTab;
window.toggleCardCollapse = UI.toggleCardCollapse;
window.openPreview = UI.openPreview;
window.closePreview = UI.closePreview;

// Compare Modal
window.openCompareModal = Compare.openCompareModal;
window.closeCompareModal = Compare.closeCompareModal;
window.setCompareMode = Compare.setCompareMode;

// Converter
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

// Dupfinder
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

// Organizer
window.orgModeChanged = Organizer.orgModeChanged;
window.orgStartPlan = Organizer.orgStartPlan;
window.orgExecute = Organizer.orgExecute;
window.orgCancelJob = Organizer.orgCancelJob;

// Trash & History
window.loadTrashHistory = Trash.loadTrashHistory;
window.undoAuditAction = Trash.undoAuditAction;
window.purgeExpiredTrash = Trash.purgeExpiredTrash;
window.loadAuditHistory = History.loadAuditHistory;

// Settings
window.loadWatchFolders = Settings.loadWatchFolders;
window.addWatchFolder = Settings.addWatchFolder;
window.deleteWatchFolder = Settings.deleteWatchFolder;

// ── Export New Module functions to global scope ───────────────────────

// Tagger
window.taggerInitTab = Tagger.taggerInitTab;
window.taggerScan = Tagger.taggerScan;
window.taggerFilterResults = Tagger.taggerFilterResults;
window.taggerToggleSelectAll = Tagger.taggerToggleSelectAll;
window.taggerToggleSelectFile = Tagger.taggerToggleSelectFile;
window.taggerDeselectAll = Tagger.taggerDeselectAll;
window.taggerApplyBatch = Tagger.taggerApplyBatch;
window.taggerConfirmStrip = Tagger.taggerConfirmStrip;
window.taggerStripSingle = Tagger.taggerStripSingle;
window.taggerEditSingle = Tagger.taggerEditSingle;

// Inspector
window.inspectorInitTab = Inspector.inspectorInitTab;
window.inspectorAnalyze = Inspector.inspectorAnalyze;
window.inspectorSetMode = Inspector.inspectorSetMode;
window.inspectorSetColor = Inspector.inspectorSetColor;
window.inspectorResetZoom = Inspector.inspectorResetZoom;
window.inspectorJumpBreadcrumb = Inspector.inspectorJumpBreadcrumb;

// Gallery & Lightbox
window.galleryInitTab = Gallery.galleryInitTab;
window.galleryLoad = Gallery.galleryLoad;
window.gallerySetFilter = Gallery.gallerySetFilter;
window.galleryFilterByName = Gallery.galleryFilterByName;
window.gallerySetSort = Gallery.gallerySetSort;
window.gallerySetGridSize = Gallery.gallerySetGridSize;
window.openLightbox = Gallery.openLightbox;
window.closeLightbox = Gallery.closeLightbox;
window.nextLightboxItem = Gallery.nextLightboxItem;
window.prevLightboxItem = Gallery.prevLightboxItem;
window.lightboxZoom = Gallery.lightboxZoom;
window.lightboxZoomReset = Gallery.lightboxZoomReset;

// Trimmer
window.trimmerInitTab = Trimmer.trimmerInitTab;
window.trimmerLoad = Trimmer.trimmerLoad;
window.trimmerRotate = Trimmer.trimmerRotate;
window.trimmerFlipH = Trimmer.trimmerFlipH;
window.trimmerFlipV = Trimmer.trimmerFlipV;
window.trimmerSetAspect = Trimmer.trimmerSetAspect;
window.trimmerUpdateTransforms = Trimmer.trimmerUpdateTransforms;
window.trimmerOnResizeInput = Trimmer.trimmerOnResizeInput;
window.trimmerSetStartCurrent = Trimmer.trimmerSetStartCurrent;
window.trimmerSetEndCurrent = Trimmer.trimmerSetEndCurrent;
window.trimmerUpdateVideoRange = Trimmer.trimmerUpdateVideoRange;
window.trimmerExport = Trimmer.trimmerExport;

// Hasher
window.hasherInitTab = Hasher.hasherInitTab;
window.hasherSwitchMode = Hasher.hasherSwitchMode;
window.hasherStartGenerate = Hasher.hasherStartGenerate;
window.hasherCancelJob = Hasher.hasherCancelJob;
window.hasherCopyHash = Hasher.hasherCopyHash;
window.hasherExportManifest = Hasher.hasherExportManifest;
window.hasherStartVerify = Hasher.hasherStartVerify;

// Archiver
window.archiverInitTab = Archiver.archiverInitTab;
window.archiverToggleSplit = Archiver.archiverToggleSplit;
window.archiverSetSplitPreset = Archiver.archiverSetSplitPreset;
window.archiverOnFormatChange = Archiver.archiverOnFormatChange;
window.archiverCreate = Archiver.archiverCreate;
window.archiverCancel = Archiver.archiverCancel;

// Importer
window.importerInitTab = Importer.importerInitTab;
window.importerSetSource = Importer.importerSetSource;
window.importerScanDevices = Importer.importerScanDevices;
window.importerSelectDevice = Importer.importerSelectDevice;
window.importerStart = Importer.importerStart;
window.importerCancel = Importer.importerCancel;

// Resizer
window.resizerInitTab = Resizer.resizerInitTab;
window.resizerTogglePreset = Resizer.resizerTogglePreset;
window.resizerStart = Resizer.resizerStart;
window.resizerCancel = Resizer.resizerCancel;

// Reporter
window.reporterInitTab = Reporter.reporterInitTab;
window.reporterGenerate = Reporter.reporterGenerate;
window.reporterDownloadHTML = Reporter.reporterDownloadHTML;
window.reporterPrintPDF = Reporter.reporterPrintPDF;

// Syncer
window.syncerInitTab = Syncer.syncerInitTab;
window.syncerPlan = Syncer.syncerPlan;
window.syncerExecute = Syncer.syncerExecute;
window.syncerCancel = Syncer.syncerCancel;

// ── Data-Driven Folder Browser Mapping & Helpers ───────────────────────

const folderConfigMap = {
    converter: { input: 'convFolder', browser: 'convBrowser' },
    dupfinder: { input: 'dupFolder', browser: 'dupBrowser' },
    organizer: { input: 'orgFolder', browser: 'orgBrowser' },
    tagger: { input: 'taggerFolder', browser: 'taggerBrowser' },
    inspector: { input: 'inspectorFolder', browser: 'inspectorBrowser' },
    gallery: { input: 'galleryFolder', browser: 'galleryBrowser' },
    trimmer: { input: 'trimmerPath', browser: 'trimmerBrowser' },
    hasher: { input: 'hasherFolder', browser: 'hasherBrowser' },
    archiver: { input: 'archiverFolder', browser: 'archiverBrowser' },
    importer: { input: 'importerFolder', browser: 'importerBrowser' },
    resizer: { input: 'resizerFolder', browser: 'resizerBrowser' },
    resizerOut: { input: 'resizerOutputFolder', browser: 'resizerOutputBrowser' },
    reporter: { input: 'reporterFolder', browser: 'reporterBrowser' },
    syncerSource: { input: 'syncerSourceFolder', browser: 'syncerSourceBrowser' },
    syncerTarget: { input: 'syncerTargetFolder', browser: 'syncerTargetBrowser' }
};

function getFolderElements(prefix) {
    const cfg = folderConfigMap[prefix] || { input: `${prefix}Folder`, browser: `${prefix}Browser` };
    return {
        input: document.getElementById(cfg.input),
        browser: document.getElementById(cfg.browser),
        inputId: cfg.input,
        browserId: cfg.browser
    };
}

const currentBrowserPath = {};
const browseDebounceTimers = {};

async function resolveSmartMountPath(currentVal, folderName) {
    if (!folderName) return '';

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
    const { input } = getFolderElements(prefix);

    // 1. Browser native directory picker
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
            if (err.name === 'AbortError') return;
            console.warn('showDirectoryPicker fallback:', err);
        }
    }

    // 2. Server-side native dialog fallback
    try {
        const res = await fetch('/api/browse/native', { method: 'POST' });
        const data = await res.json();
        if (data.available && data.folder) {
            if (input) input.value = data.folder;
            UI.showToast(`Selected folder: ${data.folder}`, 'info');
            return;
        }
    } catch (_) {}

    // 3. In-page server folder browser
    await window.openBrowser(prefix);
};

window.toggleBrowser = async function(prefix) {
    const { browser } = getFolderElements(prefix);
    if (!browser) return;
    if (browser.style.display === 'block') {
        window.closeBrowser(prefix);
    } else {
        await window.openBrowser(prefix);
    }
};

window.openBrowser = async function(prefix) {
    const { input, browser } = getFolderElements(prefix);
    if (!browser) return;
    browser.style.display = 'block';

    const initialPath = input?.value.trim() || currentBrowserPath[prefix] || '';
    await window.loadBrowserPath(prefix, initialPath);
};

window.closeBrowser = function(prefix) {
    const { browser } = getFolderElements(prefix);
    if (browser) browser.style.display = 'none';
};

window.loadBrowserPath = async function(prefix, path) {
    const { input, browser } = getFolderElements(prefix);
    if (!browser) return;

    try {
        const res = await fetch(`/api/browse?path=${encodeURIComponent(path)}`);
        if (!res.ok) return;

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
    const { input } = getFolderElements(prefix);
    const formattedPath = path.endsWith('/') ? path : (path + '/');
    if (input) input.value = formattedPath;
    window.loadBrowserPath(prefix, formattedPath);
};

window.selectBrowserFolder = function(prefix) {
    const { input } = getFolderElements(prefix);
    if (input && currentBrowserPath[prefix]) {
        const p = currentBrowserPath[prefix];
        input.value = p.endsWith('/') ? p : (p + '/');
        UI.showToast(`Selected: ${input.value}`, 'info');
    }
    window.closeBrowser(prefix);
};

function initFolderInputEvents() {
    const prefixes = Object.keys(folderConfigMap);

    prefixes.forEach(prefix => {
        const { input, browser } = getFolderElements(prefix);
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
        prefixes.forEach(prefix => {
            const { input, browser } = getFolderElements(prefix);
            if (browser && browser.style.display === 'block') {
                if (!browser.contains(e.target) && e.target !== input) {
                    window.closeBrowser(prefix);
                }
            }
        });
    });
}

// ── Initialize App & Modules on DOMContentLoaded ───────────────────────
document.addEventListener('DOMContentLoaded', () => {
    UI.restoreCollapsedState();
    UI.initGroupedNavigation();
    Events.initEventSource();
    initFolderInputEvents();

    Converter.initConverter();
    Dupfinder.initDupfinder();
    Organizer.initOrganizer();
    Trash.initTrash();
    History.initHistory();
    Settings.initSettings();

    // Init new modules
    Tagger.initTagger();
    Inspector.initInspector();
    Gallery.initGallery();
    Trimmer.initTrimmer();
    Hasher.initHasher();
    Archiver.initArchiver();
    Importer.initImporter();
    Resizer.initResizer();
    Reporter.initReporter();
    Syncer.initSyncer();
});
