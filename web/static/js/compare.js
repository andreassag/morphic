// ── Side-by-Side Media Comparison & Diff Inspector ───────────────────

import { showToast, formatFileSize } from './ui.js';

let compareMode = 'split'; // 'split' | 'diff'
let leftFilePath = '';
let rightFilePath = '';
let isDraggingHandle = false;

let currentDiffUrl = '';

export function openCompareModal(leftPath, rightPath) {
    leftFilePath = leftPath;
    rightFilePath = rightPath;
    currentDiffUrl = '';

    const modal = document.getElementById('compareModal');
    if (!modal) return;

    modal.classList.add('active');

    // Reset split slider to 50%
    const container = document.getElementById('splitViewContainer');
    if (container) container.style.setProperty('--split-pos', '50%');
    const handle = document.getElementById('splitHandle');
    if (handle) handle.style.left = '50%';

    setCompareMode('split');
    loadComparisonData();
    initSplitHandleDrag();
}

export function closeCompareModal() {
    const modal = document.getElementById('compareModal');
    if (modal) modal.classList.remove('active');
}

export function setCompareMode(mode) {
    compareMode = mode;
    const btnSplit = document.getElementById('btnModeSplit');
    const btnDiff = document.getElementById('btnModeDiff');
    const splitView = document.getElementById('splitViewContainer');
    const diffView = document.getElementById('diffViewContainer');

    if (btnSplit) btnSplit.classList.toggle('active', mode === 'split');
    if (btnDiff) btnDiff.classList.toggle('active', mode === 'diff');

    if (splitView) splitView.style.display = mode === 'split' ? 'block' : 'none';
    if (diffView) diffView.style.display = mode === 'diff' ? 'flex' : 'none';

    if (mode === 'diff') {
        const diffImg = document.getElementById('compareDiffImg');
        const loader = document.getElementById('diffLoadingIndicator');
        const errorMsg = document.getElementById('diffErrorMsg');
        const diffUrl = `/api/media/diff?left=${encodeURIComponent(leftFilePath)}&right=${encodeURIComponent(rightFilePath)}`;

        if (currentDiffUrl !== diffUrl && leftFilePath && rightFilePath) {
            currentDiffUrl = diffUrl;
            if (loader) loader.style.display = 'flex';
            if (errorMsg) errorMsg.style.display = 'none';
            if (diffImg) {
                diffImg.style.display = 'none';
                diffImg.onload = () => {
                    if (loader) loader.style.display = 'none';
                    diffImg.style.display = 'block';
                };
                diffImg.onerror = () => {
                    if (loader) loader.style.display = 'none';
                    if (errorMsg) {
                        errorMsg.textContent = 'Failed to generate visual difference heatmap';
                        errorMsg.style.display = 'block';
                    }
                };
                diffImg.src = diffUrl;
            }
        }
    }
}

async function loadComparisonData() {
    const imgLeft = document.getElementById('compareImgLeft');
    const imgRight = document.getElementById('compareImgRight');
    const leftDetails = document.getElementById('metaLeftDetails');
    const rightDetails = document.getElementById('metaRightDetails');
    const diffDetails = document.getElementById('metaDiffDetails');

    if (imgLeft) imgLeft.src = `/api/media?path=${encodeURIComponent(leftFilePath)}`;
    if (imgRight) imgRight.src = `/api/media?path=${encodeURIComponent(rightFilePath)}`;

    try {
        const res = await fetch(`/api/media/compare?left=${encodeURIComponent(leftFilePath)}&right=${encodeURIComponent(rightFilePath)}`);
        if (!res.ok) throw new Error('Failed to load metadata comparison');
        const data = await res.json();

        if (leftDetails) {
            leftDetails.innerHTML = `
                <div class="meta-row"><span class="meta-label">File:</span> <span class="meta-val">${data.left.filename}</span></div>
                <div class="meta-row"><span class="meta-label">Size:</span> <span class="meta-val">${data.left.size_fmt}</span></div>
                <div class="meta-row"><span class="meta-label">Res:</span> <span class="meta-val">${data.left.resolution || 'N/A'}</span></div>
                <div class="meta-row"><span class="meta-label">Format:</span> <span class="meta-val">${data.left.format}</span></div>
            `;
        }

        if (rightDetails) {
            rightDetails.innerHTML = `
                <div class="meta-row"><span class="meta-label">File:</span> <span class="meta-val">${data.right.filename}</span></div>
                <div class="meta-row"><span class="meta-label">Size:</span> <span class="meta-val">${data.right.size_fmt}</span></div>
                <div class="meta-row"><span class="meta-label">Res:</span> <span class="meta-val">${data.right.resolution || 'N/A'}</span></div>
                <div class="meta-row"><span class="meta-label">Format:</span> <span class="meta-val">${data.right.format}</span></div>
            `;
        }

        if (diffDetails) {
            const savingsPct = data.left.size > 0 ? (((data.left.size - data.right.size) / data.left.size) * 100).toFixed(1) : '0';
            diffDetails.innerHTML = `
                <div class="meta-row"><span class="meta-label">Size Delta:</span> <span class="meta-val">${data.size_diff_fmt} (${savingsPct}%)</span></div>
                <div class="meta-row"><span class="meta-label">Dimensions:</span> <span class="meta-val">${data.is_same_dimensions ? 'Identical' : 'Different'}</span></div>
            `;
        }
    } catch (err) {
        showToast(err.message, 'error');
    }
}

function initSplitHandleDrag() {
    const viewport = document.getElementById('compareViewport');
    const container = document.getElementById('splitViewContainer');
    const handle = document.getElementById('splitHandle');
    if (!viewport || !container || !handle) return;

    function updatePos(clientX) {
        const rect = viewport.getBoundingClientRect();
        if (rect.width <= 0) return;
        let pos = (clientX - rect.left) / rect.width;
        pos = Math.max(0.01, Math.min(0.99, pos));
        const pct = `${(pos * 100).toFixed(2)}%`;
        container.style.setProperty('--split-pos', pct);
        handle.style.left = pct;
    }

    function onMove(e) {
        if (!isDraggingHandle) return;
        const clientX = e.touches ? e.touches[0].clientX : e.clientX;
        updatePos(clientX);
    }

    handle.onmousedown = (e) => {
        e.preventDefault();
        e.stopPropagation();
        isDraggingHandle = true;
    };
    handle.ontouchstart = (e) => {
        e.stopPropagation();
        isDraggingHandle = true;
    };

    container.onmousedown = (e) => {
        if (e.target === handle || handle.contains(e.target)) return;
        isDraggingHandle = true;
        updatePos(e.clientX);
    };

    window.onmousemove = onMove;
    window.ontouchmove = onMove;
    window.onmouseup = () => { isDraggingHandle = false; };
    window.ontouchend = () => { isDraggingHandle = false; };
}
