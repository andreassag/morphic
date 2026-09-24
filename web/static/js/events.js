// ── Morphic Unified SSE Event Hub ─────────────────────────────────────

import { showToast } from './ui.js';

const listeners = new Map();
let eventSource = null;
let reconnectTimer = null;

export function onEvent(topic, callback) {
    if (!listeners.has(topic)) {
        listeners.set(topic, new Set());
    }
    listeners.get(topic).add(callback);
    return () => listeners.get(topic)?.delete(callback);
}

export function initEventSource() {
    if (eventSource) {
        eventSource.close();
    }

    const indicator = document.getElementById('sseIndicator');

    try {
        eventSource = new EventSource('/api/events/stream');

        eventSource.onopen = () => {
            if (indicator) {
                indicator.className = 'status-indicator online';
                indicator.textContent = '● Live';
            }
        };

        const topics = ['converter', 'dupfinder', 'organizer', 'watcher', 'trash'];
        topics.forEach(topic => {
            eventSource.addEventListener(topic, (e) => {
                try {
                    const parsed = JSON.parse(e.data);
                    const payload = parsed.payload !== undefined ? parsed.payload : parsed.data;
                    dispatch(topic, { ...parsed, payload, data: payload });
                } catch (err) {
                    console.error('Failed to parse SSE payload', err);
                }
            });
        });

        eventSource.onerror = () => {
            if (indicator) {
                indicator.className = 'status-indicator';
                indicator.textContent = '○ Offline';
            }
            eventSource.close();

            if (!reconnectTimer) {
                reconnectTimer = setTimeout(() => {
                    reconnectTimer = null;
                    initEventSource();
                }, 4000);
            }
        };
    } catch (err) {
        console.error('SSE initialization error', err);
    }
}

function dispatch(topic, eventData) {
    const topicListeners = listeners.get(topic);
    if (topicListeners) {
        topicListeners.forEach(cb => cb(eventData));
    }
    const wildcardListeners = listeners.get('*');
    if (wildcardListeners) {
        wildcardListeners.forEach(cb => cb({ topic, ...eventData }));
    }
}
