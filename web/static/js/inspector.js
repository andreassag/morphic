// ── Inspector Module: Storage Analytics & Interactive Visualizer ────────

import * as UI from './ui.js';

let inspectorData = null;
let currentRootNode = null;
let nodeHistory = [];
let vizMode = 'treemap'; // 'treemap' or 'sunburst'
let colorMode = 'type'; // 'type', 'size', 'age'
let hoveredNode = null;
let canvasRects = []; // for treemap hit testing
let canvasArcs = []; // for sunburst hit testing

const EXT_COLORS = {
    jpg: '#6c7bf7', jpeg: '#6c7bf7', png: '#38bdf8', webp: '#06b6d4',
    gif: '#818cf8', svg: '#c084fc', tiff: '#a78bfa',
    mp4: '#fbbf24', mkv: '#f59e0b', mov: '#d97706', avi: '#b45309', webm: '#fcd34d',
    mp3: '#4ade80', flac: '#22c55e', wav: '#16a34a',
    zip: '#f43f5e', rar: '#e11d48', '7z': '#be123c', tar: '#9f1239', gz: '#881337',
    other: '#64748b', dir: '#1e293b'
};

export function initInspector() {
    window.addEventListener('resize', () => {
        if (currentRootNode) renderVisualization();
    });

    const canvas = document.getElementById('inspectorCanvas');
    if (canvas) {
        canvas.addEventListener('mousemove', onCanvasMouseMove);
        canvas.addEventListener('mouseleave', onCanvasMouseLeave);
        canvas.addEventListener('click', onCanvasClick);
    }
}

export function inspectorInitTab() {
    const input = document.getElementById('inspectorFolder');
    if (input && !input.value) {
        const convInput = document.getElementById('convFolder');
        if (convInput && convInput.value) {
            input.value = convInput.value;
        }
    }
}

export async function inspectorAnalyze() {
    const folderInput = document.getElementById('inspectorFolder');
    const folder = folderInput ? folderInput.value.trim() : '';
    if (!folder) {
        UI.showToast('Please specify a folder to analyze', 'warning');
        return;
    }

    const progressCard = document.getElementById('inspectorProgressCard');
    const analyzeBtn = document.getElementById('inspectorAnalyzeBtn');
    if (progressCard) progressCard.style.display = 'block';
    if (analyzeBtn) analyzeBtn.disabled = true;

    try {
        const res = await fetch('/api/inspector/analyze', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ folder })
        });

        if (!res.ok) {
            const err = await res.json().catch(() => ({ error: 'Analysis failed' }));
            throw new Error(err.error || `HTTP ${res.status}`);
        }

        const data = await res.json();
        inspectorData = data;
        currentRootNode = data.tree || data;
        nodeHistory = [currentRootNode];

        renderKPIs(data);
        renderBreadcrumbs();
        renderFormatBar(data.stats || {});
        renderHighlights(data.stats || {});

        const kpisEl = document.getElementById('inspectorKpis');
        const vizCard = document.getElementById('inspectorVizCard');
        const detailRow = document.getElementById('inspectorDetailRow');
        if (kpisEl) kpisEl.style.display = 'grid';
        if (vizCard) vizCard.style.display = 'block';
        if (detailRow) detailRow.style.display = 'flex';

        renderVisualization();
        UI.showToast('Storage analysis completed', 'success');
    } catch (err) {
        console.error('Inspector analyze error:', err);
        UI.showToast(`Analysis error: ${err.message}`, 'error');
    } finally {
        if (progressCard) progressCard.style.display = 'none';
        if (analyzeBtn) analyzeBtn.disabled = false;
    }
}

function renderKPIs(data) {
    const stats = data.stats || {};
    const totalSize = stats.total_size || data.tree?.size || 0;
    const totalFiles = stats.total_files || data.tree?.count || 0;

    const totalSizeEl = document.getElementById('inspectorTotalSize');
    const totalFilesEl = document.getElementById('inspectorTotalFiles');
    const imgSizeEl = document.getElementById('inspectorImagesSize');
    const imgCountEl = document.getElementById('inspectorImagesCount');
    const vidSizeEl = document.getElementById('inspectorVideosSize');
    const vidCountEl = document.getElementById('inspectorVideosCount');

    if (totalSizeEl) totalSizeEl.textContent = UI.formatFileSize(totalSize);
    if (totalFilesEl) totalFilesEl.textContent = totalFiles.toLocaleString();

    if (imgSizeEl) imgSizeEl.textContent = UI.formatFileSize(stats.images_size || 0);
    if (imgCountEl) imgCountEl.textContent = `${(stats.images_count || 0).toLocaleString()} files`;

    if (vidSizeEl) vidSizeEl.textContent = UI.formatFileSize(stats.videos_size || 0);
    if (vidCountEl) vidCountEl.textContent = `${(stats.videos_count || 0).toLocaleString()} files`;
}

function renderFormatBar(stats) {
    const formatBar = document.getElementById('inspectorFormatBar');
    const formatLegend = document.getElementById('inspectorFormatLegend');
    if (!formatBar || !formatLegend) return;

    const breakdown = stats.format_breakdown || {};
    const entries = Object.entries(breakdown).sort((a, b) => b[1].size - a[1].size);
    const totalSize = stats.total_size || 1;

    formatBar.innerHTML = entries.map(([ext, info]) => {
        const pct = Math.max(1, (info.size / totalSize) * 100);
        const color = EXT_COLORS[ext.toLowerCase()] || EXT_COLORS.other;
        return `<div class="format-segment" style="width:${pct}%; background:${color};" title=".${ext}: ${UI.formatFileSize(info.size)} (${pct.toFixed(1)}%)"></div>`;
    }).join('');

    formatLegend.innerHTML = entries.slice(0, 7).map(([ext, info]) => {
        const color = EXT_COLORS[ext.toLowerCase()] || EXT_COLORS.other;
        return `
            <div class="legend-item">
                <span class="legend-color" style="background:${color};"></span>
                <span>.${ext} (${UI.formatFileSize(info.size)})</span>
            </div>
        `;
    }).join('');
}

function renderHighlights(stats) {
    const largestTable = document.getElementById('inspectorLargestTable');
    const oldestTable = document.getElementById('inspectorOldestTable');

    if (largestTable) {
        const largest = stats.largest_files || [];
        if (largest.length === 0) {
            largestTable.innerHTML = '<tr><td colspan="5" style="text-align:center;color:var(--text-dim);">No data</td></tr>';
        } else {
            largestTable.innerHTML = largest.map((f, i) => `
                <tr>
                    <td style="color:var(--text-dim);">${i + 1}</td>
                    <td>
                        <div style="font-weight:600;">${f.path.split('/').pop()}</div>
                        <div class="mono-text" style="font-size:10px;">${f.path}</div>
                    </td>
                    <td style="font-weight:700;">${UI.formatFileSize(f.size)}</td>
                    <td><span class="badge badge-info">${f.type || 'file'}</span></td>
                    <td style="text-align:right;">
                        <button class="btn btn-ghost btn-sm" onclick="openPreview('${f.path.replace(/'/g, "\\'")}')">👁️</button>
                    </td>
                </tr>
            `).join('');
        }
    }

    if (oldestTable) {
        const oldest = stats.oldest_files || [];
        if (oldest.length === 0) {
            oldestTable.innerHTML = '<tr><td colspan="5" style="text-align:center;color:var(--text-dim);">No data</td></tr>';
        } else {
            oldestTable.innerHTML = oldest.map((f, i) => `
                <tr>
                    <td style="color:var(--text-dim);">${i + 1}</td>
                    <td>
                        <div style="font-weight:600;">${f.path.split('/').pop()}</div>
                        <div class="mono-text" style="font-size:10px;">${f.path}</div>
                    </td>
                    <td style="font-size:12px; white-space:nowrap;">${f.mod_time || '-'}</td>
                    <td>${UI.formatFileSize(f.size)}</td>
                    <td style="text-align:right;">
                        <button class="btn btn-ghost btn-sm" onclick="openPreview('${f.path.replace(/'/g, "\\'")}')">👁️</button>
                    </td>
                </tr>
            `).join('');
        }
    }
}

function renderBreadcrumbs() {
    const breadcrumbs = document.getElementById('inspectorBreadcrumbs');
    if (!breadcrumbs) return;

    breadcrumbs.innerHTML = nodeHistory.map((node, index) => {
        const isCurrent = index === nodeHistory.length - 1;
        const name = node.name || (index === 0 ? 'Root' : 'Dir');
        return `
            <span class="viz-crumb ${isCurrent ? 'current' : ''}" onclick="inspectorJumpBreadcrumb(${index})">
                ${name}
            </span>
            ${!isCurrent ? '<span style="color:var(--text-muted);">/</span>' : ''}
        `;
    }).join('');
}

export function inspectorJumpBreadcrumb(index) {
    if (index >= 0 && index < nodeHistory.length) {
        nodeHistory = nodeHistory.slice(0, index + 1);
        currentRootNode = nodeHistory[index];
        renderBreadcrumbs();
        renderVisualization();
    }
}

export function inspectorResetZoom() {
    if (nodeHistory.length > 0) {
        inspectorJumpBreadcrumb(0);
    }
}

export function inspectorSetMode(mode) {
    vizMode = mode;
    document.getElementById('btnVizTreemap')?.classList.toggle('active', mode === 'treemap');
    document.getElementById('btnVizSunburst')?.classList.toggle('active', mode === 'sunburst');
    renderVisualization();
}

export function inspectorSetColor(color) {
    colorMode = color;
    renderVisualization();
}

function getNodeColor(node) {
    if (colorMode === 'type') {
        const ext = (node.name || '').split('.').pop().toLowerCase();
        if (node.children && node.children.length > 0) {
            return '#1e293b';
        }
        return EXT_COLORS[ext] || EXT_COLORS.other;
    }
    if (colorMode === 'size') {
        const mb = (node.size || 0) / (1024 * 1024);
        if (mb > 100) return '#f43f5e';
        if (mb > 25) return '#fbbf24';
        if (mb > 5) return '#6c7bf7';
        return '#38bdf8';
    }
    // Age
    return '#818cf8';
}

function renderVisualization() {
    if (!currentRootNode) return;
    const canvas = document.getElementById('inspectorCanvas');
    if (!canvas) return;

    const ctx = canvas.getContext('2d');
    const container = canvas.parentElement;
    const width = container.clientWidth || 800;
    const height = container.clientHeight || 520;
    const dpr = window.devicePixelRatio || 1;

    canvas.width = width * dpr;
    canvas.height = height * dpr;
    ctx.scale(dpr, dpr);

    ctx.clearRect(0, 0, width, height);

    if (vizMode === 'treemap') {
        canvasRects = [];
        canvasArcs = [];
        renderTreemap(ctx, currentRootNode, 0, 0, width, height);
    } else {
        canvasRects = [];
        canvasArcs = [];
        renderSunburst(ctx, currentRootNode, width / 2, height / 2, Math.min(width, height) / 2 - 20);
    }
}

// ── Squarified Treemap ─────────────────────────────────────────────────

function renderTreemap(ctx, node, x, y, w, h) {
    if (w < 2 || h < 2) return;

    const children = (node.children || []).slice().sort((a, b) => (b.size || 0) - (a.size || 0));

    if (children.length === 0) {
        // Leaf rect
        const color = getNodeColor(node);
        ctx.fillStyle = color;
        ctx.fillRect(x + 1, y + 1, w - 2, h - 2);

        ctx.strokeStyle = '#0f1117';
        ctx.lineWidth = 1;
        ctx.strokeRect(x + 0.5, y + 0.5, w - 1, h - 1);

        canvasRects.push({ node, x, y, w, h });

        if (w > 45 && h > 20) {
            ctx.fillStyle = '#ffffff';
            ctx.font = '11px -apple-system, sans-serif';
            ctx.fillText(node.name || '', x + 6, y + 15, w - 10);
            if (h > 36) {
                ctx.fillStyle = 'rgba(255,255,255,0.7)';
                ctx.font = '10px -apple-system, sans-serif';
                ctx.fillText(UI.formatFileSize(node.size), x + 6, y + 28, w - 10);
            }
        }
        return;
    }

    // Layout children using slice & dice / squarify
    const totalSize = children.reduce((acc, c) => acc + (c.size || 1), 0) || 1;
    let currX = x;
    let currY = y;
    const isHorizontal = w >= h;

    children.forEach(child => {
        const ratio = (child.size || 1) / totalSize;
        let childW, childH;

        if (isHorizontal) {
            childW = w * ratio;
            childH = h;
            renderTreemap(ctx, child, currX, currY, childW, childH);
            currX += childW;
        } else {
            childW = w;
            childH = h * ratio;
            renderTreemap(ctx, child, currX, currY, childW, childH);
            currY += childH;
        }

        canvasRects.push({ node: child, x: isHorizontal ? (currX - childW) : x, y: isHorizontal ? y : (currY - childH), w: childW, h: childH });
    });
}

// ── Interactive Sunburst ───────────────────────────────────────────────

function renderSunburst(ctx, rootNode, cx, cy, maxRadius) {
    const totalSize = rootNode.size || 1;
    const levels = 4;
    const ringWidth = maxRadius / (levels + 1);

    // Center root circle
    ctx.beginPath();
    ctx.arc(cx, cy, ringWidth, 0, Math.PI * 2);
    ctx.fillStyle = '#1e293b';
    ctx.fill();
    ctx.strokeStyle = '#0f1117';
    ctx.lineWidth = 2;
    ctx.stroke();

    ctx.fillStyle = '#ffffff';
    ctx.font = 'bold 12px -apple-system, sans-serif';
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    ctx.fillText(rootNode.name || 'Root', cx, cy - 8);
    ctx.fillStyle = 'var(--text-dim)';
    ctx.font = '10px -apple-system, sans-serif';
    ctx.fillText(UI.formatFileSize(rootNode.size), cx, cy + 8);

    canvasArcs.push({ node: rootNode, r0: 0, r1: ringWidth, a0: 0, a1: Math.PI * 2, cx, cy });

    function drawRing(nodes, depth, startAngle, endAngle) {
        if (depth > levels || !nodes || nodes.length === 0) return;
        const total = nodes.reduce((sum, n) => sum + (n.size || 1), 0) || 1;
        const span = endAngle - startAngle;
        let curAngle = startAngle;

        const r0 = depth * ringWidth;
        const r1 = (depth + 1) * ringWidth;

        nodes.forEach(node => {
            const arcSpan = span * ((node.size || 1) / total);
            const a0 = curAngle;
            const a1 = curAngle + arcSpan;
            curAngle += arcSpan;

            if (arcSpan > 0.02) {
                ctx.beginPath();
                ctx.arc(cx, cy, r1, a0, a1);
                ctx.arc(cx, cy, r0, a1, a0, true);
                ctx.closePath();

                ctx.fillStyle = getNodeColor(node);
                ctx.fill();
                ctx.strokeStyle = '#0f1117';
                ctx.lineWidth = 1.5;
                ctx.stroke();

                canvasArcs.push({ node, r0, r1, a0, a1, cx, cy });

                if (node.children && node.children.length > 0) {
                    drawRing(node.children, depth + 1, a0, a1);
                }
            }
        });
    }

    if (rootNode.children && rootNode.children.length > 0) {
        drawRing(rootNode.children, 1, 0, Math.PI * 2);
    }
}

// ── Hit Testing & Tooltips ─────────────────────────────────────────────

function onCanvasMouseMove(e) {
    const canvas = e.currentTarget;
    const rect = canvas.getBoundingClientRect();
    const x = e.clientX - rect.left;
    const y = e.clientY - rect.top;
    const tooltip = document.getElementById('inspectorTooltip');

    let hit = null;

    if (vizMode === 'treemap') {
        for (let i = canvasRects.length - 1; i >= 0; i--) {
            const r = canvasRects[i];
            if (x >= r.x && x <= r.x + r.w && y >= r.y && y <= r.y + r.h) {
                hit = r.node;
                break;
            }
        }
    } else {
        for (let i = canvasArcs.length - 1; i >= 0; i--) {
            const arc = canvasArcs[i];
            const dx = x - arc.cx;
            const dy = y - arc.cy;
            const dist = Math.sqrt(dx * dx + dy * dy);
            if (dist >= arc.r0 && dist <= arc.r1) {
                let angle = Math.atan2(dy, dx);
                if (angle < 0) angle += Math.PI * 2;
                if (angle >= arc.a0 && angle <= arc.a1) {
                    hit = arc.node;
                    break;
                }
            }
        }
    }

    if (hit && tooltip) {
        tooltip.style.display = 'block';
        tooltip.style.left = `${Math.min(x + 16, rect.width - 200)}px`;
        tooltip.style.top = `${Math.min(y + 16, rect.height - 80)}px`;
        const pct = currentRootNode.size ? ((hit.size / currentRootNode.size) * 100).toFixed(1) : 0;
        tooltip.innerHTML = `
            <strong>${hit.name}</strong>
            <div>Size: ${UI.formatFileSize(hit.size)} (${pct}%)</div>
            ${hit.count ? `<div>Files: ${hit.count.toLocaleString()}</div>` : ''}
            <div class="mono-text" style="font-size:10px; margin-top:2px;">${hit.path || ''}</div>
        `;
    } else if (tooltip) {
        tooltip.style.display = 'none';
    }
}

function onCanvasMouseLeave() {
    const tooltip = document.getElementById('inspectorTooltip');
    if (tooltip) tooltip.style.display = 'none';
}

function onCanvasClick(e) {
    const canvas = e.currentTarget;
    const rect = canvas.getBoundingClientRect();
    const x = e.clientX - rect.left;
    const y = e.clientY - rect.top;

    let hit = null;

    if (vizMode === 'treemap') {
        for (let i = canvasRects.length - 1; i >= 0; i--) {
            const r = canvasRects[i];
            if (x >= r.x && x <= r.x + r.w && y >= r.y && y <= r.y + r.h) {
                hit = r.node;
                break;
            }
        }
    } else {
        for (let i = canvasArcs.length - 1; i >= 0; i--) {
            const arc = canvasArcs[i];
            const dx = x - arc.cx;
            const dy = y - arc.cy;
            const dist = Math.sqrt(dx * dx + dy * dy);
            if (dist >= arc.r0 && dist <= arc.r1) {
                let angle = Math.atan2(dy, dx);
                if (angle < 0) angle += Math.PI * 2;
                if (angle >= arc.a0 && angle <= arc.a1) {
                    hit = arc.node;
                    break;
                }
            }
        }
    }

    if (hit) {
        if (hit === currentRootNode) {
            // Clicked center in sunburst: zoom out to parent
            if (nodeHistory.length > 1) {
                nodeHistory.pop();
                currentRootNode = nodeHistory[nodeHistory.length - 1];
                renderBreadcrumbs();
                renderVisualization();
            }
        } else if (hit.children && hit.children.length > 0) {
            // Zoom into directory node
            nodeHistory.push(hit);
            currentRootNode = hit;
            renderBreadcrumbs();
            renderVisualization();
        } else if (hit.path) {
            // Leaf media file: open preview
            UI.openPreview(hit.path);
        }
    }
}
