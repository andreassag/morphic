// ── Gallery Module: Responsive Media Browser & Lightbox ───────────────

import * as UI from './ui.js';

let galleryItems = [];
let filteredItems = [];
let currentItemIndex = -1;
let currentFilter = 'all'; // 'all', 'images', 'videos'
let currentSort = 'date-desc';
let zoomScale = 1.0;
let panX = 0;
let panY = 0;
let isPanning = false;
let startX = 0;
let startY = 0;
let lazyObserver = null;

export function initGallery() {
    initLightboxListeners();
    initIntersectionObserver();
}

export function galleryInitTab() {
    const input = document.getElementById('galleryFolder');
    if (input && !input.value) {
        const convInput = document.getElementById('convFolder');
        if (convInput && convInput.value) {
            input.value = convInput.value;
        }
    }
}

function initIntersectionObserver() {
    if ('IntersectionObserver' in window) {
        lazyObserver = new IntersectionObserver((entries, observer) => {
            entries.forEach(entry => {
                if (entry.isIntersecting) {
                    const img = entry.target.querySelector('img[data-src]');
                    if (img) {
                        img.src = img.getAttribute('data-src');
                        img.removeAttribute('data-src');
                    }
                    observer.unobserve(entry.target);
                }
            });
        }, { rootMargin: '200px 0px' });
    }
}

export async function galleryLoad() {
    const folderInput = document.getElementById('galleryFolder');
    const folder = folderInput ? folderInput.value.trim() : '';
    if (!folder) {
        UI.showToast('Please specify a media folder to load', 'warning');
        return;
    }

    const loadBtn = document.getElementById('galleryLoadBtn');
    if (loadBtn) loadBtn.disabled = true;

    try {
        const res = await fetch('/api/gallery/items', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ folder })
        });

        if (!res.ok) {
            // Try fallback GET /api/gallery/items?folder=...
            const fallbackRes = await fetch(`/api/gallery/items?folder=${encodeURIComponent(folder)}`);
            if (!fallbackRes.ok) {
                const err = await res.json().catch(() => ({ error: 'Gallery load failed' }));
                throw new Error(err.error || `HTTP ${res.status}`);
            }
            const data = await fallbackRes.json();
            galleryItems = Array.isArray(data.items) ? data.items : [];
        } else {
            const data = await res.json();
            galleryItems = Array.isArray(data.items) ? data.items : [];
        }

        applyFiltersAndSort();
        const toolbar = document.getElementById('galleryToolbar');
        if (toolbar) toolbar.style.display = 'flex';

        UI.showToast(`Loaded ${galleryItems.length} media items`, 'success');
    } catch (err) {
        console.error('Gallery load error:', err);
        UI.showToast(`Error loading gallery: ${err.message}`, 'error');
    } finally {
        if (loadBtn) loadBtn.disabled = false;
    }
}

function applyFiltersAndSort() {
    let items = galleryItems.slice();

    // 1. Filter by type
    if (currentFilter === 'images') {
        items = items.filter(i => isImage(i.path));
    } else if (currentFilter === 'videos') {
        items = items.filter(i => isVideo(i.path));
    }

    // 2. Filter by search text
    const searchVal = document.getElementById('gallerySearch')?.value.toLowerCase().trim() || '';
    if (searchVal) {
        items = items.filter(i => (i.name || i.path).toLowerCase().includes(searchVal));
    }

    // 3. Sort
    items.sort((a, b) => {
        if (currentSort === 'date-desc') return new Date(b.mod_time || 0) - new Date(a.mod_time || 0);
        if (currentSort === 'date-asc') return new Date(a.mod_time || 0) - new Date(b.mod_time || 0);
        if (currentSort === 'size-desc') return (b.size || 0) - (a.size || 0);
        if (currentSort === 'size-asc') return (a.size || 0) - (b.size || 0);
        if (currentSort === 'name-asc') return (a.name || '').localeCompare(b.name || '');
        return 0;
    });

    filteredItems = items;
    renderGalleryGrid();
}

function renderGalleryGrid() {
    const grid = document.getElementById('galleryGrid');
    const emptyState = document.getElementById('galleryEmptyState');
    const countBadge = document.getElementById('galleryItemCount');

    if (!grid) return;

    if (countBadge) countBadge.textContent = `${filteredItems.length} items`;

    if (filteredItems.length === 0) {
        grid.innerHTML = '';
        if (emptyState) emptyState.style.display = 'block';
        return;
    }

    if (emptyState) emptyState.style.display = 'none';

    grid.innerHTML = filteredItems.map((item, idx) => {
        const ext = item.path.split('.').pop().toUpperCase();
        const thumbUrl = `/api/thumbnail?path=${encodeURIComponent(item.path)}`;
        const sizeStr = UI.formatFileSize(item.size);
        const name = item.name || item.path.split('/').pop();

        return `
            <div class="gallery-card" onclick="openLightbox(${idx})">
                <div class="gallery-thumb-wrap">
                    <img class="gallery-thumb" data-src="${thumbUrl}" alt="${name}" loading="lazy" onerror="this.style.opacity='0.2'" />
                    <span class="gallery-badge">${ext}</span>
                </div>
                <div class="gallery-card-info">
                    <div class="gallery-card-title" title="${name}">${name}</div>
                    <div class="gallery-card-meta">
                        <span>${sizeStr}</span>
                        <span>${item.width ? `${item.width}×${item.height}` : ''}</span>
                    </div>
                </div>
            </div>
        `;
    }).join('');

    // Attach IntersectionObserver for lazy loading
    if (lazyObserver) {
        grid.querySelectorAll('.gallery-card').forEach(card => {
            lazyObserver.observe(card);
        });
    } else {
        // Fallback: load all
        grid.querySelectorAll('img[data-src]').forEach(img => {
            img.src = img.getAttribute('data-src');
        });
    }
}

export function gallerySetFilter(filter) {
    currentFilter = filter;
    document.getElementById('btnFilterAll')?.classList.toggle('active', filter === 'all');
    document.getElementById('btnFilterImages')?.classList.toggle('active', filter === 'images');
    document.getElementById('btnFilterVideos')?.classList.toggle('active', filter === 'videos');
    applyFiltersAndSort();
}

export function galleryFilterByName() {
    applyFiltersAndSort();
}

export function gallerySetSort(sortVal) {
    currentSort = sortVal;
    applyFiltersAndSort();
}

export function gallerySetGridSize(size) {
    const grid = document.getElementById('galleryGrid');
    if (!grid) return;
    grid.classList.remove('compact', 'large');
    if (size !== 'normal') grid.classList.add(size);
}

// ── Lightbox Implementation ────────────────────────────────────────────

export function openLightbox(index) {
    if (index < 0 || index >= filteredItems.length) return;
    currentItemIndex = index;
    const item = filteredItems[index];

    const overlay = document.getElementById('lightboxOverlay');
    const container = document.getElementById('lbMediaContainer');
    const counter = document.getElementById('lbCounter');
    const filename = document.getElementById('lbFilename');
    const sizeEl = document.getElementById('lbSize');
    const pathEl = document.getElementById('lbPath');
    const dimsEl = document.getElementById('lbDimensions');
    const download = document.getElementById('lbDownload');

    if (!overlay || !container) return;

    // Reset zoom/pan
    zoomScale = 1.0;
    panX = 0;
    panY = 0;

    const name = item.name || item.path.split('/').pop();
    if (counter) counter.textContent = `${index + 1} / ${filteredItems.length}`;
    if (filename) {
        filename.textContent = name;
        filename.title = item.path;
    }
    if (sizeEl) sizeEl.textContent = UI.formatFileSize(item.size);
    if (pathEl) pathEl.textContent = item.path;
    if (dimsEl) dimsEl.textContent = item.width ? `${item.width} × ${item.height} px` : '';
    if (download) {
        download.href = `/api/media?path=${encodeURIComponent(item.path)}`;
        download.setAttribute('download', name);
    }

    container.innerHTML = '';

    if (isVideo(item.path)) {
        const video = document.createElement('video');
        video.src = `/api/media?path=${encodeURIComponent(item.path)}`;
        video.controls = true;
        video.autoplay = true;
        container.appendChild(video);
    } else {
        const img = document.createElement('img');
        img.src = `/api/media?path=${encodeURIComponent(item.path)}`;
        img.alt = name;
        img.id = 'lbActiveImage';
        container.appendChild(img);
        updateImageTransform();
    }

    overlay.classList.add('active');
    overlay.focus();
}

export function closeLightbox() {
    const overlay = document.getElementById('lightboxOverlay');
    const container = document.getElementById('lbMediaContainer');
    if (overlay) overlay.classList.remove('active');
    if (container) {
        const video = container.querySelector('video');
        if (video) video.pause();
        container.innerHTML = '';
    }
    currentItemIndex = -1;
}

export function nextLightboxItem() {
    if (filteredItems.length === 0) return;
    const nextIdx = (currentItemIndex + 1) % filteredItems.length;
    openLightbox(nextIdx);
}

export function prevLightboxItem() {
    if (filteredItems.length === 0) return;
    const prevIdx = (currentItemIndex - 1 + filteredItems.length) % filteredItems.length;
    openLightbox(prevIdx);
}

export function lightboxZoom(delta) {
    zoomScale = Math.min(5.0, Math.max(0.5, zoomScale + delta));
    updateImageTransform();
    const resetBtn = document.getElementById('lbZoomReset');
    if (resetBtn) resetBtn.textContent = `${Math.round(zoomScale * 100)}%`;
}

export function lightboxZoomReset() {
    zoomScale = 1.0;
    panX = 0;
    panY = 0;
    updateImageTransform();
    const resetBtn = document.getElementById('lbZoomReset');
    if (resetBtn) resetBtn.textContent = '100%';
}

function updateImageTransform() {
    const img = document.getElementById('lbActiveImage');
    if (img) {
        img.style.transform = `scale(${zoomScale}) translate(${panX / zoomScale}px, ${panY / zoomScale}px)`;
    }
}

function initLightboxListeners() {
    // Keyboard navigation
    document.addEventListener('keydown', (e) => {
        const overlay = document.getElementById('lightboxOverlay');
        if (!overlay || !overlay.classList.contains('active')) return;

        if (e.key === 'ArrowRight') {
            e.preventDefault();
            nextLightboxItem();
        } else if (e.key === 'ArrowLeft') {
            e.preventDefault();
            prevLightboxItem();
        } else if (e.key === 'Escape') {
            e.preventDefault();
            closeLightbox();
        } else if (e.key === '+' || e.key === '=') {
            e.preventDefault();
            lightboxZoom(0.25);
        } else if (e.key === '-' || e.key === '_') {
            e.preventDefault();
            lightboxZoom(-0.25);
        } else if (e.key === '0') {
            e.preventDefault();
            lightboxZoomReset();
        }
    });

    const body = document.getElementById('lbBody');
    if (body) {
        // Mouse wheel zoom
        body.addEventListener('wheel', (e) => {
            const overlay = document.getElementById('lightboxOverlay');
            if (!overlay || !overlay.classList.contains('active')) return;
            const img = document.getElementById('lbActiveImage');
            if (!img) return; // only zoom images

            e.preventDefault();
            const delta = e.deltaY < 0 ? 0.15 : -0.15;
            lightboxZoom(delta);
        }, { passive: false });

        // Mouse drag pan
        body.addEventListener('mousedown', (e) => {
            if (zoomScale <= 1.0) return;
            isPanning = true;
            startX = e.clientX - panX;
            startY = e.clientY - panY;
        });

        window.addEventListener('mousemove', (e) => {
            if (!isPanning) return;
            panX = e.clientX - startX;
            panY = e.clientY - startY;
            updateImageTransform();
        });

        window.addEventListener('mouseup', () => {
            isPanning = false;
        });
    }
}

function isImage(path) {
    const ext = path.split('.').pop().toLowerCase();
    return ['jpg', 'jpeg', 'png', 'webp', 'gif', 'bmp', 'svg', 'tiff', 'heic', 'avif'].includes(ext);
}

function isVideo(path) {
    const ext = path.split('.').pop().toLowerCase();
    return ['mp4', 'mkv', 'mov', 'avi', 'webm', 'flv', 'wmv', 'm4v'].includes(ext);
}
