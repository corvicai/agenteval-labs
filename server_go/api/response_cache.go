package api

import (
	"sync"
	"time"
)

// CachedResponse stores a serialized response payload for a short period so
// clients can retrieve it after a brief network disconnect (Plan 24, Layer 4).
type CachedResponse struct {
	MsgType     string
	PayloadJSON []byte
	StoredAt    time.Time
}

// ResponseCache is a bounded, TTL-based in-memory cache of recently sent
// correlation-ID-bearing responses. It is safe for concurrent use.
//
// Only non-idempotent, slow commands are cached (CMD_START_RUN, CMD_RERUN_TASK,
// CMD_RUN_EVALUATORS, CMD_CANCEL_RUN). The intent is to let the frontend resolve
// a pending promise after a transient reconnect rather than silently dropping the
// response.
type ResponseCache struct {
	mu      sync.Mutex
	entries map[string]CachedResponse
	ttl     time.Duration
}

// NewResponseCache creates a cache whose entries live for at most ttl.
func NewResponseCache(ttl time.Duration) *ResponseCache {
	return &ResponseCache{
		entries: make(map[string]CachedResponse),
		ttl:     ttl,
	}
}

// Store saves a response. correlationID must be non-empty; silently ignored otherwise.
func (rc *ResponseCache) Store(correlationID, msgType string, payloadJSON []byte) {
	if correlationID == "" {
		return
	}
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.evictExpiredLocked()
	rc.entries[correlationID] = CachedResponse{
		MsgType:     msgType,
		PayloadJSON: payloadJSON,
		StoredAt:    time.Now(),
	}
}

// Get retrieves a stored response. Returns false when absent or expired.
func (rc *ResponseCache) Get(correlationID string) (CachedResponse, bool) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	entry, ok := rc.entries[correlationID]
	if !ok {
		return CachedResponse{}, false
	}
	if time.Since(entry.StoredAt) > rc.ttl {
		delete(rc.entries, correlationID)
		return CachedResponse{}, false
	}
	return entry, true
}

// evictExpiredLocked removes stale entries. Must be called with mu held.
func (rc *ResponseCache) evictExpiredLocked() {
	now := time.Now()
	for id, entry := range rc.entries {
		if now.Sub(entry.StoredAt) > rc.ttl {
			delete(rc.entries, id)
		}
	}
}
