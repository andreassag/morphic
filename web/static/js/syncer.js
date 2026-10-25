// ── Syncer Module: One-Way / Two-Way Directory Synchronization ─────────

import * as UI from './ui.js';

let currentPlanId = null;
let currentPlanDiff = [];
let syncerJobId = null;
let syncerPollTimer = null;

export function initSyncer() {
    // Optional setup
}

export function syncerInitTab() {
    const srcInput = document.getElementById('syncerSourceFolder');
    if (srcInput && !srcInput.value) {
        const convInput = document.getElementById('convFolder');
        if (convInput && convInput.value) {
            srcInput.value = convInput.value;
            const targetInput = document.getElementById('syncerTargetFolder');
            if (targetInput && !targetInput.value) {
                targetInput.value = `${convInput.value.replace(/\/+$/, '')}_mirror`;
            }
        }
    }
}

export async function syncerPlan() {
    const sourceFolder = document.getElementById('syncerSourceFolder')?.value.trim() || '';
    const targetFolder = document.getElementById('syncerTargetFolder')?.value.trim() || '';

    if (!sourceFolder || !targetFolder) {
        UI.showToast('Please specify both source and destination folders', 'warning');
        return;
    }

    if (sourceFolder === targetFolder) {
        UI.showToast('Source and destination cannot be identical', 'warning');
        return;
    }

    const mode = document.getElementById('syncerMode')?.value || 'one-way';
    const deleteExtra = document.getElementById('syncerDeleteExtraneous')?.checked || false;

    const planBtn = document.getElementById('syncerPlanBtn');
    const planCard = document.getElementById('syncerPlanCard');
    const progressCard = document.getElementById('syncerProgressCard');
    const progressFill = document.getElementById('syncerProgressFill');
    const progressMsg = document.getElementById('syncerProgressMsg');

    if (planCard) planCard.style.display = 'none';
    if (progressCard) progressCard.style.display = 'block';
    if (progressFill) progressFill.style.width = '30%';
    if (progressMsg) progressMsg.textContent = 'Comparing directories…';
    if (planBtn) planBtn.disabled = true;

    try {
        const res = await fetch('/api/syncer/plan', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                source_folder: sourceFolder,
                target_folder: targetFolder,
                mode,
                delete_extraneous: deleteExtra
            })
        });

        if (!res.ok) {
            const err = await res.json().catch(() => ({ error: 'Sync planning failed' }));
            throw new Error(err.error || `HTTP ${res.status}`);
        }

        const data = await res.json();
        currentPlanId = data.plan_id;
        currentPlanDiff = Array.isArray(data.diff) ? data.diff : [];

        renderSyncPlan(data);
        UI.showToast(`Sync plan generated: ${currentPlanDiff.length} file actions`, 'success');
    } catch (err) {
        console.error('Syncer plan error:', err);
        UI.showToast(`Planning error: ${err.message}`, 'error');
    } finally {
        if (progressCard) progressCard.style.display = 'none';
        if (planBtn) planBtn.disabled = false;
    }
}

function renderSyncPlan(plan) {
    const planCard = document.getElementById('syncerPlanCard');
    const tbody = document.getElementById('syncerDiffTableBody');
    if (!planCard || !tbody) return;

    planCard.style.display = 'block';

    const stats = plan.stats || {};
    document.getElementById('syncerKpiCopy').textContent = (stats.to_copy || 0).toLocaleString();
    document.getElementById('syncerKpiUpdate').textContent = (stats.to_update || 0).toLocaleString();
    document.getElementById('syncerKpiDelete').textContent = (stats.to_delete || 0).toLocaleString();
    document.getElementById('syncerKpiConflict').textContent = (stats.conflicts || 0).toLocaleString();

    if (currentPlanDiff.length === 0) {
        tbody.innerHTML = '<tr><td colspan="5" style="text-align:center; padding:20px; color:var(--text-dim);">Directories are already identical. No sync actions required.</td></tr>';
        return;
    }

    tbody.innerHTML = currentPlanDiff.map(item => {
        let badgeClass = 'diff-copy';
        let actionLabel = '+ NEW';

        if (item.action === 'update') {
            badgeClass = 'diff-update';
            actionLabel = '~ UPDATE';
        } else if (item.action === 'delete') {
            badgeClass = 'diff-delete';
            actionLabel = '- DELETE';
        } else if (item.action === 'conflict') {
            badgeClass = 'diff-conflict';
            actionLabel = '! CONFLICT';
        }

        const srcInfo = item.source_size !== undefined ? `${UI.formatFileSize(item.source_size)} / ${item.source_mtime || '-'}` : '-';
        const targetInfo = item.target_size !== undefined ? `${UI.formatFileSize(item.target_size)} / ${item.target_mtime || '-'}` : '-';

        return `
            <tr>
                <td><span class="diff-badge ${badgeClass}">${actionLabel}</span></td>
                <td>
                    <div style="font-weight:600;">${item.path.split('/').pop()}</div>
                    <div class="mono-text" style="font-size:11px;">${item.path}</div>
                </td>
                <td style="font-size:12px;">${srcInfo}</td>
                <td style="font-size:12px;">${targetInfo}</td>
                <td style="font-size:12px; color:var(--text-dim);">${item.reason || '-'}</td>
            </tr>
        `;
    }).join('');
}

export async function syncerExecute() {
    if (!currentPlanId) {
        UI.showToast('No active sync plan to execute', 'warning');
        return;
    }

    const confirmed = confirm('Are you sure you want to execute this sync plan? Files may be copied, updated, or deleted according to the plan.');
    if (!confirmed) return;

    const execBtn = document.getElementById('syncerExecBtn');
    const progressCard = document.getElementById('syncerProgressCard');
    const progressFill = document.getElementById('syncerProgressFill');
    const progressMsg = document.getElementById('syncerProgressMsg');
    const progressPct = document.getElementById('syncerProgressPct');

    if (progressCard) progressCard.style.display = 'block';
    if (progressFill) progressFill.style.width = '10%';
    if (progressMsg) progressMsg.textContent = 'Executing sync plan…';
    if (progressPct) progressPct.textContent = '0%';
    if (execBtn) execBtn.disabled = true;

    try {
        const res = await fetch('/api/syncer/execute', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ plan_id: currentPlanId })
        });

        if (!res.ok) {
            const err = await res.json().catch(() => ({ error: 'Sync execution failed' }));
            throw new Error(err.error || `HTTP ${res.status}`);
        }

        const data = await res.json();
        if (data.job_id) {
            syncerJobId = data.job_id;
            pollSyncJob(data.job_id);
        } else {
            if (progressCard) progressCard.style.display = 'none';
            UI.showToast('Synchronization completed successfully', 'success');
        }
    } catch (err) {
        console.error('Syncer execute error:', err);
        UI.showToast(`Execution error: ${err.message}`, 'error');
        if (progressCard) progressCard.style.display = 'none';
    } finally {
        if (execBtn) execBtn.disabled = false;
    }
}

function pollSyncJob(jobId) {
    clearInterval(syncerPollTimer);
    const progressFill = document.getElementById('syncerProgressFill');
    const progressMsg = document.getElementById('syncerProgressMsg');
    const progressPct = document.getElementById('syncerProgressPct');
    const progressCard = document.getElementById('syncerProgressCard');

    syncerPollTimer = setInterval(async () => {
        try {
            const res = await fetch(`/api/syncer/status/${jobId}`);
            if (!res.ok) return;

            const job = await res.json();
            const pct = job.total ? Math.round((job.processed / job.total) * 100) : 0;
            if (progressFill) progressFill.style.width = `${pct}%`;
            if (progressPct) progressPct.textContent = `${pct}%`;
            if (progressMsg) progressMsg.textContent = `Syncing: ${job.processed || 0} / ${job.total || 0} files`;

            if (job.status === 'completed' || job.status === 'failed' || job.status === 'cancelled') {
                clearInterval(syncerPollTimer);
                if (progressCard) progressCard.style.display = 'none';

                if (job.status === 'completed') {
                    UI.showToast('Sync completed successfully', 'success');
                } else if (job.status === 'cancelled') {
                    UI.showToast('Sync cancelled', 'info');
                } else {
                    UI.showToast(`Sync failed: ${job.error || 'Unknown error'}`, 'error');
                }
            }
        } catch (_) {}
    }, 600);
}

export async function syncerCancel() {
    if (!syncerJobId) return;
    try {
        await fetch(`/api/syncer/cancel/${syncerJobId}`, { method: 'POST' });
        clearInterval(syncerPollTimer);
        const progressCard = document.getElementById('syncerProgressCard');
        if (progressCard) progressCard.style.display = 'none';
        UI.showToast('Cancelling sync…', 'info');
    } catch (err) {
        console.error('Cancel syncer error:', err);
    }
}
