package api

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// AudienceType identifies how a broadcast event was scoped when it was
// dispatched. Resume callers replay events by filtering the buffer against
// their own (userID, workspaceID, accessible QS IDs).
type AudienceType string

const (
	AudienceWorkspace AudienceType = "workspace"
	AudienceQS        AudienceType = "qs"
	AudienceAgent     AudienceType = "agent"
	AudienceUser      AudienceType = "user"
	AudienceAll       AudienceType = "all"
)

// BufferedEvent is the wire-ready snapshot the buffer stores for each event
// emitted by SendEvent / SendEventToQS / SendEventForRun. Msg is already a
// full, marshaled envelope including `event_id`, so resume handlers can
// re-deliver it verbatim.
type BufferedEvent struct {
	EventID      string
	Counter      uint64
	Timestamp    time.Time
	AudienceType AudienceType
	AudienceID   uuid.UUID
	Type         string
	Msg          []byte
}

// EventBuffer is a bounded, in-memory ring buffer of broadcast events used to
// replay messages missed during brief WebSocket disconnects. It is safe for
// concurrent writes and reads.
//
// The buffer is per-process; multi-instance deployments would need a shared
// store (Redis / NATS) instead. With a single Cloud Run instance this
// trades a small amount of RAM for a much smoother reconnect UX.
type EventBuffer struct {
	mu          sync.RWMutex
	events      []BufferedEvent
	maxSize     int
	ttl         time.Duration
	counter     atomic.Uint64
	serverNonce string
}

// NewEventBuffer returns a buffer that holds up to maxSize events, expiring
// entries older than ttl on read. A random nonce is generated so clients can
// detect server restarts and fall back to a full sync.
func NewEventBuffer(maxSize int, ttl time.Duration) *EventBuffer {
	if maxSize <= 0 {
		maxSize = 2000
	}
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	return &EventBuffer{
		maxSize:     maxSize,
		ttl:         ttl,
		serverNonce: uuid.NewString()[:8],
	}
}

// ServerNonce returns the random nonce stamped into every event ID. Useful
// for tests and for servers that want to expose it (e.g. on first connect).
func (b *EventBuffer) ServerNonce() string { return b.serverNonce }

// NextEventID allocates the next monotonic ID. The returned string encodes
// the server nonce so stale IDs from a previous process can be detected.
func (b *EventBuffer) NextEventID() (string, uint64) {
	n := b.counter.Add(1)
	return fmt.Sprintf("%s:%d", b.serverNonce, n), n
}

// Add stores a new event. Callers are expected to have already filled
// EventID / Counter / Timestamp (typically via NextEventID + time.Now).
func (b *EventBuffer) Add(entry BufferedEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, entry)
	if len(b.events) > b.maxSize {
		// Drop oldest so the slice stays bounded.
		drop := len(b.events) - b.maxSize
		copy(b.events, b.events[drop:])
		b.events = b.events[:b.maxSize]
	}
}

// parseEventID splits "{nonce}:{counter}" into its parts. An empty nonce or a
// non-numeric counter returns ok=false.
func parseEventID(id string) (nonce string, counter uint64, ok bool) {
	if id == "" {
		return "", 0, false
	}
	idx := strings.LastIndex(id, ":")
	if idx <= 0 || idx == len(id)-1 {
		return "", 0, false
	}
	nonce = id[:idx]
	n, err := strconv.ParseUint(id[idx+1:], 10, 64)
	if err != nil {
		return "", 0, false
	}
	return nonce, n, true
}

// Since returns every event strictly newer than sinceEventID that passes
// filter. If the buffer cannot resume from sinceEventID (server restart,
// unknown nonce, event rotated out of the ring) needsFullSync is true and
// events is empty — the caller should trigger a full REQ_SYNC_STATE.
//
// filter receives each candidate event and should return true if the event
// belongs to the requesting client's audience. filter=nil means "accept
// everything".
func (b *EventBuffer) Since(sinceEventID string, filter func(BufferedEvent) bool) (events []BufferedEvent, lastEventID string, needsFullSync bool) {
	nonce, counter, ok := parseEventID(sinceEventID)
	if !ok {
		return nil, "", true
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	if nonce != b.serverNonce {
		return nil, "", true
	}

	// If the buffer is empty we can't tell whether we missed anything — safest
	// option is to signal full sync when the counter predates the current
	// session's start (counter > 0 but no events yet means nothing to replay).
	if len(b.events) == 0 {
		if counter >= b.counter.Load() {
			// Caller is up to date; nothing missed, no resync needed.
			return nil, sinceEventID, false
		}
		return nil, "", true
	}

	first := b.events[0]
	// Oldest event we still have is first.Counter. If the client is behind
	// that point the rest has rotated out — resync.
	if first.Counter > counter+1 {
		return nil, "", true
	}

	now := time.Now()
	out := make([]BufferedEvent, 0, 16)
	var last string
	for _, e := range b.events {
		if e.Counter <= counter {
			continue
		}
		if now.Sub(e.Timestamp) > b.ttl {
			// An event the client should have received is now past TTL and
			// we can't guarantee ordering/completeness → force resync.
			return nil, "", true
		}
		if filter != nil && !filter(e) {
			// Advance the cursor past events the client doesn't care about
			// so a follow-up call doesn't re-inspect them.
			last = e.EventID
			continue
		}
		out = append(out, e)
		last = e.EventID
	}
	if last == "" {
		last = sinceEventID
	}
	return out, last, false
}

// Len returns the number of events currently retained (for tests/metrics).
func (b *EventBuffer) Len() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.events)
}
