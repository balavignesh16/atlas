package buffer

import (
	"sync"
	"sync/atomic"

	"github.com/atlas/intelligence-engine/internal/event"
)

// EventBuffer is a thread-safe, bounded circular buffer for ATLASEvents.
type EventBuffer struct {
	mu       sync.RWMutex
	events   []event.ATLASEvent
	capacity int
	dropped  atomic.Uint64
}

// NewEventBuffer creates a new bounded event buffer.
func NewEventBuffer(capacity int) *EventBuffer {
	if capacity <= 0 {
		capacity = 10000 // default
	}
	return &EventBuffer{
		events:   make([]event.ATLASEvent, 0, capacity),
		capacity: capacity,
	}
}

// Add appends an event to the buffer. If the buffer is full, it drops the oldest event.
func (b *EventBuffer) Add(e event.ATLASEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.events) >= b.capacity {
		// Drop oldest by shifting
		copy(b.events, b.events[1:])
		b.events[len(b.events)-1] = e
		b.dropped.Add(1)
	} else {
		b.events = append(b.events, e)
	}
}

// GetAll returns a copy of all events currently in the buffer.
func (b *EventBuffer) GetAll() []event.ATLASEvent {
	b.mu.RLock()
	defer b.mu.RUnlock()

	res := make([]event.ATLASEvent, len(b.events))
	copy(res, b.events)
	return res
}

// GetDroppedCount returns the number of events dropped due to overflow.
func (b *EventBuffer) GetDroppedCount() uint64 {
	return b.dropped.Load()
}
