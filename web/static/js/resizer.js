// ── Resizer Module: Batch Image Resizing & Preset Generator ───────────

import * as UI from './ui.js';

let resizerJobId = null;
let resizerPollTimer = null;
const selectedPresets = new Set(['web-1920', 'web-1280', 'thumb-300']);

export function initResizer() {
    // Optional setup
}

export function resizerInitTab() {
    const input = document.getElementById('resizerFolder');
    if (input && !input.value) {
        const convInput = document.getElementById('convFolder');
        if (convInput && convInput.value) {
            input.value = convInput.value;
            const outInput = document.getElementById('resizerOutputFolder');
            if (outInput && !outInput.value) {
                outInput.value = `${convInput.value.replace(/\/+$/, '')}_resized`;
            }
        }
    }
}

export function resizerTogglePreset(cardEl, presetKey) {
    const cb = cardEl.querySelector('input[type="checkbox"]');
    if (selectedPresets.has(presetKey)) {
        selectedPresets.delete(presetKey);
        cardEl.classList.remove('selected');
        if (cb) cb.checked = false;
    } else {
        selectedPresets.add(presetKey);
        cardEl.classList.add('selected');
        if (cb) cb.checked = true;
    }

    if (presetKey === 'custom') {
        const customInputs = document.getElementById('resizerCustomInputs');
        if (customInputs) {
            customInputs.style.display = selectedPresets.has('custom') ? 'flex' : 'none';
        }
    }
}

export async function resizerStart() {
    const folder = document.getElementById('resizerFolder')?.value.trim() || '';
    const outputFolder = document.getElementById('resizerOutputFolder')?.value.trim() || '';

    if (!folder) {
        UI.showToast('Please specify a source image directory', 'warning');
        return;
    }
    if (!outputFolder) {
        UI.showToast('Please specify an output root directory', 'warning');
        return;
    }
    if (selectedPresets.size === 0) {
        UI.showToast('Please select at least one resize preset', 'warning');
        return;
    }

    const format = document.getElementById('resizerFormat')?.value || 'webp';
    const quality = parseInt(document.getElementById('resizerQuality')?.value) || 85;
    const aspectMode = document.getElementById('resizerAspectMode')?.value || 'fit';

    const payload = {
        folder,
        output_folder: outputFolder,
        presets: Array.from(selectedPresets),
        format,
        quality,
        aspect_mode: aspectMode
    };

    if (selectedPresets.has('custom')) {
        payload.custom_width = parseInt(document.getElementById('resizerCustomW')?.value) || 800;
        payload.custom_height = parseInt(document.getElementById('resizerCustomH')?.value) || 600;
    }

    const progressCard = document.getElementById('resizerProgressCard');
    const progressFill = document.getElementById('resizerProgressFill');
    const progressMsg = document.getElementById('resizerProgressMsg');
    const progressPct = document.getElementById('resizerProgressPct');
    const startBtn = document.getElementById('resizerStartBtn');
    const summaryCard = document.getElementById('resizerSummaryCard');

    if (summaryCard) summaryCard.style.display = 'none';
    if (progressCard) progressCard.style.display = 'block';
    if (progressFill) progressFill.style.width = '10%';
    if (progressMsg) progressMsg.textContent = 'Starting batch resize…';
    if (progressPct) progressPct.textContent = '0%';
    if (startBtn) startBtn.disabled = true;

    try {
        const res = await fetch('/api/resizer/batch', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload)
        });

        if (!res.ok) {
            const err = await res.json().catch(() => ({ error: 'Batch resize failed' }));
            throw new Error(err.error || `HTTP ${res.status}`);
        }

        const data = await res.json();
        if (data.job_id) {
            resizerJobId = data.job_id;
            pollResizerJob(data.job_id, outputFolder);
        } else {
            showResizerSummary(data, outputFolder);
            if (progressCard) progressCard.style.display = 'none';
        }
    } catch (err) {
        console.error('Resizer error:', err);
        UI.showToast(`Batch resize error: ${err.message}`, 'error');
        if (progressCard) progressCard.style.display = 'none';
    } finally {
        if (startBtn) startBtn.disabled = false;
    }
}

function pollResizerJob(jobId, outputFolder) {
    clearInterval(resizerPollTimer);
    const progressFill = document.getElementById('resizerProgressFill');
    const progressMsg = document.getElementById('resizerProgressMsg');
    const progressPct = document.getElementById('resizerProgressPct');
    const progressCard = document.getElementById('resizerProgressCard');

    resizerPollTimer = setInterval(async () => {
        try {
            const res = await fetch(`/api/resizer/status/${jobId}`);
            if (!res.ok) return;

            const job = await res.json();
            const pct = job.total ? Math.round((job.processed / job.total) * 100) : 0;
            if (progressFill) progressFill.style.width = `${pct}%`;
            if (progressPct) progressPct.textContent = `${pct}%`;
            if (progressMsg) progressMsg.textContent = `Resizing: ${job.processed || 0} / ${job.total || 0} variants`;

            if (job.status === 'completed' || job.status === 'failed' || job.status === 'cancelled') {
                clearInterval(resizerPollTimer);
                if (progressCard) progressCard.style.display = 'none';

                if (job.status === 'completed') {
                    showResizerSummary(job, outputFolder);
                    UI.showToast('Batch resizing completed successfully', 'success');
                } else if (job.status === 'cancelled') {
                    UI.showToast('Batch resize cancelled', 'info');
                } else {
                    UI.showToast(`Batch resize failed: ${job.error || 'Unknown error'}`, 'error');
                }
            }
        } catch (_) {}
    }, 600);
}

function showResizerSummary(job, outputFolder) {
    const card = document.getElementById('resizerSummaryCard');
    const countEl = document.getElementById('resizerSummaryCount');
    const pathEl = document.getElementById('resizerSummaryPath');

    if (!card) return;
    card.style.display = 'block';

    if (countEl) countEl.textContent = (job.total || job.processed || 0).toLocaleString();
    if (pathEl) pathEl.textContent = job.output_folder || outputFolder;
}

export async function resizerCancel() {
    if (!resizerJobId) return;
    try {
        await fetch(`/api/resizer/cancel/${resizerJobId}`, { method: 'POST' });
        clearInterval(resizerPollTimer);
        const progressCard = document.getElementById('resizerProgressCard');
        if (progressCard) progressCard.style.display = 'none';
        UI.showToast('Cancelling resize job…', 'info');
    } catch (err) {
        console.error('Cancel resizer error:', err);
    }
}
