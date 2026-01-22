
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
        try {
            const batch = []

            // 1. Pick priority item if exists and needed
            if (this.priorityId && this.queue.has(this.priorityId)) {
                batch.push(this.priorityId)
                this.queue.delete(this.priorityId)
                this.priorityId = null // Consumed
            }

            // 2. Fill rest of batch
            for (const id of this.queue) {
                if (batch.length >= this.batchSize) break
                batch.push(id)
                this.queue.delete(id)
            }

            if (batch.length > 0) {
                console.log('[DownloadManager] Fetching batch:', batch.length, 'items')
                await wsService.getResultDetails(batch)
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
        }
    }
}

export const downloadManager = new DownloadManager()
