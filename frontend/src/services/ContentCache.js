
// ContentCache.js
// Stores run result content keying by content hash.
// This allows deduplication across different runs/agents if the answer is identical.

class ContentCache {
    constructor() {
        this.cache = new Map()
    }

    has(hash) {
        if (!hash) return false
        return this.cache.has(hash)
    }

    get(hash) {
        if (!hash) return null
        return this.cache.get(hash)
    }

    set(hash, data) {
        if (!hash || !data) return
        this.cache.set(hash, data)
    }

    clear() {
        this.cache.clear()
    }
}

export const contentCache = new ContentCache()
