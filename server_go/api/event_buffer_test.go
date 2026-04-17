package api

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
)

func addEvent(t *testing.T, b *EventBuffer, audType AudienceType, audID uuid.UUID, evtType string) string {
	t.Helper()
	id, counter := b.NextEventID()
	b.Add(BufferedEvent{
		EventID:      id,
		Counter:      counter,
		Timestamp:    time.Now(),
		AudienceType: audType,
		AudienceID:   audID,
		Type:         evtType,
		Msg:          []byte(fmt.Sprintf(`{"type":%q,"event_id":%q}`, evtType, id)),
	})
	return id
}

func TestEventBuffer_NextEventIDMonotonic(t *testing.T) {
	b := NewEventBuffer(10, time.Minute)

	prev := uint64(0)
	for i := 0; i < 5; i++ {
		id, n := b.NextEventID()
		if n != prev+1 {
			t.Fatalf("expected counter %d, got %d", prev+1, n)
		}
		expected := fmt.Sprintf("%s:%d", b.ServerNonce(), n)
		if id != expected {
			t.Fatalf("expected id %q, got %q", expected, id)
		}
		prev = n
	}
}

func TestEventBuffer_SinceReturnsNewerEventsOnly(t *testing.T) {
	b := NewEventBuffer(10, time.Minute)
	ws := uuid.New()

	_ = addEvent(t, b, AudienceWorkspace, ws, "EVT_TASK_STARTED")
	second := addEvent(t, b, AudienceWorkspace, ws, "EVT_TASK_PROGRESS")
	third := addEvent(t, b, AudienceWorkspace, ws, "EVT_TASK_COMPLETED")

	events, last, needsFullSync := b.Since(second, nil)
	if needsFullSync {
		t.Fatalf("did not expect full sync")
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event after cursor, got %d", len(events))
	}
	if events[0].EventID != third {
		t.Fatalf("expected %q, got %q", third, events[0].EventID)
	}
	if last != third {
		t.Fatalf("expected last=%q, got %q", third, last)
	}
}

func TestEventBuffer_SinceAppliesFilter(t *testing.T) {
	b := NewEventBuffer(10, time.Minute)
	wsA := uuid.New()
	wsB := uuid.New()

	// Anchor cursor at the start of the session (before any event).
	first := addEvent(t, b, AudienceWorkspace, wsA, "EVT_A1")
	_ = addEvent(t, b, AudienceWorkspace, wsB, "EVT_B1")
	thirdForA := addEvent(t, b, AudienceWorkspace, wsA, "EVT_A2")

	events, last, needsFullSync := b.Since(first, func(e BufferedEvent) bool {
		return e.AudienceType == AudienceWorkspace && e.AudienceID == wsA
	})
	if needsFullSync {
		t.Fatalf("did not expect full sync")
	}
	if len(events) != 1 || events[0].EventID != thirdForA {
		t.Fatalf("expected only thirdForA, got %+v", events)
	}
	// The filtered-out event should still advance the cursor so the client
	// doesn't ask for it repeatedly.
	if last != thirdForA {
		t.Fatalf("expected last=%q, got %q", thirdForA, last)
	}
}

func TestEventBuffer_RingDropsOldest(t *testing.T) {
	b := NewEventBuffer(3, time.Minute)
	ws := uuid.New()

	// Anchor cursor at counter=1; add four more so the ring rotates far
	// enough that the oldest entry is counter=3 — i.e. counter=2 was lost.
	first := addEvent(t, b, AudienceWorkspace, ws, "EVT_1")
	_ = addEvent(t, b, AudienceWorkspace, ws, "EVT_2")
	_ = addEvent(t, b, AudienceWorkspace, ws, "EVT_3")
	_ = addEvent(t, b, AudienceWorkspace, ws, "EVT_4")
	_ = addEvent(t, b, AudienceWorkspace, ws, "EVT_5")

	if b.Len() != 3 {
		t.Fatalf("expected len=3, got %d", b.Len())
	}

	_, _, needsFullSync := b.Since(first, nil)
	if !needsFullSync {
		t.Fatalf("expected needsFullSync=true: EVT_2 rotated out before delivery")
	}
}

func TestEventBuffer_SinceMismatchNonceTriggersResync(t *testing.T) {
	b := NewEventBuffer(5, time.Minute)
	addEvent(t, b, AudienceWorkspace, uuid.New(), "EVT_X")

	_, _, needsFullSync := b.Since("othernonce:1", nil)
	if !needsFullSync {
		t.Fatalf("expected needsFullSync=true on nonce mismatch")
	}
}

func TestEventBuffer_SinceMalformedCursorTriggersResync(t *testing.T) {
	b := NewEventBuffer(5, time.Minute)
	for _, cursor := range []string{"", "garbage", "abc:", ":42", "abc:notanumber"} {
		_, _, needsFullSync := b.Since(cursor, nil)
		if !needsFullSync {
			t.Fatalf("expected needsFullSync=true for cursor %q", cursor)
		}
	}
}

func TestEventBuffer_SinceEmptyBufferWithMatchingCounter(t *testing.T) {
	b := NewEventBuffer(5, time.Minute)
	// Advance the counter without ever adding events (simulates a broadcast
	// that was dispatched but never bufferized — shouldn't happen in prod,
	// but the behaviour must stay deterministic).
	id, _ := b.NextEventID()
	events, _, needsFullSync := b.Since(id, nil)
	if needsFullSync {
		t.Fatalf("expected no resync when counter matches current head")
	}
	if len(events) != 0 {
		t.Fatalf("expected 0 events, got %d", len(events))
	}
}

func TestEventBuffer_TTLExpiryAboveCursorTriggersResync(t *testing.T) {
	b := NewEventBuffer(10, 10*time.Millisecond)
	ws := uuid.New()

	// Cursor at counter=1; counter=2 expires before the client reconnects.
	// The buffer cannot safely serve only counter=3 (that would skip 2).
	first := addEvent(t, b, AudienceWorkspace, ws, "EVT_1")
	_ = addEvent(t, b, AudienceWorkspace, ws, "EVT_2")
	time.Sleep(25 * time.Millisecond)
	_ = addEvent(t, b, AudienceWorkspace, ws, "EVT_3")

	_, _, needsFullSync := b.Since(first, nil)
	if !needsFullSync {
		t.Fatalf("expected needsFullSync=true: EVT_2 expired before delivery")
	}
}

func TestEventBuffer_TTLExpiryBelowCursorIgnored(t *testing.T) {
	b := NewEventBuffer(10, 10*time.Millisecond)
	ws := uuid.New()

	// Cursor advances past counter=1 (client already saw EVT_1). Even if
	// EVT_1 later expires we don't need a resync — we only care about
	// events strictly newer than the cursor.
	_ = addEvent(t, b, AudienceWorkspace, ws, "EVT_1")
	second := addEvent(t, b, AudienceWorkspace, ws, "EVT_2")
	time.Sleep(25 * time.Millisecond)
	third := addEvent(t, b, AudienceWorkspace, ws, "EVT_3")

	events, last, needsFullSync := b.Since(second, nil)
	if needsFullSync {
		t.Fatalf("did not expect resync: EVT_1 expired but is below cursor")
	}
	if len(events) != 1 || events[0].EventID != third {
		t.Fatalf("expected only EVT_3, got %+v", events)
	}
	if last != third {
		t.Fatalf("expected last=%q, got %q", third, last)
	}
}
