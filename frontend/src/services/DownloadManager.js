
import wsService from './websocket.js'

class DownloadManager {
    constructor() {
        this.queue = new Set()
        this.processing = false
        this.batchSize = 5
        this.interval = null
        this.priorityId = null
    }

    // Add IDs to the download queue
    enqueue(ids) {
        if (!ids || ids.length === 0) return
        ids.forEach(id => this.queue.add(id))
        this.startProcessing()
    }

    // prioritizing a specific result (e.g., currently viewed question)
    prioritize(id) {
        if (!id) return
        this.priorityId = id
        // If it's already in the queue, we want to fetch it ASAP
        if (this.queue.has(id)) {
            // Trigger immediate process check
            this.processQueue()
        }
    }

    // Clear everything (e.g., when switching runs)
    cancelAll() {
        this.queue.clear()
        this.priorityId = null
        this.processing = false
        if (this.interval) {
            clearInterval(this.interval)
            this.interval = null
        }
    }

    startProcessing() {
        if (this.interval) return
        // Process every 200ms to avoid flooding but keep it snappy
        this.interval = setInterval(() => this.processQueue(), 200)
    }

    async processQueue() {
        if (this.processing || this.queue.size === 0) return

        this.processing = true
        const batch = []
        let consumePriority = false
        try {
            // 1. Pick priority item if exists and needed
            if (this.priorityId && this.queue.has(this.priorityId)) {
                batch.push(this.priorityId)
                consumePriority = true
            }

            // 2. Fill rest of batch
            for (const id of this.queue) {
                if (batch.length >= this.batchSize) break
                if (id === this.priorityId) continue
                batch.push(id)
            }

            if (batch.length > 0) {
                console.log('[DownloadManager] Fetching batch:', batch.length, 'items')
                await wsService.getResultDetails(batch)
                batch.forEach(id => this.queue.delete(id))
                if (consumePriority) {
                    this.priorityId = null
                }
            } else {
                // Queue empty, stop interval
                if (this.interval) {
                    clearInterval(this.interval)
                    this.interval = null
                }
            }
        } catch (e) {
            console.error('[DownloadManager] Failed to process batch:', e)
        } finally {
            this.processing = false
            if (this.queue.size === 0 && this.interval) {
                clearInterval(this.interval)
                this.interval = null
            }
        }
    }
}

export const downloadManager = new DownloadManager()
