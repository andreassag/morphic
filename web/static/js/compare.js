// ── Side-by-Side Media Comparison & Diff Inspector ───────────────────

import { showToast, formatFileSize } from './ui.js';

let compareMode = 'split'; // 'split' | 'diff' | 'video'
let leftFilePath = '';
let rightFilePath = '';
let isDraggingHandle = false;
let currentDiffUrl = '';

// Video inspection state
const VIDEO_EXTS = new Set(['mp4', 'mov', 'avi', 'mkv', 'webm', 'flv', 'wmv', 'm4v', 'mpeg', '3gp', 'ts']);
let isVideoMode = false;
let isVideoSynced = true;
let isVideoLoop = true;
let audioState = 'muted'; // 'muted' | 'left' | 'right'

function isVideoFile(path) {
    if (!path) return false;
    const ext = path.split('.').pop().toLowerCase();
    return VIDEO_EXTS.has(ext);
}

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

    isVideoMode = isVideoFile(leftPath) || isVideoFile(rightPath);

    const imageModeGroup = document.getElementById('imageModeGroup');
    const videoModeBadge = document.getElementById('videoModeBadge');
    const splitView = document.getElementById('splitViewContainer');
    const diffView = document.getElementById('diffViewContainer');
    const videoView = document.getElementById('videoViewContainer');
    const titleEl = document.getElementById('compareModalTitle');

    if (isVideoMode) {
        if (titleEl) titleEl.textContent = '🎬 Side-by-Side Video Inspector';
        if (imageModeGroup) imageModeGroup.style.display = 'none';
        if (videoModeBadge) videoModeBadge.style.display = 'flex';
        if (splitView) splitView.style.display = 'none';
        if (diffView) diffView.style.display = 'none';
        if (videoView) videoView.style.display = 'flex';

        initVideoPlayers();
    } else {
        if (titleEl) titleEl.textContent = '🔍 Side-by-Side Media Inspector';
        if (imageModeGroup) imageModeGroup.style.display = 'flex';
        if (videoModeBadge) videoModeBadge.style.display = 'none';
        if (videoView) {
            videoView.style.display = 'none';
            pauseVideoPlayers();
        }

        setCompareMode('split');
        initSplitHandleDrag();
    }

    loadComparisonData();
}

export function closeCompareModal() {
    const modal = document.getElementById('compareModal');
    if (modal) modal.classList.remove('active');
    pauseVideoPlayers();

    const vidLeft = document.getElementById('compareVidLeft');
    const vidRight = document.getElementById('compareVidRight');
    if (vidLeft) { vidLeft.pause(); vidLeft.removeAttribute('src'); vidLeft.load(); }
    if (vidRight) { vidRight.pause(); vidRight.removeAttribute('src'); vidRight.load(); }
}

export function setCompareMode(mode) {
    if (isVideoMode) return;
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

function initVideoPlayers() {
    const vidLeft = document.getElementById('compareVidLeft');
    const vidRight = document.getElementById('compareVidRight');
    const labelLeft = document.getElementById('vidLabelLeft');
    const labelRight = document.getElementById('vidLabelRight');
    const btnPlay = document.getElementById('vidBtnPlayPause');
    const timeDisplay = document.getElementById('vidTimeDisplay');
    const scrubber = document.getElementById('vidScrubber');

    if (!vidLeft || !vidRight) return;

    const leftName = leftFilePath.split('/').pop();
    const rightName = rightFilePath.split('/').pop();
    if (labelLeft) labelLeft.textContent = `Left: ${leftName}`;
    if (labelRight) labelRight.textContent = `Right: ${rightName}`;

    vidLeft.src = `/api/media?path=${encodeURIComponent(leftFilePath)}`;
    vidRight.src = `/api/media?path=${encodeURIComponent(rightFilePath)}`;
    vidLeft.muted = true;
    vidRight.muted = true;
    vidLeft.loop = isVideoLoop;
    vidRight.loop = isVideoLoop;
    audioState = 'muted';
    updateAudioBtnText();

    if (btnPlay) btnPlay.textContent = '▶ Play';
    if (scrubber) scrubber.value = '0';
    if (timeDisplay) timeDisplay.textContent = '0:00 / 0:00';

    vidLeft.ontimeupdate = () => updateVideoProgress();
    vidLeft.onended = () => {
        if (!isVideoLoop && btnPlay) btnPlay.textContent = '▶ Play';
    };

    vidRight.onplay = () => {
        if (isVideoSynced && vidLeft.paused) vidLeft.play();
    };
    vidRight.onpause = () => {
        if (isVideoSynced && !vidLeft.paused) vidLeft.pause();
    };
}

function pauseVideoPlayers() {
    const vidLeft = document.getElementById('compareVidLeft');
    const vidRight = document.getElementById('compareVidRight');
    const btnPlay = document.getElementById('vidBtnPlayPause');
    if (vidLeft) vidLeft.pause();
    if (vidRight) vidRight.pause();
    if (btnPlay) btnPlay.textContent = '▶ Play';
}

export function togglePlayVideos() {
    const vidLeft = document.getElementById('compareVidLeft');
    const vidRight = document.getElementById('compareVidRight');
    const btnPlay = document.getElementById('vidBtnPlayPause');
    if (!vidLeft || !vidRight) return;

    if (vidLeft.paused || vidRight.paused) {
        if (isVideoSynced) {
            // Re-align right to left time if slight desync
            if (Math.abs(vidLeft.currentTime - vidRight.currentTime) > 0.2) {
                vidRight.currentTime = vidLeft.currentTime;
            }
        }
        vidLeft.play().catch(() => {});
        vidRight.play().catch(() => {});
        if (btnPlay) btnPlay.textContent = '⏸ Pause';
    } else {
        vidLeft.pause();
        vidRight.pause();
        if (btnPlay) btnPlay.textContent = '▶ Play';
    }
}

export function stepVideoFrame(frames) {
    const vidLeft = document.getElementById('compareVidLeft');
    const vidRight = document.getElementById('compareVidRight');
    if (!vidLeft && !vidRight) return;

    pauseVideoPlayers();
    const fps = 30; // standard approximation
    const delta = frames / fps;

    if (vidLeft) vidLeft.currentTime = Math.max(0, vidLeft.currentTime + delta);
    if (isVideoSynced && vidRight && vidLeft) {
        vidRight.currentTime = vidLeft.currentTime;
    } else if (vidRight) {
        vidRight.currentTime = Math.max(0, vidRight.currentTime + delta);
    }
    updateVideoProgress();
}

export function onScrubberInput(val) {
    const vidLeft = document.getElementById('compareVidLeft');
    const vidRight = document.getElementById('compareVidRight');
    if (!vidLeft) return;

    const duration = vidLeft.duration || (vidRight ? vidRight.duration : 0) || 0;
    if (duration <= 0) return;

    const targetTime = (parseFloat(val) / 100) * duration;
    vidLeft.currentTime = targetTime;
    if (isVideoSynced && vidRight) {
        vidRight.currentTime = targetTime;
    }
    updateVideoProgress();
}

function updateVideoProgress() {
    const vidLeft = document.getElementById('compareVidLeft');
    const vidRight = document.getElementById('compareVidRight');
    const scrubber = document.getElementById('vidScrubber');
    const timeDisplay = document.getElementById('vidTimeDisplay');
    if (!vidLeft) return;

    const cur = vidLeft.currentTime || 0;
    const dur = vidLeft.duration || (vidRight ? vidRight.duration : 0) || 0;

    if (scrubber && dur > 0) {
        scrubber.value = ((cur / dur) * 100).toFixed(1);
    }
    if (timeDisplay) {
        timeDisplay.textContent = `${formatTime(cur)} / ${formatTime(dur)}`;
    }

    // Keep right in sync if synced mode
    if (isVideoSynced && vidRight && !vidLeft.paused && Math.abs(vidLeft.currentTime - vidRight.currentTime) > 0.3) {
        vidRight.currentTime = vidLeft.currentTime;
    }
}

function formatTime(secs) {
    if (!secs || isNaN(secs) || secs < 0) return '0:00';
    const m = Math.floor(secs / 60);
    const s = Math.floor(secs % 60);
    return `${m}:${s < 10 ? '0' : ''}${s}`;
}

export function toggleVideoSync() {
    isVideoSynced = !isVideoSynced;
    const btn = document.getElementById('vidSyncBtn');
    if (btn) {
        btn.classList.toggle('active', isVideoSynced);
        btn.textContent = isVideoSynced ? '🔗 Synced' : '🔓 Unsynced';
    }
    if (isVideoSynced) {
        const vidLeft = document.getElementById('compareVidLeft');
        const vidRight = document.getElementById('compareVidRight');
        if (vidLeft && vidRight) vidRight.currentTime = vidLeft.currentTime;
    }
}

export function toggleVideoAudio() {
    const vidLeft = document.getElementById('compareVidLeft');
    const vidRight = document.getElementById('compareVidRight');
    if (!vidLeft || !vidRight) return;

    if (audioState === 'muted') {
        audioState = 'left';
        vidLeft.muted = false;
        vidRight.muted = true;
    } else if (audioState === 'left') {
        audioState = 'right';
        vidLeft.muted = true;
        vidRight.muted = false;
    } else {
        audioState = 'muted';
        vidLeft.muted = true;
        vidRight.muted = true;
    }
    updateAudioBtnText();
}

function updateAudioBtnText() {
    const btn = document.getElementById('vidAudioBtn');
    if (!btn) return;
    if (audioState === 'left') {
        btn.textContent = '🔊 Left Audio';
        btn.classList.add('active');
    } else if (audioState === 'right') {
        btn.textContent = '🔊 Right Audio';
        btn.classList.add('active');
    } else {
        btn.textContent = '🔇 Muted';
        btn.classList.remove('active');
    }
}

export function toggleVideoLoop() {
    isVideoLoop = !isVideoLoop;
    const vidLeft = document.getElementById('compareVidLeft');
    const vidRight = document.getElementById('compareVidRight');
    const btn = document.getElementById('vidLoopBtn');
    if (vidLeft) vidLeft.loop = isVideoLoop;
    if (vidRight) vidRight.loop = isVideoLoop;
    if (btn) {
        btn.classList.toggle('active', isVideoLoop);
        btn.textContent = isVideoLoop ? '🔁 Loop' : '➡️ Once';
    }
}

async function loadComparisonData() {
    const imgLeft = document.getElementById('compareImgLeft');
    const imgRight = document.getElementById('compareImgRight');
    const leftDetails = document.getElementById('metaLeftDetails');
    const rightDetails = document.getElementById('metaRightDetails');
    const diffDetails = document.getElementById('metaDiffDetails');

    if (!isVideoMode) {
        if (imgLeft) imgLeft.src = `/api/media?path=${encodeURIComponent(leftFilePath)}`;
        if (imgRight) imgRight.src = `/api/media?path=${encodeURIComponent(rightFilePath)}`;
    }

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
                ${data.left.duration_fmt ? `<div class="meta-row"><span class="meta-label">Duration:</span> <span class="meta-val">${data.left.duration_fmt}</span></div>` : ''}
                ${data.left.fps ? `<div class="meta-row"><span class="meta-label">FPS:</span> <span class="meta-val">${data.left.fps}</span></div>` : ''}
            `;
        }

        if (rightDetails) {
            rightDetails.innerHTML = `
                <div class="meta-row"><span class="meta-label">File:</span> <span class="meta-val">${data.right.filename}</span></div>
                <div class="meta-row"><span class="meta-label">Size:</span> <span class="meta-val">${data.right.size_fmt}</span></div>
                <div class="meta-row"><span class="meta-label">Res:</span> <span class="meta-val">${data.right.resolution || 'N/A'}</span></div>
                <div class="meta-row"><span class="meta-label">Format:</span> <span class="meta-val">${data.right.format}</span></div>
                ${data.right.duration_fmt ? `<div class="meta-row"><span class="meta-label">Duration:</span> <span class="meta-val">${data.right.duration_fmt}</span></div>` : ''}
                ${data.right.fps ? `<div class="meta-row"><span class="meta-label">FPS:</span> <span class="meta-val">${data.right.fps}</span></div>` : ''}
            `;
        }

        if (diffDetails) {
            const savingsPct = data.left.size > 0 ? (((data.left.size - data.right.size) / data.left.size) * 100).toFixed(1) : '0';
            diffDetails.innerHTML = `
                <div class="meta-row"><span class="meta-label">Size Delta:</span> <span class="meta-val">${data.size_diff_fmt} (${savingsPct}%)</span></div>
                <div class="meta-row"><span class="meta-label">Dimensions:</span> <span class="meta-val">${data.is_same_dimensions ? 'Identical' : 'Different'}</span></div>
                ${data.left.duration && data.right.duration ? `<div class="meta-row"><span class="meta-label">Dur Delta:</span> <span class="meta-val">${Math.abs(data.left.duration - data.right.duration).toFixed(1)}s</span></div>` : ''}
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

// Expose handlers globally for template onclick events
window.openCompareModal = openCompareModal;
window.closeCompareModal = closeCompareModal;
window.setCompareMode = setCompareMode;
window.togglePlayVideos = togglePlayVideos;
window.stepVideoFrame = stepVideoFrame;
window.onScrubberInput = onScrubberInput;
window.toggleVideoSync = toggleVideoSync;
window.toggleVideoAudio = toggleVideoAudio;
window.toggleVideoLoop = toggleVideoLoop;
