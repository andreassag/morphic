// ── Organizer Module ─────────────────────────────────────────────────

import { showToast, formatFileSize } from './ui.js';
import { onEvent } from './events.js';

let currentOrgJobId = null;
let orgPollTimer = null;

export function initOrganizer() {
    onEvent('organizer', (event) => {
        const payload = event.payload || event.data || event;
        if (event.type === 'progress') {
            if (payload.phase === 'planned') {
                stopOrgPolling();
                finishOrgPlan(payload);
            } else if (payload.phase === 'done') {
                stopOrgPolling();
                finishOrgExecute(payload);
            } else {
                updateOrgProgress(payload);
            }
        } else if (event.type === 'job_done') {
            stopOrgPolling();
            if (payload.phase === 'planned') {
                finishOrgPlan(payload);
            } else {
                finishOrgExecute(payload);
            }
        } else if (event.type === 'job_failed') {
            stopOrgPolling();
            cancelOrgUI(payload.error || 'Organizer job failed');
        } else if (event.type === 'job_cancelled') {
            stopOrgPolling();
            cancelOrgUI('Organizer job was cancelled');
        }
    });

    onEvent('system', (event) => {
        if (event.type === 'reconnect' && currentOrgJobId) {
            pollOrgStatus(currentOrgJobId);
        }
    });
}

function startOrgPolling(jobId) {
    stopOrgPolling();
    orgPollTimer = setInterval(() => {
        if (currentOrgJobId === jobId) {
            pollOrgStatus(jobId);
        }
    }, 1500);
}

function stopOrgPolling() {
    if (orgPollTimer) {
        clearInterval(orgPollTimer);
        orgPollTimer = null;
    }
}

async function pollOrgStatus(jobId) {
    if (!jobId) return;
    try {
        const res = await fetch(`/api/organizer/status/${jobId}`);
        if (!res.ok) return;
        const data = await res.json();
        if (currentOrgJobId !== jobId) return;

        if (data.phase === 'planned') {
            stopOrgPolling();
            finishOrgPlan(data);
        } else if (data.phase === 'done' || data.status === 'done') {
            stopOrgPolling();
            finishOrgExecute(data);
        } else if (data.status === 'failed') {
            stopOrgPolling();
            cancelOrgUI(data.error || 'Job failed');
        } else if (data.status === 'cancelled') {
            stopOrgPolling();
            cancelOrgUI('Job cancelled');
        } else {
            updateOrgProgress(data);
        }
    } catch (err) {
        console.error('Error polling organizer status:', err);
    }
}

export function orgModeChanged() {
    const mode = document.getElementById('orgMode')?.value;
    const sortOpts = document.getElementById('orgSortOptions');
    const renameOpts = document.getElementById('orgRenameOptions');

    if (sortOpts) sortOpts.style.display = mode === 'sort' ? 'block' : 'none';
    if (renameOpts) renameOpts.style.display = mode === 'rename' ? 'block' : 'none';
}

export async function orgStartPlan() {
    const folder = document.getElementById('orgFolder')?.value.trim();
    if (!folder) {
        showToast('Please enter a source folder path', 'warning');
        return;
    }

    const mode = document.getElementById('orgMode')?.value || 'sort';
    const operation = document.getElementById('orgOperation')?.value || 'copy';
    const template = mode === 'sort' 
        ? document.getElementById('orgTemplate')?.value || '{year}/{month}'
        : document.getElementById('orgRenameTemplate')?.value || '{date}_{seq:4}';
    const destination = document.getElementById('orgDest')?.value || '';
    const startSeq = parseInt(document.getElementById('orgStartSeq')?.value || '1', 10);

    const progressCard = document.getElementById('orgProgress');
    const planResults = document.getElementById('orgPlanResults');
    const execResults = document.getElementById('orgExecResults');
    const execBtn = document.getElementById('orgExecBtn');

    if (progressCard) progressCard.style.display = 'block';
    if (planResults) planResults.style.display = 'none';
    if (execResults) execResults.style.display = 'none';
    if (execBtn) execBtn.style.display = 'none';

    try {
        const res = await fetch('/api/organizer/plan', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                folder: folder,
                mode: mode,
                operation: operation,
                template: template,
                destination: destination,
                start_seq: startSeq
            })
        });

        if (!res.ok) {
            const errData = await res.json();
            throw new Error(errData.error?.message || 'Failed to start plan');
        }

        const data = await res.json();
        currentOrgJobId = data.job_id;
        startOrgPolling(currentOrgJobId);
        showToast('Generating organize plan...', 'info');
    } catch (err) {
        showToast(err.message, 'error');
        if (progressCard) progressCard.style.display = 'none';
    }
}

export async function orgExecute() {
    if (!currentOrgJobId) return;

    const progressCard = document.getElementById('orgProgress');
    const execBtn = document.getElementById('orgExecBtn');

    if (progressCard) progressCard.style.display = 'block';
    if (execBtn) execBtn.disabled = true;

    try {
        const res = await fetch('/api/organizer/execute', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ job_id: currentOrgJobId })
        });

        if (!res.ok) throw new Error('Failed to start execution');
        startOrgPolling(currentOrgJobId);
        showToast('Executing organize plan...', 'info');
    } catch (err) {
        showToast(err.message, 'error');
        if (progressCard) progressCard.style.display = 'none';
        if (execBtn) execBtn.disabled = false;
    }
}

export async function orgCancelJob() {
    if (!currentOrgJobId) return;
    try {
        await fetch(`/api/organizer/cancel/${currentOrgJobId}`, { method: 'POST' });
        showToast('Cancelling organizer job...', 'warning');
    } catch (err) {
        console.error(err);
    }
}

function cancelOrgUI(msg) {
    const progressCard = document.getElementById('orgProgress');
    if (progressCard) progressCard.style.display = 'none';
    const execBtn = document.getElementById('orgExecBtn');
    if (execBtn) execBtn.disabled = false;
    showToast(msg || 'Organizer job interrupted', 'warning');
}

function updateOrgProgress(payload) {
    const bar = document.getElementById('orgProgressBar');
    const pct = document.getElementById('orgProgressPct');
    const msg = document.getElementById('orgProgressMsg');

    const progress = Math.round((payload.progress || 0) * 100);
    if (bar) bar.style.width = `${progress}%`;
    if (pct) pct.textContent = `${progress}%`;
    if (msg) msg.textContent = payload.message || 'Processing...';
}

function finishOrgPlan(payload) {
    const progressCard = document.getElementById('orgProgress');
    const planResults = document.getElementById('orgPlanResults');
    const planContent = document.getElementById('orgPlanContent');
    const execBtn = document.getElementById('orgExecBtn');

    if (progressCard) progressCard.style.display = 'none';
    if (execBtn) {
        execBtn.style.display = 'inline-flex';
        execBtn.disabled = false;
    }

    const plan = payload.plan || [];
    const conflicts = payload.conflicts || 0;

    let html = `
        <div class="card-title">📋 Plan Preview (${plan.length} files, ${conflicts} conflict(s))</div>
        <div class="table-responsive" style="max-height:400px;overflow:auto;">
            <table class="data-table">
                <thead>
                    <tr>
                        <th>Source File</th>
                        <th>Destination</th>
                        <th>Status</th>
                    </tr>
                </thead>
                <tbody>
    `;

    plan.forEach(item => {
        const conflictLabel = item.conflict_reason ? `Conflict: ${item.conflict_reason}` : 'Conflict';
        html += `
            <tr>
                <td>${item.source.split('/').pop()}</td>
                <td><code>${item.destination}</code></td>
                <td>
                    ${item.conflict 
                        ? `<span class="badge badge-danger">${conflictLabel}</span>` 
                        : `<span class="badge badge-success">OK</span>`}
                </td>
            </tr>
        `;
    });

    html += `</tbody></table></div>`;

    if (planContent) planContent.innerHTML = html;
    if (planResults) planResults.style.display = 'block';
    showToast(`Plan ready: ${plan.length} files`, 'success');
}

function finishOrgExecute(payload) {
    const progressCard = document.getElementById('orgProgress');
    const execResults = document.getElementById('orgExecResults');
    const execContent = document.getElementById('orgExecContent');
    const execBtn = document.getElementById('orgExecBtn');

    if (progressCard) progressCard.style.display = 'none';
    if (execBtn) execBtn.style.display = 'none';

    const execData = payload.execution || {};
    const completed = execData.completed || payload.processed || 0;
    const errors = execData.errors || 0;
    const skipped = execData.skipped || 0;

    if (execContent) {
        execContent.innerHTML = `
            <div class="card">
                <div class="card-title">✅ Execution Complete</div>
                <p style="font-size:14px;color:var(--text);margin-top:4px;">
                    Successfully processed <strong>${completed}</strong> file(s)
                    ${skipped > 0 ? `, <strong>${skipped}</strong> skipped (conflicts)` : ''}
                    ${errors > 0 ? `, <span style="color:var(--danger);"><strong>${errors}</strong> errors</span>` : ''}.
                </p>
            </div>
        `;
    }
    if (execResults) execResults.style.display = 'block';
    showToast('Organize execution completed!', 'success');
}
