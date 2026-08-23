/* ── Morphic — Unified JS ─────────────────────────────────────────── */

// =====================================================================
// Global state
// =====================================================================

let activeTab = 'converter';
let lastSelectedFolder = '';

// Converter state
let convScanData = null;        // last scan result
let convFilterType = 'both';    // images|videos|both
let convFilterExt = null;       // filter by specific extension
let convSelectedFiles = [];     // files selected for conversion
let convAvailableTargets = [];  // image format targets for the image dropdown
let convAv1Available = false;   // AV1 support from ffmpeg
let convVideoFormats = [];      // VideoContainerConfig[] from backend
let convCodecLabels = {         // codec ID → display label
    h264: 'H.264 (AVC)', h265: 'H.265 (HEVC)',
    av1: 'AV1', vp8: 'VP8', vp9: 'VP9',
};
let convFileResults = new Map();     // path → {status:'ok'|'error'|'converting', ...}
let convBatchMode = 'intersection';    // union|intersection
const convAv1Crf = 35;
let convJobId = null;
let convPollTimer = null;
let convScanning = false;           // true while folder scan is in flight
let convScanController = null;     // AbortController for active scan request
let convScanElapsedTimer = null;   // interval for scan elapsed counter
let showFullPaths = false;

async function convLoadFormats() {
    try {
        const savedMode = localStorage.getItem('convBatchMode');
        if (savedMode === 'union' || savedMode === 'intersection') {
            convBatchMode = savedMode;
        }

        const modeSelect = document.getElementById('convBatchMode');
        if (modeSelect) {
            modeSelect.value = convBatchMode;
        }

        const resp = await fetch('/api/converter/formats');
        const data = await resp.json();

        // Augment with system diagnostic info for AV1 support
        try {
            const sysResp = await fetch('/api/system_info');
            const sysInfo = await sysResp.json();
            const ffenc = sysInfo.ffmpeg && sysInfo.ffmpeg.encoders || [];
            convAv1Available = ffenc.some(e => e.includes('av1_nvenc') || e.includes('libsvtav1') || e.includes('libaom-av1'));
        } catch (e) {
            convAv1Available = false;
        }

        // Parse structured video container config
        convVideoFormats = (data.video && data.video.containers) || [];

        // Collect image format targets for the image dropdown
        const imagetargets = new Set();
        if (data.image) {
            Object.values(data.image).flat().forEach(t => imagetargets.add(t));
        }
        convAvailableTargets = [...imagetargets].sort();

        convInitBatchVideoDropdowns();
        convSetBatchTargets(convAvailableTargets);
    } catch (e) {
        console.error('Failed to load converter formats:', e);
    }
}

function convInitBatchVideoDropdowns() {
    const containerSel = document.getElementById('convBatchContainer');
    if (!containerSel || convVideoFormats.length === 0) return;
    containerSel.innerHTML = convVideoFormats.map(c =>
        `<option value="${c.name}">${c.name}</option>`
    ).join('');
    convOnBatchContainerChange();
}

function convOnBatchContainerChange() {
    const containerSel = document.getElementById('convBatchContainer');
    const codecSel = document.getElementById('convBatchCodec');
    const extSel = document.getElementById('convBatchExt');
    if (containerSel && codecSel && extSel) {
        convFilterCodecExt(containerSel.value, codecSel, extSel);
    }
}

function convFilterCodecExt(containerName, codecSel, extSel) {
    const container = convVideoFormats.find(c => c.name === containerName);
    if (!container) return;

    const prevCodec = codecSel.value;
    const prevExt = extSel.value;

    codecSel.innerHTML = container.codecs.map(codec => {
        const label = convCodecLabels[codec] || codec.toUpperCase();
        const disabled = codec === 'av1' && !convAv1Available;
        return `<option value="${codec}" ${disabled ? 'disabled' : ''}>${label}${disabled ? ' (unavailable)' : ''}</option>`;
    }).join('');

    if (container.codecs.includes(prevCodec) && !(prevCodec === 'av1' && !convAv1Available)) {
        codecSel.value = prevCodec;
    }

    extSel.innerHTML = container.extensions.map(ext =>
        `<option value="${ext}">${ext}</option>`
    ).join('');

    if (container.extensions.includes(prevExt)) {
        extSel.value = prevExt;
    }
}

function convGetSelectedByType() {
    if (!convScanData || !convScanData.files) return { videos: [], images: [] };
    const selectedSet = new Set(convSelectedFiles);
    const videos = [], images = [];
    for (const f of convScanData.files) {
        if (!selectedSet.has(f.path)) continue;
        if (f.type === 'video') videos.push(f.path);
        else images.push(f.path);
    }
    return { videos, images };
}

function convUpdateBatchDropdowns() {
    const { videos, images } = convGetSelectedByType();
    const videoDiv = document.getElementById('convVideoDropdowns');
    const imageDiv = document.getElementById('convImageDropdown');

    if (videoDiv) videoDiv.style.display = videos.length > 0 ? 'flex' : 'none';
    if (imageDiv) imageDiv.style.display = images.length > 0 ? 'flex' : 'none';

    if (images.length > 0) {
        const targets = convGetBatchTargets();
        convSetBatchTargets(targets);
    }

    const batchBtn = document.getElementById('convBatchBtn');
    if (batchBtn) {
        batchBtn.disabled = videos.length === 0 && images.length === 0;
    }
}

function convOnRowContainerChange(containerSel) {
    const group = containerSel.closest('.conv-video-format-group');
    const codecSel = group.querySelector('.conv-vid-codec');
    const extSel = group.querySelector('.conv-vid-ext');
    convFilterCodecExt(containerSel.value, codecSel, extSel);
}

function convBuildVideoSelectsHtml() {
    if (convVideoFormats.length === 0) {
        return `<select class="conv-vid-ext" disabled><option>Loading…</option></select>`;
    }
    const first = convVideoFormats[0];
    const selStyle = 'font-size:12px;padding:3px 6px;background:var(--surface2);color:var(--text);border:1px solid var(--border);border-radius:4px;';
    const containerOpts = convVideoFormats.map(c =>
        `<option value="${c.name}">${c.name}</option>`
    ).join('');
    const codecOpts = first.codecs.map(codec => {
        const label = convCodecLabels[codec] || codec.toUpperCase();
        const disabled = codec === 'av1' && !convAv1Available;
        return `<option value="${codec}" ${disabled ? 'disabled' : ''}>${label}${disabled ? ' (N/A)' : ''}</option>`;
    }).join('');
    const extOpts = first.extensions.map(ext => `<option value="${ext}">${ext}</option>`).join('');
    return `<div class="conv-video-format-group">
        <select class="conv-vid-container" onchange="convOnRowContainerChange(this)" style="${selStyle}">${containerOpts}</select>
        <select class="conv-vid-codec" style="${selStyle}">${codecOpts}</select>
        <select class="conv-vid-ext" style="${selStyle}">${extOpts}</select>
    </div>`;
}

// Thumbnail lazy-loading helper
let lazyThumbnailObserver = null;

function initLazyThumbnailObserver() {
    if (lazyThumbnailObserver) {
        return;
    }
    if (!('IntersectionObserver' in window)) {
        return;
    }

    lazyThumbnailObserver = new IntersectionObserver((entries) => {
        for (const entry of entries) {
            if (!entry.isIntersecting) {
                continue;
            }
            const img = entry.target;
            const dataSrc = img.dataset.src;
            if (dataSrc) {
                img.src = dataSrc;
                img.removeAttribute('data-src');
            }
            lazyThumbnailObserver.unobserve(img);
        }
    }, {
        rootMargin: '400px',
        threshold: 0.01,
    });
}

function observeThumbnails(container) {
    if (!lazyThumbnailObserver) {
        initLazyThumbnailObserver();
    }
    if (!lazyThumbnailObserver) {
        return;
    }
    const images = container.querySelectorAll('img[data-src]');
    images.forEach(img => lazyThumbnailObserver.observe(img));
}

// ── SSE Stream Helper ────────────────────────────────────────────────
function connectJobSSE(url, callbacks = {}) {
    const es = new EventSource(url);
    es.onmessage = (event) => {
        try {
            const data = JSON.parse(event.data);
            if (callbacks.onMessage) callbacks.onMessage(data);
            if (data.status === 'done') {
                es.close();
                if (callbacks.onDone) callbacks.onDone(data);
            } else if (data.status === 'cancelled') {
                es.close();
                if (callbacks.onCancelled) callbacks.onCancelled(data);
            } else if (data.status === 'failed' || data.phase === 'error') {
                es.close();
                if (callbacks.onFailed) callbacks.onFailed(data);
            }
        } catch (e) {
            console.error('SSE parse error:', e);
        }
    };
    es.onerror = (err) => {
        if (callbacks.onError) callbacks.onError(err);
    };
    return es;
}

// Dupfinder state
let dupJobId = null;
let dupPollTimer = null;
let dupEventSource = null;
let dupAllGroups = [];
let dupSelectedFiles = new Set();
let dupRunning = false;

// Organizer state
let orgJobId = null;
let orgPollTimer = null;
let orgEventSource = null;
let orgRunning = false;

// Converter convert state
let convConvertJobId = null;
let convEventSource = null;

// =====================================================================
// Tabs
// =====================================================================

function switchTab(tab) {
    activeTab = tab;
    document.querySelectorAll('.tab-btn').forEach(b => {
        b.classList.toggle('active', b.dataset.tab === tab);
    });
    document.querySelectorAll('.tab-content').forEach(c => {
        c.classList.toggle('active', c.id === 'tab-' + tab);
    });
}

// =====================================================================
// Shared: In-page Folder Browser
// =====================================================================

const _browserPrefixes = {
    converter: 'conv', dupfinder: 'dup',
    organizer: 'org',
};

function _bp(tab) { return _browserPrefixes[tab] || tab; }

function _setLastFolder(path) {
    lastSelectedFolder = path;
    try {
        localStorage.setItem('morphic_last_folder', path);
    } catch (e) {
        // ignore
    }
}

function _storeFolder(tab, path) {
    _setLastFolder(path);
    try {
        localStorage.setItem(`morphic_folder_${tab}`, path);
    } catch (e) {
        // Ignore storage errors in privacy modes
    }
}

function _loadFolder(tab) {
    try {
        return localStorage.getItem(`morphic_folder_${tab}`) || '';
    } catch (e) {
        return '';
    }
}

function _loadLastFolder() {
    try {
        return localStorage.getItem('morphic_last_folder') || '';
    } catch (e) {
        return '';
    }
}


function loadFolderPreferences() {
    lastSelectedFolder = _loadLastFolder();
    const tabs = ['converter', 'dupfinder', 'organizer'];
    for (const tab of tabs) {
        const input = document.getElementById(_bp(tab) + 'Folder');
        if (!input) continue;

        let saved = _loadFolder(tab);
        if (!saved) {
            saved = lastSelectedFolder || '';
        }

        if (saved) {
            input.value = saved;
            _setLastFolder(saved);
        }

        let indicator = input.closest('.form-group')?.querySelector('.folder-saved-indicator');
        if (!indicator && input.closest('.form-group')) {
            indicator = document.createElement('span');
            indicator.className = 'folder-saved-indicator';
            indicator.style.marginLeft = '10px';
            indicator.style.fontSize = '12px';
            indicator.style.color = 'var(--success, #06c)';
            input.closest('.form-group').appendChild(indicator);
        }

        if (indicator) {
            indicator.textContent = saved ? '✅ saved' : '';
        }

        input.addEventListener('input', () => {
            const value = input.value.trim();
            _storeFolder(tab, value);
            if (indicator) {
                indicator.textContent = value ? '✅ saved' : '';
            }
        });
    }
}

function toggleBrowser(tab) {
    const browser = document.getElementById(_bp(tab) + 'Browser');
    if (browser.classList.contains('open')) {
        browser.classList.remove('open');
    } else {
        const input = document.getElementById(_bp(tab) + 'Folder');
        browseTo(input.value.trim() || '~', tab);
    }
}

async function openNativeFolderExplorer(tab) {
    const input = document.getElementById(_bp(tab) + 'Folder');
    const initialDir = input?.value.trim() || lastSelectedFolder || '~';

    try {
        const resp = await fetch('/api/browse/native', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ initial_dir: initialDir }),
        });
        const data = await resp.json();

        if (data.available === false) {
            // No native dialog tool — fall back to in-page browser
            browseTo(initialDir, tab);
            return;
        }
        if (data.folder) {
            if (input) {
                input.value = data.folder;
                _storeFolder(tab, data.folder);
            }
            showToast('Folder selected: ' + data.folder, 'success');
        } else {
            showToast('Native folder dialog was cancelled', 'info');
        }
    } catch (e) {
        showToast('Native folder open failed: ' + e.message, 'error');
    }
}

function closeBrowser(tab) {
    document.getElementById(_bp(tab) + 'Browser').classList.remove('open');
}

function selectBrowserFolder(tab) {
    closeBrowser(tab);
    showToast('Folder selected', 'success');
}

async function browseTo(path, tab) {
    try {
        const resp = await fetch(`/api/browse?path=${encodeURIComponent(path)}`);
        const data = await resp.json();
        if (data.error) { showToast(data.error, 'error'); return; }

        const prefix = _bp(tab);
        const input = document.getElementById(prefix + 'Folder');
        input.value = data.current;
        _storeFolder(tab, data.current);

        const browser = document.getElementById(prefix + 'Browser');
        const fbPath = browser.querySelector('.fb-path');
        fbPath.textContent = data.current;

        const parentEl = browser.querySelector('.fb-parent');
        if (data.parent) {
            parentEl.style.display = 'flex';
            parentEl.onclick = () => browseTo(data.parent, tab);
        } else {
            parentEl.style.display = 'none';
        }

        const entriesEl = browser.querySelector('.fb-entries');
        entriesEl.innerHTML = '';
        for (const entry of data.entries) {
            const div = document.createElement('div');
            div.className = 'fb-item';
            div.innerHTML = `<span class="icon">📁</span> ${escapeHtml(entry.name)}`;
            div.onclick = () => browseTo(entry.path, tab);
            entriesEl.appendChild(div);
        }

        browser.classList.add('open');
    } catch (e) {
        showToast('Failed to browse: ' + e.message, 'error');
    }
}

// =====================================================================
// Converter: Scan
// =====================================================================

async function convScan() {
    // Toggle: if scan already running, abort it
    if (convScanning) {
        if (convScanController) convScanController.abort();
        return;
    }

    const folder = document.getElementById('convFolder').value.trim();
    if (!folder) { showToast('Enter a folder path', 'error'); return; }
    _storeFolder('converter', folder);
    convFileResults = new Map();

    const includeSubfolders = document.getElementById('convSubfolders').checked;
    const filterType = document.getElementById('convFilterType').value;

    convScanController = new AbortController();
    convSetScanStopMode();
    document.getElementById('convResults').style.display = 'none';

    try {
        const resp = await fetch('/api/converter/scan', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                folder, include_subfolders: includeSubfolders, filter_type: filterType,
            }),
            signal: convScanController.signal,
        });
        const data = await resp.json();
        if (data.error) { showToast(data.error, 'error'); convRestoreScanBtn(); return; }

        convScanData = data;
        convFilterType = filterType;
        convFilterExt = null;
        convSelectedFiles = [];
        convRestoreScanBtn();
        renderConvResults();
    } catch (e) {
        if (e.name === 'AbortError') {
            convShowScanInterrupted();
        } else {
            showToast('Scan failed: ' + e.message, 'error');
        }
        convRestoreScanBtn();
    }
}

function convSetScanStopMode() {
    convScanning = true;
    const btn = document.getElementById('convScanBtn');
    btn.textContent = '⏹ Stop Scan';
    btn.classList.remove('btn-primary');
    btn.classList.add('btn-stop');
    btn.disabled = false;

    // Show scan progress panel with elapsed timer
    let elapsed = 0;
    document.getElementById('convScanElapsed').textContent = '0s';
    document.getElementById('convScanProgressMsg').textContent = 'Walking folder tree...';
    document.getElementById('convScanProgress').classList.add('active');
    convScanElapsedTimer = setInterval(() => {
        elapsed++;
        document.getElementById('convScanElapsed').textContent = formatDuration(elapsed);
    }, 1000);
}

function convRestoreScanBtn() {
    convScanning = false;
    convScanController = null;
    clearInterval(convScanElapsedTimer);
    convScanElapsedTimer = null;
    document.getElementById('convScanProgress').classList.remove('active');
    const btn = document.getElementById('convScanBtn');
    btn.disabled = false;
    btn.textContent = '🔍 Scan Folder';
    btn.classList.remove('btn-stop');
    btn.classList.add('btn-primary');
}

function convShowScanInterrupted() {
    document.getElementById('convResults').style.display = 'block';
    document.getElementById('convSummary').innerHTML =
        '<div class="scan-interrupted">⏹ Scan was interrupted — no results to display.</div>';
    document.getElementById('convFileTable').innerHTML = '';
    document.getElementById('convBulkBar').style.display = 'none';
}

function renderConvResults() {
    if (!convScanData) return;
    const section = document.getElementById('convResults');
    section.style.display = 'block';

    // Summary
    const summary = document.getElementById('convSummary');
    const entries = Object.entries(convScanData.summary);
    if (entries.length === 0) {
        summary.innerHTML = '<div class="empty-state"><div class="icon">📂</div><h3>No media files found</h3></div>';
        document.getElementById('convFileTable').innerHTML = '';
        return;
    }

    let html = `<strong>${convScanData.total}</strong> files found in <code>${escapeHtml(convScanData.folder)}</code>`;
    html += '<div class="filter-pills">';
    html += `<span class="filter-pill ${!convFilterExt ? 'active' : ''}" onclick="convSetExtFilter(null)">All</span>`;
    for (const [ext, count] of entries) {
        const isActive = convFilterExt === ext;
        html += `<span class="filter-pill ${isActive ? 'active' : ''}" onclick="convSetExtFilter('${ext}')">${ext} (${count})</span>`;
    }
    html += '</div>';
    summary.innerHTML = html;

    // File table
    let files = convScanData.files;
    if (convFilterExt) {
        files = files.filter(f => f.ext === convFilterExt);
    }

    const table = document.getElementById('convFileTable');
    if (files.length === 0) {
        table.innerHTML = '<p style="color:var(--text-dim);padding:20px;">No files match current filter.</p>';
        return;
    }

    let thtml = `<table class="file-table">
        <thead><tr>
            <th style="width:50px"></th>
            <th><label class="checkbox-label"><input type="checkbox" id="convSelectAll" onchange="convToggleAll(this.checked)"> File</label></th>
            <th>Type</th>
            <th>Size</th>
            <th>Result</th>
            <th>Convert to</th>
            <th></th>
        </tr></thead><tbody>`;

    for (const f of files) {
        const thumbUrl = `/api/thumbnail?path=${encodeURIComponent(f.path)}`;
        const displayName = showFullPaths ? f.path : f.name;
        const result = convFileResults.get(f.path);
        const hasError = result?.status === 'error';

        let resultCell = '<td></td>';
        if (result) {
            if (result.status === 'converting') {
                resultCell = '<td><span class="conv-result"><span class="result-converting">Converting…</span></span></td>';
            } else if (result.status === 'ok') {
                const pct = result.original_size > 0
                    ? Math.round((1 - result.new_size / result.original_size) * 100) : 0;
                const sign = pct >= 0 ? '−' : '+';
                resultCell = `<td><span class="conv-result"><span class="status-ok">✓</span> <span class="size-change">${result.original_size_fmt} → ${result.new_size_fmt} (${sign}${Math.abs(pct)}%)</span></span></td>`;
            } else if (result.status === 'error') {
                const short = escapeHtml((result.error || 'unknown').slice(0, 80));
                resultCell = `<td><span class="conv-result"><span class="status-err" title="${escapeAttr(result.error || '')}">✗ ${short}</span></span></td>`;
            }
        }

        thtml += `<tr data-path="${escapeAttr(f.path)}" class="${hasError ? 'failed-file' : ''}">
            <td><img data-src="${thumbUrl}" src="data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///ywAAAAAAQABAAACAUwAOw==" class="file-thumb" loading="lazy" decoding="async" onclick="openPreview('${escapeAttr(f.path)}', '${f.type}')" onerror="this.style.display='none'" /></td>
            <td>
                <label class="checkbox-label">
                    <input type="checkbox" class="conv-check" value="${escapeAttr(f.path)}" onchange="convUpdateSelection()">
                    <span class="file-name" title="${escapeAttr(f.path)}" onclick="copyPath('${escapeAttr(f.path)}')">${escapeHtml(displayName)}</span>
                </label>
            </td>
            <td>${f.ext}</td>
            <td>${formatBytes(f.size)}</td>
            ${resultCell}
            <td>
                ${f.type === 'video'
                    ? convBuildVideoSelectsHtml()
                    : `<select class="conv-target" style="font-size:12px;padding:4px 8px;background:var(--surface2);color:var(--text);border:1px solid var(--border);border-radius:4px;">
                    ${(f.targets || []).map(t => `<option value="${t}">${t}</option>`).join('')}
                </select>`}
            </td>
            <td>
                <button class="btn btn-sm btn-ghost" onclick="convConvertSingle('${escapeAttr(f.path)}', this)">Convert</button>
                ${hasError ? `<button class="btn btn-sm btn-warning" onclick="convRetrySingle('${escapeAttr(f.path)}', this)">Retry</button>` : ''}
                <button class="btn btn-sm btn-ghost" style="color:var(--danger)" onclick="convDeleteSingle('${escapeAttr(f.path)}')">✕</button>
            </td>
        </tr>`;
    }

    thtml += '</tbody></table>';
    table.innerHTML = thtml;
    observeThumbnails(table);
    convUpdateBatchDropdowns();
}

function convSetBatchTargets(targets) {
    const select = document.getElementById('convBatchTarget');
    if (!select) return;
    const prevValue = select.value;

    const targetArray = Array.isArray(targets) ? targets.filter(Boolean) : [];
    const uniqueTargets = [...new Set(targetArray.map(t => t.toLowerCase()))].sort();

    select.innerHTML = '';
    if (uniqueTargets.length === 0) {
        select.disabled = true;
        const opt = document.createElement('option');
        opt.value = '';
        opt.textContent = 'No compatible image formats';
        opt.disabled = true;
        select.appendChild(opt);
        return;
    }

    select.disabled = false;
    for (const target of uniqueTargets) {
        const option = document.createElement('option');
        option.value = target;
        option.textContent = target;
        select.appendChild(option);
    }

    if (prevValue && uniqueTargets.includes(prevValue.toLowerCase())) {
        select.value = prevValue.toLowerCase();
    } else {
        select.value = uniqueTargets[0];
    }
}

function convGetBatchTargets() {
    const selectedFilePaths = new Set(convSelectedFiles || []);
    const modeSelect = document.getElementById('convBatchMode');
    if (modeSelect) {
        convBatchMode = modeSelect.value || 'intersection';
        localStorage.setItem('convBatchMode', convBatchMode);
    }

    const modeHint = document.getElementById('convBatchModeHint');
    if (modeHint) {
        modeHint.textContent = convBatchMode === 'intersection'
            ? 'Image formats: common to all selected images'
            : 'Image formats: supported by any selected image';
    }

    // Only image files contribute to the image format dropdown
    if (!convScanData || !convScanData.files || convScanData.files.length === 0) {
        return [...new Set(convAvailableTargets)].sort();
    }

    const imageFiles = convScanData.files.filter(f =>
        f.type === 'image' && (selectedFilePaths.size === 0 || selectedFilePaths.has(f.path))
    );

    if (imageFiles.length === 0) {
        return [...new Set(convAvailableTargets)].sort();
    }

    const fileTargets = imageFiles.map(f => new Set((f.targets || []).map(t => t.toLowerCase())));

    if (convBatchMode === 'intersection') {
        let intersection = new Set(fileTargets[0]);
        for (let i = 1; i < fileTargets.length; i++) {
            intersection = new Set([...intersection].filter(t => fileTargets[i].has(t)));
        }
        return [...intersection].sort();
    }

    const union = new Set();
    for (const targetSet of fileTargets) {
        for (const t of targetSet) union.add(t);
    }
    return [...union].sort();
}

function convUpdateBatchTargets() {
    convUpdateBatchDropdowns();
}

function convSetExtFilter(ext) {
    convFilterExt = ext;
    renderConvResults();
}

function convToggleAll(checked) {
    document.querySelectorAll('.conv-check').forEach(cb => cb.checked = checked);
    convUpdateSelection();
}

function convUpdateSelection() {
    convSelectedFiles = [];
    document.querySelectorAll('.conv-check:checked').forEach(cb => {
        convSelectedFiles.push(cb.value);
    });

    const bar = document.getElementById('convBulkBar');
    if (convSelectedFiles.length > 0) {
        bar.style.display = 'flex';
        document.getElementById('convBulkCount').textContent = convSelectedFiles.length;
    } else {
        bar.style.display = 'none';
    }

    convUpdateBatchDropdowns();
}

function toggleFullPaths() {
    showFullPaths = !showFullPaths;
    renderConvResults();
}

// =====================================================================
// Converter: Convert
// =====================================================================

async function convConvertSingle(filePath, btnEl) {
    const row = btnEl.closest('tr');
    const deleteOrig = document.getElementById('convDeleteOrig').checked;

    let targetExt, codec;
    const videoContainer = row.querySelector('.conv-vid-container');
    if (videoContainer) {
        targetExt = row.querySelector('.conv-vid-ext').value;
        codec = row.querySelector('.conv-vid-codec').value;
    } else {
        targetExt = row.querySelector('.conv-target').value;
    }

    btnEl.disabled = true;
    btnEl.textContent = '...';

    try {
        const body = {
            files: [filePath],
            target_ext: targetExt,
            delete_original: deleteOrig,
            av1_crf: convAv1Crf,
        };
        if (codec) body.codec = codec;

        const resp = await fetch('/api/converter/convert', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body),
        });
        const data = await resp.json();
        if (data.job_id) {
            await convWaitForJob(data.job_id);
        }
    } catch (e) {
        convFileResults.set(filePath, { status: 'error', error: e.message });
        renderConvResults();
    } finally {
        btnEl.disabled = false;
        btnEl.textContent = 'Convert';
    }
}

async function convConvertBatch() {
    if (convSelectedFiles.length === 0) return;
    const { videos, images } = convGetSelectedByType();
    const deleteOrig = document.getElementById('convDeleteOrig').checked;

    document.getElementById('convBatchBtn').disabled = true;

    try {
        if (videos.length > 0) {
            const targetExt = document.getElementById('convBatchExt').value;
            const codec = document.getElementById('convBatchCodec').value;
            if (!targetExt || !codec) {
                showToast('Please select a video format', 'error');
                return;
            }
            const resp = await fetch('/api/converter/convert', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    files: videos,
                    target_ext: targetExt,
                    codec,
                    delete_original: deleteOrig,
                    av1_crf: convAv1Crf,
                }),
            });
            const data = await resp.json();
            if (data.job_id) {
                convConvertJobId = data.job_id;
                convShowProgress();
                await convStreamProgress(data.job_id);
            }
        }

        if (images.length > 0) {
            const targetExt = document.getElementById('convBatchTarget').value;
            if (!targetExt) {
                showToast('No valid image target format selected', 'error');
            } else {
                const resp = await fetch('/api/converter/convert', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({
                        files: images,
                        target_ext: targetExt,
                        delete_original: deleteOrig,
                    }),
                });
                const data = await resp.json();
                if (data.job_id) {
                    convConvertJobId = data.job_id;
                    convShowProgress();
                    await convStreamProgress(data.job_id);
                }
            }
        }
    } catch (e) {
        showToast('Batch convert failed: ' + e.message, 'error');
    } finally {
        document.getElementById('convBatchBtn').disabled = false;
    }
}

function convShowProgress() {
    const stopBtn = document.getElementById('convStopBtn');
    if (stopBtn) { stopBtn.disabled = false; stopBtn.textContent = '⏹ Stop'; }
    const p = document.getElementById('convProgress');
    p.classList.add('active');
    p.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
}

function convStreamProgress(jobId) {
    return new Promise(resolve => {
        if (convEventSource) {
            convEventSource.close();
            convEventSource = null;
        }

        convEventSource = connectJobSSE(`/api/converter/progress/${jobId}/stream`, {
            onMessage: (data) => {
                const pct = data.total > 0 ? Math.round((data.completed / data.total) * 100) : 0;
                document.getElementById('convProgressBar').style.width = pct + '%';
                document.getElementById('convProgressPct').textContent = pct + '%';
                document.getElementById('convProgressMsg').textContent =
                    data.current_file ? `Converting: ${data.current_file}` : (data.status === 'done' ? 'Completed' : 'Processing...');
                _convSyncResults(data);
            },
            onDone: () => {
                document.getElementById('convProgress').classList.remove('active');
                convConvertJobId = null;
                convEventSource = null;
                resolve('done');
            },
            onCancelled: () => {
                document.getElementById('convProgress').classList.remove('active');
                convConvertJobId = null;
                convEventSource = null;
                showToast('Conversion was stopped', 'warning');
                resolve('cancelled');
            },
            onFailed: (data) => {
                document.getElementById('convProgress').classList.remove('active');
                convConvertJobId = null;
                convEventSource = null;
                showToast('Conversion failed: ' + (data.error || 'Unknown error'), 'error');
                resolve('failed');
            },
            onError: () => {
                // Keep resolving on fallback or close
            }
        });
    });
}

function _convSyncResults(data) {
    const table = document.getElementById('convResultsTable');
    for (const r of data.results || []) {
        const existing = convFileResults.get(r.source);
        if (existing && existing.status !== 'converting') continue; // already finalised
        if (r.status === 'ok') {
            convFileResults.set(r.source, {
                status: 'ok',
                original_size_fmt: r.original_size_fmt,
                new_size_fmt: r.new_size_fmt,
                original_size: r.original_size,
                new_size: r.new_size,
            });
        } else {
            convFileResults.set(r.source, { status: 'error', error: r.error || 'unknown' });
        }
        if (table) updateConvTableRow(table, r.source);
    }

    if (data.current_file) {
        if (!convFileResults.has(data.current_file) || convFileResults.get(data.current_file)?.status === 'converting') {
            convFileResults.set(data.current_file, { status: 'converting' });
            if (table) updateConvTableRow(table, data.current_file);
        }
    }
}

function updateConvTableRow(table, filePath) {
    if (!table) return;
    const row = table.querySelector(`tr[data-path="${escapeAttr(filePath)}"]`);
    if (!row) return;

    const result = convFileResults.get(filePath);
    const resultCell = row.children[4];
    if (!resultCell) return;

    if (!result) {
        resultCell.innerHTML = '';
        row.classList.remove('failed-file');
        return;
    }

    if (result.status === 'converting') {
        resultCell.innerHTML = '<span class="conv-result"><span class="result-converting">Converting…</span></span>';
        row.classList.remove('failed-file');
    } else if (result.status === 'ok') {
        const pct = result.original_size > 0
            ? Math.round((1 - result.new_size / result.original_size) * 100) : 0;
        const sign = pct >= 0 ? '−' : '+';
        resultCell.innerHTML = `<span class="conv-result"><span class="status-ok">✓</span> <span class="size-change">${result.original_size_fmt} → ${result.new_size_fmt} (${sign}${Math.abs(pct)}%)</span></span>`;
        row.classList.remove('failed-file');
    } else if (result.status === 'error') {
        const short = escapeHtml((result.error || 'unknown').slice(0, 80));
        resultCell.innerHTML = `<span class="conv-result"><span class="status-err" title="${escapeAttr(result.error || '')}">✗ ${short}</span></span>`;
        row.classList.add('failed-file');
    }
}

async function convStopConvert() {
    if (!convConvertJobId) return;
    const btn = document.getElementById('convStopBtn');
    if (btn) {
        btn.disabled = true;
        btn.textContent = 'Stopping…';
    }
    try {
        await fetch(`/api/converter/progress/${convConvertJobId}/cancel`, { method: 'POST' });
    } catch (e) { /* ignore */ }
    if (convEventSource) {
        convEventSource.close();
        convEventSource = null;
    }
}

async function convWaitForJob(jobId) {
    await convStreamProgress(jobId);
}

async function convRetrySingle(filePath, btnEl) {
    const row = btnEl.closest('tr');
    const deleteOrig = document.getElementById('convDeleteOrig').checked;

    let targetExt, codec;
    const videoContainer = row.querySelector('.conv-vid-container');
    if (videoContainer) {
        targetExt = row.querySelector('.conv-vid-ext').value;
        codec = row.querySelector('.conv-vid-codec').value;
    } else {
        targetExt = row.querySelector('.conv-target')?.value;
    }

    if (!targetExt) {
        showToast('No target selected for retry', 'error');
        return;
    }

    btnEl.disabled = true;
    btnEl.textContent = 'Retrying...';

    try {
        const body = { files: [filePath], target_ext: targetExt, delete_original: deleteOrig, av1_crf: convAv1Crf };
        if (codec) body.codec = codec;

        const resp = await fetch('/api/converter/convert', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body),
        });
        const data = await resp.json();
        if (data.job_id) {
            await convWaitForJob(data.job_id);
        }
    } catch (e) {
        convFileResults.set(filePath, { status: 'error', error: e.message });
        renderConvResults();
    } finally {
        btnEl.disabled = false;
        btnEl.textContent = 'Retry';
    }
}

function convShowConversionResults(data) {
    // results are already synced live via _convSyncResults during polling;
    // this is kept as a no-op hook for any future post-completion logic.
}

async function convDeleteSingle(filePath) {
    if (!confirm('Delete this file permanently?')) return;
    try {
        const resp = await fetch('/api/converter/delete', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ files: [filePath] }),
        });
        const data = await resp.json();
        const r = data.results[0];
        if (r.status === 'deleted') {
            showToast('Deleted', 'success');
            // Remove from scan data
            if (convScanData) {
                convScanData.files = convScanData.files.filter(f => f.path !== filePath);
                convScanData.total = convScanData.files.length;
            }
            renderConvResults();
        } else {
            showToast('Delete failed: ' + (r.status), 'error');
        }
    } catch (e) {
        showToast('Delete failed: ' + e.message, 'error');
    }
}

async function convDeleteBatch() {
    if (convSelectedFiles.length === 0) return;
    if (!confirm(`Permanently delete ${convSelectedFiles.length} file(s)?`)) return;
    try {
        const resp = await fetch('/api/converter/delete', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ files: convSelectedFiles }),
        });
        const data = await resp.json();
        const deleted = data.results.filter(r => r.status === 'deleted');
        const failed = data.results.filter(r => r.status !== 'deleted');

        if (deleted.length > 0) {
            showToast(`Deleted ${deleted.length} file(s), freed ${data.total_freed_formatted}`, 'success');
            if (convScanData) {
                const deletedPaths = new Set(deleted.map(r => r.path));
                convScanData.files = convScanData.files.filter(f => !deletedPaths.has(f.path));
                convScanData.total = convScanData.files.length;
                // Recalculate summary
                const newSummary = {};
                for (const f of convScanData.files) {
                    newSummary[f.ext] = (newSummary[f.ext] || 0) + 1;
                }
                convScanData.summary = newSummary;
            }
        }
        if (failed.length > 0) {
            showToast(`Failed to delete ${failed.length} file(s)`, 'error');
        }

        convSelectedFiles = [];
        renderConvResults();
    } catch (e) {
        showToast('Delete failed: ' + e.message, 'error');
    }
}

function copyPath(path) {
    navigator.clipboard.writeText(path).then(() => {
        showToast('Path copied', 'success');
    });
}

// =====================================================================
// Dupfinder: Scan
// =====================================================================

async function dupStartScan() {
    // If a scan is already running, act as stop
    if (dupRunning) {
        await dupStopScan();
        return;
    }

    const folder = document.getElementById('dupFolder').value.trim();
    if (!folder) { showToast('Enter a folder path', 'error'); return; }
    _storeFolder('dupfinder', folder);

    const scanType = document.getElementById('dupScanType').value;
    const imgThreshold = parseInt(document.getElementById('dupImgThreshold').value) / 100;
    const vidThreshold = parseInt(document.getElementById('dupVidThreshold').value) / 100;

    dupSetStopMode();
    document.getElementById('dupResults').classList.remove('active');
    const p = document.getElementById('dupProgress');
    p.classList.add('active');
    p.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
    document.getElementById('dupProgressBar').style.width = '0%';
    document.getElementById('dupProgressPct').textContent = '0%';
    document.getElementById('dupProgressMsg').textContent = 'Starting scan...';

    try {
        const resp = await fetch('/api/dupfinder/scan', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                folder, type: scanType,
                image_threshold: imgThreshold,
                video_threshold: vidThreshold,
            }),
        });
        const data = await resp.json();
        if (data.error) {
            showToast(data.error, 'error');
            dupRestoreBtn();
            return;
        }
        dupJobId = data.job_id;
        dupStreamProgress(dupJobId);
    } catch (e) {
        showToast('Scan failed: ' + e.message, 'error');
        dupRestoreBtn();
    }
}

async function dupStopScan() {
    if (!dupJobId) { dupRestoreBtn(); return; }
    const btn = document.getElementById('dupScanBtn');
    if (btn) {
        btn.disabled = true;
        btn.textContent = 'Stopping…';
    }
    try {
        await fetch(`/api/dupfinder/scan/${dupJobId}/cancel`, { method: 'POST' });
    } catch (e) { /* ignore */ }
    if (dupEventSource) {
        dupEventSource.close();
        dupEventSource = null;
    }
    dupRestoreBtn();
}

function dupSetStopMode() {
    dupRunning = true;
    const btn = document.getElementById('dupScanBtn');
    btn.textContent = '⏹ Stop Scan';
    btn.classList.remove('btn-primary');
    btn.classList.add('btn-stop');
    btn.disabled = false;
}

function dupRestoreBtn() {
    dupRunning = false;
    dupJobId = null;
    const btn = document.getElementById('dupScanBtn');
    if (btn) {
        btn.disabled = false;
        btn.textContent = '🔍 Start Scan';
        btn.classList.remove('btn-stop');
        btn.classList.add('btn-primary');
    }
}

function dupShowInterrupted() {
    document.getElementById('dupResults').classList.add('active');
    document.getElementById('dupGroups').innerHTML = '';
    document.getElementById('dupNoResults').style.display = 'none';
    document.getElementById('dupTitle').textContent = '';
    document.getElementById('dupSummary').innerHTML =
        '<div class="scan-interrupted">⏹ Scan was interrupted — no results to display.</div>';
}

function dupStreamProgress(jobId) {
    if (dupEventSource) {
        dupEventSource.close();
        dupEventSource = null;
    }

    dupEventSource = connectJobSSE(`/api/dupfinder/scan/${jobId}/stream`, {
        onMessage: (data) => {
            const pct = Math.round((data.progress || 0) * 100);
            document.getElementById('dupProgressBar').style.width = pct + '%';
            document.getElementById('dupProgressPct').textContent = pct + '%';
            document.getElementById('dupProgressMsg').textContent = data.message || '';
            document.getElementById('dupProgressElapsed').textContent = formatDuration(data.elapsed_seconds || 0);
        },
        onDone: async () => {
            dupEventSource = null;
            await dupLoadResults();
        },
        onCancelled: () => {
            dupEventSource = null;
            document.getElementById('dupProgress').classList.remove('active');
            dupShowInterrupted();
            dupRestoreBtn();
        },
        onFailed: (data) => {
            dupEventSource = null;
            showToast('Scan failed: ' + (data.error || 'Unknown error'), 'error');
            document.getElementById('dupProgress').classList.remove('active');
            dupRestoreBtn();
        }
    });
}

async function dupLoadResults() {
    try {
        const resp = await fetch(`/api/dupfinder/scan/${dupJobId}/results`);
        const data = await resp.json();

        dupAllGroups = [];
        let idx = 0;
        for (const g of (data.image_groups || [])) dupAllGroups.push({ index: idx++, type: 'image', items: g });
        for (const g of (data.video_groups || [])) dupAllGroups.push({ index: idx++, type: 'video', items: g });

        dupRenderResults(data);
        document.getElementById('dupProgress').classList.remove('active');
        document.getElementById('dupResults').classList.add('active');
        dupRestoreBtn();

        document.getElementById('dupNoResults').style.display =
            dupAllGroups.length === 0 ? 'block' : 'none';
    } catch (e) {
        showToast('Failed to load results', 'error');
        dupRestoreBtn();
    }
}

function dupResetUI() {
    dupRestoreBtn();
}

// =====================================================================
// Dupfinder: Render
// =====================================================================

function dupRenderResults(data) {
    const totalGroups = dupAllGroups.length;
    const totalDups = dupAllGroups.reduce((s, g) => s + g.items.length - 1, 0);

    document.getElementById('dupSummary').innerHTML = `
        <div class="stat-card"><div class="stat-value">${totalGroups}</div><div class="stat-label">Duplicate Groups</div></div>
        <div class="stat-card"><div class="stat-value">${totalDups}</div><div class="stat-label">Duplicate Files</div></div>
        <div class="stat-card savings"><div class="stat-value">${data.space_savings_formatted}</div><div class="stat-label">Potential Savings</div></div>
    `;

    document.getElementById('dupTitle').textContent =
        `${totalGroups} Duplicate Group${totalGroups !== 1 ? 's' : ''} Found`;

    const container = document.getElementById('dupGroups');
    container.innerHTML = '';
    for (const group of dupAllGroups) {
        container.appendChild(dupCreateGroupCard(group));
    }
    observeThumbnails(container);
}

function dupCreateGroupCard(group) {
    const div = document.createElement('div');
    div.className = 'group-card';
    div.dataset.groupIndex = group.index;

    const typeLabel = group.type === 'image' ? 'Image' : 'Video';
    const badgeClass = group.type === 'video' ? 'group-badge video' : 'group-badge';

    div.innerHTML = `
        <div class="group-header" onclick="dupToggleGroup(${group.index})">
            <div class="group-title">
                <span class="${badgeClass}">${typeLabel}</span>
                Group ${group.index + 1} — ${group.items.length} files
            </div>
            <span class="toggle-icon">▼</span>
        </div>
        <div class="group-body" id="dupGroupBody${group.index}">
            <div class="dup-grid">
                ${group.items.map((item, i) => dupCreateFileCard(item, i === 0, group.index)).join('')}
            </div>
        </div>
    `;
    return div;
}

function dupCreateFileCard(item, isBest) {
    const isSelected = dupSelectedFiles.has(item.path);
    const selClass = isSelected ? 'selected-for-delete' : (isBest ? 'is-best' : '');

    let thumb;
    if (item.type === 'video') {
        thumb = `<video src="/api/media?path=${encodeURIComponent(item.path)}" muted preload="metadata"></video>`;
    } else {
        thumb = `<img data-src="/api/thumbnail?path=${encodeURIComponent(item.path)}" src="data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///ywAAAAAAQABAAACAUwAOw==" alt="${escapeAttr(item.filename)}" loading="lazy" />`;
    }

    const badges = [];
    if (isBest) {
        badges.push(`<span class="best-badge">★ BEST</span>`);
    } else {
        badges.push(`<span class="sim-badge">${item.similarity}%</span>`);
    }

    const meta = [];
    meta.push(item.resolution);
    meta.push(item.file_size_formatted);
    if (item.duration_formatted) meta.push(item.duration_formatted);
    if (item.format) meta.push(item.format);

    return `
        <div class="file-card ${selClass}" data-path="${escapeAttr(item.path)}" data-size="${item.file_size}">
            <div class="thumb-container" onclick="openPreview('${escapeAttr(item.path)}', '${item.type}')">
                ${thumb}
                <div class="badge-row">${badges.join('')}</div>
                <div class="thumb-overlay">Click to preview</div>
            </div>
            <div class="file-info">
                <div class="file-name" title="${escapeAttr(item.path)}">${escapeHtml(item.filename)}</div>
                <div class="file-dir" title="${escapeAttr(item.directory)}">${escapeHtml(item.directory)}</div>
                <div class="file-meta">${meta.join(' · ')}</div>
            </div>
            <div class="file-actions">
                ${isBest
                    ? `<button class="btn ${isSelected ? 'btn-ghost' : 'btn-ghost'} btn-sm" style="flex:1;border-color:${isSelected ? 'var(--danger)' : 'var(--success)'}"
                         onclick="dupToggleFile('${escapeAttr(item.path)}')">
                         ${isSelected ? '↩ Unselect (★ best)' : '★ Recommended to keep'}
                       </button>`
                    : `<button class="btn ${isSelected ? 'btn-ghost' : 'btn-danger'} btn-sm" style="flex:1"
                         onclick="dupToggleFile('${escapeAttr(item.path)}')">
                         ${isSelected ? '↩ Unselect' : '🗑 Select to delete'}
                       </button>`
                }
            </div>
        </div>
    `;
}

// =====================================================================
// Dupfinder: Selection & Deletion
// =====================================================================

function dupToggleGroup(index) {
    const body = document.getElementById(`dupGroupBody${index}`);
    body.classList.toggle('collapsed');
    const icon = body.closest('.group-card').querySelector('.toggle-icon');
    icon.textContent = body.classList.contains('collapsed') ? '▶' : '▼';
}

function dupExpandAll() {
    document.querySelectorAll('#dupGroups .group-body').forEach(b => b.classList.remove('collapsed'));
    document.querySelectorAll('#dupGroups .toggle-icon').forEach(i => i.textContent = '▼');
}

function dupCollapseAll() {
    document.querySelectorAll('#dupGroups .group-body').forEach(b => b.classList.add('collapsed'));
    document.querySelectorAll('#dupGroups .toggle-icon').forEach(i => i.textContent = '▶');
}

function dupToggleFile(path) {
    if (dupSelectedFiles.has(path)) dupSelectedFiles.delete(path);
    else dupSelectedFiles.add(path);
    dupUpdateSelectionUI();
}

function dupAutoSelect() {
    dupSelectedFiles.clear();
    for (const g of dupAllGroups) {
        for (let i = 1; i < g.items.length; i++) {
            dupSelectedFiles.add(g.items[i].path);
        }
    }
    dupUpdateSelectionUI();
    showToast(`Selected ${dupSelectedFiles.size} duplicate(s)`, 'success');
}

// Initialize converter formats for batch target dropdown
convLoadFormats();
loadFolderPreferences();

function dupClearSelection() {
    dupSelectedFiles.clear();
    dupUpdateSelectionUI();
}

function dupUpdateSelectionUI() {
    document.querySelectorAll('#dupGroups .file-card').forEach(card => {
        const path = card.dataset.path;
        const isSelected = dupSelectedFiles.has(path);
        card.classList.toggle('selected-for-delete', isSelected);
        card.classList.toggle('is-best', !isSelected && !!card.querySelector('.best-badge'));

        const btn = card.querySelector('.file-actions button');
        if (btn) {
            const hasBest = !!card.querySelector('.best-badge');
            if (isSelected) {
                btn.className = 'btn btn-ghost btn-sm';
                btn.style.flex = '1';
                btn.style.borderColor = hasBest ? 'var(--danger)' : '';
                btn.innerHTML = hasBest ? '↩ Unselect (★ best)' : '↩ Unselect';
            } else if (hasBest) {
                btn.className = 'btn btn-ghost btn-sm';
                btn.style.flex = '1';
                btn.style.borderColor = 'var(--success)';
                btn.innerHTML = '★ Recommended to keep';
            } else {
                btn.className = 'btn btn-danger btn-sm';
                btn.style.flex = '1';
                btn.style.borderColor = '';
                btn.innerHTML = '🗑 Select to delete';
            }
        }
    });

    const bulkBar = document.getElementById('dupBulkBar');
    if (dupSelectedFiles.size > 0) {
        bulkBar.classList.add('active');
        document.getElementById('dupBulkCount').textContent = dupSelectedFiles.size;
        let totalSize = 0;
        document.querySelectorAll('#dupGroups .file-card').forEach(card => {
            if (dupSelectedFiles.has(card.dataset.path)) {
                totalSize += parseInt(card.dataset.size) || 0;
            }
        });
        document.getElementById('dupBulkSize').textContent = formatBytes(totalSize);
    } else {
        bulkBar.classList.remove('active');
    }
}

function dupConfirmDelete() {
    if (dupSelectedFiles.size === 0) return;
    const fileList = document.getElementById('dupDeleteFileList');
    fileList.innerHTML = '';
    for (const path of dupSelectedFiles) {
        const div = document.createElement('div');
        div.textContent = path;
        fileList.appendChild(div);
    }
    document.getElementById('dupDeleteModal').classList.add('active');
}

function dupCloseModal() {
    document.getElementById('dupDeleteModal').classList.remove('active');
}

async function dupExecuteDelete() {
    const files = Array.from(dupSelectedFiles);
    const btn = document.getElementById('dupConfirmDeleteBtn');
    btn.disabled = true;
    btn.textContent = 'Deleting...';

    try {
        const resp = await fetch('/api/dupfinder/delete', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ files }),
        });
        const data = await resp.json();
        const deleted = data.results.filter(r => r.status === 'deleted');
        const failed = data.results.filter(r => r.status !== 'deleted');

        if (deleted.length > 0) showToast(`Deleted ${deleted.length} file(s), freed ${data.total_freed_formatted}`, 'success');
        if (failed.length > 0) showToast(`Failed to delete ${failed.length} file(s)`, 'error');

        for (const r of deleted) {
            dupSelectedFiles.delete(r.path);
            const card = document.querySelector(`#dupGroups .file-card[data-path="${CSS.escape(r.path)}"]`);
            if (card) card.remove();
            for (const g of dupAllGroups) {
                g.items = g.items.filter(item => item.path !== r.path);
            }
        }

        dupAllGroups = dupAllGroups.filter(g => g.items.length > 1);
        const container = document.getElementById('dupGroups');
        container.innerHTML = '';
        for (const g of dupAllGroups) container.appendChild(dupCreateGroupCard(g));
        dupUpdateSelectionUI();

        if (dupAllGroups.length === 0) {
            document.getElementById('dupNoResults').style.display = 'block';
            document.getElementById('dupTitle').textContent = 'All duplicates resolved!';
            document.getElementById('dupSummary').innerHTML = '';
        }
    } catch (e) {
        showToast('Delete failed: ' + e.message, 'error');
    } finally {
        dupCloseModal();
        btn.disabled = false;
        btn.textContent = '🗑️ Delete';
    }
}

// =====================================================================
// Organizer Tab
// =====================================================================

function orgModeChanged() {
    const mode = document.getElementById('orgMode').value;
    document.getElementById('orgSortOptions').style.display = mode === 'sort' ? 'block' : 'none';
    document.getElementById('orgRenameOptions').style.display = mode === 'rename' ? 'block' : 'none';
    document.getElementById('orgExecBtn').style.display = 'none';
    document.getElementById('orgPlanResults').style.display = 'none';
    document.getElementById('orgExecResults').style.display = 'none';
}

async function orgStartPlan() {
    // If planning is running, act as stop
    if (orgRunning) {
        await orgStopScan();
        return;
    }

    const folder = document.getElementById('orgFolder').value.trim();
    if (!folder) { showToast('Enter a folder path', 'error'); return; }
    _storeFolder('organizer', folder);

    const mode = document.getElementById('orgMode').value;
    const operation = document.getElementById('orgOperation').value;

    let body = { folder, mode, operation };
    if (mode === 'sort') {
        body.template = document.getElementById('orgTemplate').value.trim() || '{year}/{month}';
        body.destination = document.getElementById('orgDest').value.trim() || null;
    } else {
        body.template = document.getElementById('orgRenameTemplate').value.trim() || '{date}_{seq:4}';
        body.start_seq = parseInt(document.getElementById('orgStartSeq').value) || 1;
    }

    orgSetStopMode();
    document.getElementById('orgExecBtn').style.display = 'none';
    document.getElementById('orgPlanResults').style.display = 'none';
    document.getElementById('orgExecResults').style.display = 'none';
    const p = document.getElementById('orgProgress');
    p.style.display = 'block';
    p.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
    document.getElementById('orgProgressBar').style.width = '0%';
    document.getElementById('orgProgressMsg').textContent = 'Planning...';

    try {
        const resp = await fetch('/api/organizer/plan', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body),
        });
        const data = await resp.json();
        if (data.error) { showToast(data.error, 'error'); orgRestoreBtn(); return; }
        orgJobId = data.job_id;
        orgStreamStatus(orgJobId);
    } catch (e) {
        showToast('Plan failed: ' + e.message, 'error');
        document.getElementById('orgProgress').style.display = 'none';
        orgRestoreBtn();
    }
}

async function orgStopScan() {
    if (!orgJobId) { orgRestoreBtn(); return; }
    const btn = document.getElementById('orgPlanBtn');
    if (btn) {
        btn.disabled = true;
        btn.textContent = 'Stopping…';
    }
    try {
        await fetch(`/api/organizer/cancel/${orgJobId}`, { method: 'POST' });
    } catch (e) { /* ignore */ }
    if (orgEventSource) {
        orgEventSource.close();
        orgEventSource = null;
    }
    orgRestoreBtn();
}

function orgSetStopMode() {
    orgRunning = true;
    const btn = document.getElementById('orgPlanBtn');
    btn.textContent = '⏹ Stop Scan';
    btn.classList.remove('btn-primary');
    btn.classList.add('btn-stop');
    btn.disabled = false;
}

function orgRestoreBtn() {
    orgRunning = false;
    const btn = document.getElementById('orgPlanBtn');
    if (btn) {
        btn.disabled = false;
        btn.textContent = '📋 Preview Plan';
        btn.classList.remove('btn-stop');
        btn.classList.add('btn-primary');
    }
}

function orgShowInterrupted() {
    document.getElementById('orgPlanResults').style.display = 'block';
    document.getElementById('orgPlanContent').innerHTML =
        '<div class="scan-interrupted">⏹ Scan was interrupted — no results to display.</div>';
    document.getElementById('orgExecBtn').style.display = 'none';
}

function orgStreamStatus(jobId) {
    if (orgEventSource) {
        orgEventSource.close();
        orgEventSource = null;
    }

    orgEventSource = connectJobSSE(`/api/organizer/status/${jobId}/stream`, {
        onMessage: (data) => {
            const pct = Math.round((data.progress || 0) * 100);
            document.getElementById('orgProgressBar').style.width = pct + '%';
            document.getElementById('orgProgressPct').textContent = pct + '%';
            document.getElementById('orgProgressMsg').textContent =
                data.phase === 'executing' ? `Executing...` : `Planning...`;

            if (data.phase === 'planned') {
                orgRestoreBtn();
                orgRenderPlan(data);
            }
        },
        onDone: (data) => {
            orgEventSource = null;
            document.getElementById('orgProgress').style.display = 'none';
            if (data.phase === 'planned') {
                orgRestoreBtn();
                orgRenderPlan(data);
            } else {
                orgRenderExecResults(data);
            }
        },
        onCancelled: () => {
            orgEventSource = null;
            document.getElementById('orgProgress').style.display = 'none';
            orgShowInterrupted();
            orgRestoreBtn();
        },
        onFailed: (data) => {
            orgEventSource = null;
            document.getElementById('orgProgress').style.display = 'none';
            showToast('Error: ' + (data.error || 'Unknown error'), 'error');
            orgRestoreBtn();
        }
    });
}

function orgRenderPlan(data) {
    const content = document.getElementById('orgPlanContent');
    const plan = data.plan || [];
    const conflicts = data.conflicts || 0;
    const planCount = data.plan_count || plan.length;

    let html = `<div class="card-title"><span class="icon">📋</span> Plan Preview — ${planCount} file(s)</div>`;
    if (conflicts > 0) {
        html += `<div class="toast error" style="position:static;margin-bottom:12px;">⚠️ ${conflicts} conflict(s) detected — those will be skipped.</div>`;
    }

    html += '<div style="max-height:400px;overflow:auto;"><table class="exif-table"><thead><tr><th>Source</th><th>→</th><th>Destination</th></tr></thead><tbody>';
    for (const item of plan.slice(0, 200)) {
        const srcName = item.source.split('/').pop();
        const dstName = item.destination.split('/').pop();
        const conflict = item.conflict ? ' style="color:var(--danger);"' : '';
        html += `<tr${conflict}><td title="${escapeAttr(item.source)}">${escapeHtml(srcName)}</td><td>→</td><td title="${escapeAttr(item.destination)}">${escapeHtml(dstName)}</td></tr>`;
    }
    if (plan.length > 200) html += `<tr><td colspan="3" style="color:var(--text-dim);">… and ${plan.length - 200} more</td></tr>`;
    html += '</tbody></table></div>';

    content.innerHTML = html;
    document.getElementById('orgPlanResults').style.display = 'block';
    document.getElementById('orgExecBtn').style.display = 'inline-flex';
}

async function orgExecute() {
    if (!orgJobId) { showToast('No plan to execute', 'error'); return; }

    document.getElementById('orgExecBtn').disabled = true;
    const p = document.getElementById('orgProgress');
    p.style.display = 'block';
    p.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
    document.getElementById('orgProgressMsg').textContent = 'Executing...';
    document.getElementById('orgProgressBar').style.width = '0%';

    try {
        const resp = await fetch('/api/organizer/execute', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ job_id: orgJobId }),
        });
        const data = await resp.json();
        if (data.error) { showToast(data.error, 'error'); return; }
        orgStreamStatus(orgJobId);
    } catch (e) {
        showToast('Execute failed: ' + e.message, 'error');
        document.getElementById('orgProgress').style.display = 'none';
    } finally {
        document.getElementById('orgExecBtn').disabled = false;
    }
}

function orgRenderExecResults(data) {
    const container = document.getElementById('orgExecContent');
    const r = data.execution || {};
    const completed = r.completed || 0;
    const errList = r.errors || [];
    const errors = typeof errList === 'number' ? errList : errList.length;
    const skipped = r.skipped || 0;

    let html = `<div class="card">
        <div class="card-title"><span class="icon">✅</span> Execution Complete</div>
        <div class="stat-row">
            <div class="stat-card"><div class="stat-value" style="color:var(--success)">${completed}</div><div class="stat-label">Processed</div></div>
            <div class="stat-card"><div class="stat-value" style="color:var(--warning)">${skipped}</div><div class="stat-label">Skipped</div></div>
            <div class="stat-card"><div class="stat-value" style="color:var(--danger)">${errors}</div><div class="stat-label">Errors</div></div>
        </div></div>`;

    if (errors > 0) {
        html += '<div class="card"><div class="card-title" style="color:var(--danger)">⚠️ Errors</div>';
        for (const e of (r.errors || [])) {
            html += `<div class="integrity-item bad"><span class="file-name">${escapeHtml(e.file || 'unknown')}</span><span class="integrity-error">${escapeHtml(e.error || '')}</span></div>`;
        }
        html += '</div>';
    }

    container.innerHTML = html;
    document.getElementById('orgExecResults').style.display = 'block';
    document.getElementById('orgPlanResults').style.display = 'none';
    document.getElementById('orgExecBtn').style.display = 'none';
    showToast(`Done: ${completed} file(s) processed`, 'success');
}

// =====================================================================
// Shared: Preview
// =====================================================================

function openPreview(path, type) {
    const content = document.getElementById('previewContent');
    if (type === 'video') {
        content.innerHTML = `<video src="/api/media?path=${encodeURIComponent(path)}" controls autoplay style="max-width:90vw;max-height:90vh;border-radius:8px;" onclick="event.stopPropagation()"></video>`;
    } else {
        content.innerHTML = `<img src="/api/media?path=${encodeURIComponent(path)}" style="max-width:90vw;max-height:90vh;border-radius:8px;" onclick="event.stopPropagation()" />`;
    }
    document.getElementById('previewOverlay').classList.add('active');
}

function closePreview() {
    const overlay = document.getElementById('previewOverlay');
    overlay.classList.remove('active');
    const video = overlay.querySelector('video');
    if (video) video.pause();
    document.getElementById('previewContent').innerHTML = '';
}

document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') { closePreview(); dupCloseModal(); }
});

// =====================================================================
// Utilities
// =====================================================================

function escapeHtml(str) {
    const d = document.createElement('div');
    d.textContent = str;
    return d.innerHTML;
}

function escapeAttr(str) {
    return str.replace(/'/g, "\\'").replace(/"/g, '&quot;');
}

function formatBytes(bytes) {
    if (bytes === 0) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(1024));
    return (bytes / Math.pow(1024, i)).toFixed(2) + ' ' + units[i];
}

function formatDuration(seconds) {
    if (seconds < 60) return Math.round(seconds) + 's';
    const m = Math.floor(seconds / 60);
    const s = Math.round(seconds % 60);
    return `${m}m ${s}s`;
}

function showToast(message, type = 'success') {
    const container = document.getElementById('toastContainer');
    const toast = document.createElement('div');
    toast.className = `toast ${type}`;
    toast.textContent = message;
    container.appendChild(toast);
    setTimeout(() => {
        toast.style.opacity = '0';
        toast.style.transition = 'opacity 0.3s';
        setTimeout(() => toast.remove(), 300);
    }, 4000);
}

function sleep(ms) { return new Promise(r => setTimeout(r, ms)); }
