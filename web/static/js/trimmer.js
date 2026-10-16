// ── Trimmer Module: Image Transforms & Video Segment Clipper ──────────

import * as UI from './ui.js';

let currentMedia = null;
let isVideoMedia = false;
let loadedImage = null;

// Transform states
let rotation = 0; // 0, 90, 180, 270
let flipH = false;
let flipV = false;
let cropAspect = 'free'; // 'free', '1:1', '4:3', '16:9', '9:16'
let originalDims = { w: 0, h: 0 };
let currentCrop = { x: 0, y: 0, w: 0, h: 0 };

export function initTrimmer() {
    // Optional setup
}

export function trimmerInitTab() {
    const input = document.getElementById('trimmerPath');
    if (input && !input.value) {
        const convInput = document.getElementById('convFolder');
        if (convInput && convInput.value) {
            input.value = convInput.value;
        }
    }
}

export async function trimmerLoad() {
    const pathInput = document.getElementById('trimmerPath');
    const path = pathInput ? pathInput.value.trim() : '';
    if (!path) {
        UI.showToast('Please specify a media file path to load', 'warning');
        return;
    }

    const ext = path.split('.').pop().toLowerCase();
    isVideoMedia = ['mp4', 'mkv', 'mov', 'avi', 'webm', 'flv', 'm4v'].includes(ext);
    currentMedia = path;

    // Reset transforms
    rotation = 0;
    flipH = false;
    flipV = false;
    cropAspect = 'free';

    const workspace = document.getElementById('trimmerWorkspace');
    const origContainer = document.getElementById('trimmerOriginalContainer');
    const imgControls = document.getElementById('trimmerImageControls');
    const vidControls = document.getElementById('trimmerVideoControls');
    const previewCanvas = document.getElementById('trimmerPreviewCanvas');
    const previewVideo = document.getElementById('trimmerPreviewVideo');
    const outputInput = document.getElementById('trimmerOutputPath');

    if (!workspace || !origContainer) return;
    workspace.style.display = 'block';

    // Default output path suggestion
    if (outputInput) {
        const parts = path.split('.');
        const extPart = parts.pop();
        outputInput.value = `${parts.join('.')}_trimmed.${extPart}`;
    }

    if (isVideoMedia) {
        if (imgControls) imgControls.style.display = 'none';
        if (vidControls) vidControls.style.display = 'block';
        if (previewCanvas) previewCanvas.style.display = 'none';
        if (previewVideo) previewVideo.style.display = 'block';

        origContainer.innerHTML = `
            <video id="trimmerOriginalVideo" src="/api/media?path=${encodeURIComponent(path)}" controls style="max-width:100%; max-height:420px; border-radius:var(--radius-sm);"></video>
        `;

        if (previewVideo) {
            previewVideo.src = `/api/media?path=${encodeURIComponent(path)}`;
        }

        const v = document.getElementById('trimmerOriginalVideo');
        if (v) {
            v.onloadedmetadata = () => {
                const dur = v.duration || 10;
                document.getElementById('trimVideoStart').value = '0.0';
                document.getElementById('trimVideoEnd').value = dur.toFixed(1);
            };
        }
    } else {
        if (imgControls) imgControls.style.display = 'block';
        if (vidControls) vidControls.style.display = 'none';
        if (previewVideo) previewVideo.style.display = 'none';
        if (previewCanvas) previewCanvas.style.display = 'block';

        origContainer.innerHTML = `<span class="spinner"></span>`;

        loadedImage = new Image();
        loadedImage.crossOrigin = 'anonymous';
        loadedImage.onload = () => {
            originalDims = { w: loadedImage.naturalWidth, h: loadedImage.naturalHeight };
            currentCrop = { x: 0, y: 0, w: originalDims.w, h: originalDims.h };

            origContainer.innerHTML = `
                <img src="${loadedImage.src}" alt="Original" style="max-width:100%; max-height:420px; object-fit:contain; border-radius:var(--radius-sm);" />
            `;

            const resizeW = document.getElementById('trimResizeW');
            const resizeH = document.getElementById('trimResizeH');
            const cropW = document.getElementById('trimCropW');
            const cropH = document.getElementById('trimCropH');

            if (resizeW) resizeW.value = originalDims.w;
            if (resizeH) resizeH.value = originalDims.h;
            if (cropW) cropW.value = originalDims.w;
            if (cropH) cropH.value = originalDims.h;

            renderTransformedPreview();
        };
        loadedImage.onerror = () => {
            origContainer.innerHTML = `<span style="color:var(--danger);">Failed to load image preview</span>`;
        };
        loadedImage.src = `/api/media?path=${encodeURIComponent(path)}`;
    }

    UI.showToast(`Loaded ${path.split('/').pop()}`, 'info');
}

// ── Interactive Before/After Canvas Transformation ─────────────────────

function renderTransformedPreview() {
    if (!loadedImage || isVideoMedia) return;

    const canvas = document.getElementById('trimmerPreviewCanvas');
    if (!canvas) return;

    const cropW = parseInt(document.getElementById('trimCropW')?.value) || originalDims.w;
    const cropH = parseInt(document.getElementById('trimCropH')?.value) || originalDims.h;
    const targetW = parseInt(document.getElementById('trimResizeW')?.value) || cropW;
    const targetH = parseInt(document.getElementById('trimResizeH')?.value) || cropH;

    // Handle 90/270 deg swap
    const isSwapped = (rotation === 90 || rotation === 270);
    const canvasW = isSwapped ? targetH : targetW;
    const canvasH = isSwapped ? targetW : targetH;

    canvas.width = canvasW;
    canvas.height = canvasH;

    const ctx = canvas.getContext('2d');
    ctx.clearRect(0, 0, canvasW, canvasH);
    ctx.save();

    // Center origin for transforms
    ctx.translate(canvasW / 2, canvasH / 2);

    // Apply rotation
    ctx.rotate((rotation * Math.PI) / 180);

    // Apply flip
    ctx.scale(flipH ? -1 : 1, flipV ? -1 : 1);

    // Crop source coordinates
    const sx = (originalDims.w - cropW) / 2;
    const sy = (originalDims.h - cropH) / 2;

    // Draw transformed image
    ctx.drawImage(
        loadedImage,
        Math.max(0, sx), Math.max(0, sy), Math.min(originalDims.w, cropW), Math.min(originalDims.h, cropH),
        -targetW / 2, -targetH / 2, targetW, targetH
    );

    ctx.restore();
}

export function trimmerRotate(deg) {
    rotation = (rotation + deg + 360) % 360;
    renderTransformedPreview();
}

export function trimmerFlipH() {
    flipH = !flipH;
    renderTransformedPreview();
}

export function trimmerFlipV() {
    flipV = !flipV;
    renderTransformedPreview();
}

export function trimmerSetAspect(aspect) {
    cropAspect = aspect;
    document.querySelectorAll('#trimmerImageControls .btn-group .btn').forEach(btn => {
        btn.classList.toggle('active', btn.textContent.toLowerCase().includes(aspect));
    });

    if (!originalDims.w) return;

    const cropW = document.getElementById('trimCropW');
    const cropH = document.getElementById('trimCropH');

    if (aspect === '1:1') {
        const side = Math.min(originalDims.w, originalDims.h);
        if (cropW) cropW.value = side;
        if (cropH) cropH.value = side;
    } else if (aspect === '4:3') {
        const w = originalDims.w;
        const h = Math.round((w * 3) / 4);
        if (cropW) cropW.value = w;
        if (cropH) cropH.value = Math.min(h, originalDims.h);
    } else if (aspect === '16:9') {
        const w = originalDims.w;
        const h = Math.round((w * 9) / 16);
        if (cropW) cropW.value = w;
        if (cropH) cropH.value = Math.min(h, originalDims.h);
    } else if (aspect === '9:16') {
        const h = originalDims.h;
        const w = Math.round((h * 9) / 16);
        if (cropW) cropW.value = Math.min(w, originalDims.w);
        if (cropH) cropH.value = h;
    } else {
        if (cropW) cropW.value = originalDims.w;
        if (cropH) cropH.value = originalDims.h;
    }

    renderTransformedPreview();
}

export function trimmerUpdateTransforms() {
    renderTransformedPreview();
}

export function trimmerOnResizeInput(changed) {
    const lockAspect = document.getElementById('trimLockAspect')?.checked;
    const wInput = document.getElementById('trimResizeW');
    const hInput = document.getElementById('trimResizeH');

    if (lockAspect && originalDims.w && originalDims.h) {
        const ratio = originalDims.w / originalDims.h;
        if (changed === 'w' && wInput && hInput) {
            hInput.value = Math.round(parseFloat(wInput.value) / ratio) || '';
        } else if (changed === 'h' && wInput && hInput) {
            wInput.value = Math.round(parseFloat(hInput.value) * ratio) || '';
        }
    }

    renderTransformedPreview();
}

// ── Video Controls ─────────────────────────────────────────────────────

export function trimmerSetStartCurrent() {
    const v = document.getElementById('trimmerOriginalVideo');
    if (v) {
        document.getElementById('trimVideoStart').value = v.currentTime.toFixed(2);
        trimmerUpdateVideoRange();
    }
}

export function trimmerSetEndCurrent() {
    const v = document.getElementById('trimmerOriginalVideo');
    if (v) {
        document.getElementById('trimVideoEnd').value = v.currentTime.toFixed(2);
        trimmerUpdateVideoRange();
    }
}

export function trimmerUpdateVideoRange() {
    const start = parseFloat(document.getElementById('trimVideoStart')?.value) || 0;
    const end = parseFloat(document.getElementById('trimVideoEnd')?.value) || 0;
    const v = document.getElementById('trimmerPreviewVideo');
    if (v && end > start) {
        v.currentTime = start;
    }
}

// ── Export ─────────────────────────────────────────────────────────────

export async function trimmerExport() {
    if (!currentMedia) {
        UI.showToast('No media file loaded', 'warning');
        return;
    }

    const outputPath = document.getElementById('trimmerOutputPath')?.value.trim();
    if (!outputPath) {
        UI.showToast('Please specify an output destination path', 'warning');
        return;
    }

    const exportBtn = document.getElementById('trimmerExportBtn');
    if (exportBtn) exportBtn.disabled = true;

    const payload = {
        path: currentMedia,
        output_path: outputPath,
        is_video: isVideoMedia
    };

    if (isVideoMedia) {
        payload.trim = {
            start_sec: parseFloat(document.getElementById('trimVideoStart')?.value) || 0,
            end_sec: parseFloat(document.getElementById('trimVideoEnd')?.value) || 0,
            stream_copy: document.getElementById('trimStreamCopy')?.checked ?? true
        };
    } else {
        payload.crop = {
            width: parseInt(document.getElementById('trimCropW')?.value) || originalDims.w,
            height: parseInt(document.getElementById('trimCropH')?.value) || originalDims.h
        };
        payload.rotate = rotation;
        payload.flip_h = flipH;
        payload.flip_v = flipV;
        payload.resize = {
            width: parseInt(document.getElementById('trimResizeW')?.value) || originalDims.w,
            height: parseInt(document.getElementById('trimResizeH')?.value) || originalDims.h
        };
    }

    try {
        const res = await fetch('/api/trimmer/export', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload)
        });

        if (!res.ok) {
            const err = await res.json().catch(() => ({ error: 'Export failed' }));
            throw new Error(err.error || `HTTP ${res.status}`);
        }

        UI.showToast(`Exported successfully: ${outputPath.split('/').pop()}`, 'success');
    } catch (err) {
        console.error('Trimmer export error:', err);
        UI.showToast(`Export error: ${err.message}`, 'error');
    } finally {
        if (exportBtn) exportBtn.disabled = false;
    }
}
