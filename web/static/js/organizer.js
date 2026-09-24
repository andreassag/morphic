// ── Organizer Module ─────────────────────────────────────────────────

import { showToast, formatFileSize } from './ui.js';
import { onEvent } from './events.js';

let currentOrgJobId = null;

export function initOrganizer() {
    onEvent('organizer', (event) => {
        const payload = event.payload || event.data || event;
        if (event.type === 'plan_progress' || event.type === 'execute_progress') {
            updateOrgProgress(payload);
        } else if (event.type === 'plan_ready') {
            finishOrgPlan(payload);
        } else if (event.type === 'execute_done') {
            finishOrgExecute(payload);
        }
    });
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
        html += `
            <tr>
                <td>${item.source.split('/').pop()}</td>
                <td><code>${item.destination}</code></td>
                <td>
                    ${item.conflict 
                        ? `<span class="badge badge-danger">Conflict: ${item.conflict_reason}</span>` 
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

    if (execContent) {
        execContent.innerHTML = `
            <div class="card">
                <div class="card-title">✅ Execution Complete</div>
                <p>Successfully processed organized files.</p>
            </div>
        `;
    }
    if (execResults) execResults.style.display = 'block';
    showToast('Organize execution completed!', 'success');
}
