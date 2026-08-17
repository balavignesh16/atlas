package buffer

import (
	"strconv"
	"sync"
	"testing"

	"github.com/atlas/intelligence-engine/internal/event"
)

func TestBufferCapacity(t *testing.T) {
	b := NewEventBuffer(10)
	for i := 0; i < 15; i++ {
		b.Add(event.ATLASEvent{EventID: strconv.Itoa(i)})
	}

	events := b.GetAll()
	if len(events) != 10 {
		t.Errorf("Expected exactly 10 events, got %d", len(events))
	}

	// Because of DROP-OLDEST, events 5-14 should be present
	if events[0].EventID != "5" {
		t.Errorf("Expected oldest event to be '5', got %s", events[0].EventID)
	}
	if b.GetDroppedCount() != 5 {
		t.Errorf("Expected 5 dropped events, got %d", b.GetDroppedCount())
	}
}

func TestBufferConcurrency(t *testing.T) {
	b := NewEventBuffer(100)
	var wg sync.WaitGroup

	// Concurrently add 500 events
	for i := 0; i < 500; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			b.Add(event.ATLASEvent{EventID: strconv.Itoa(id)})
		}(i)
	}

	wg.Wait()

	events := b.GetAll()
	if len(events) != 100 {
		t.Errorf("Expected exactly 100 events after concurrent insertions, got %d", len(events))
	}

	if b.GetDroppedCount() != 400 {
		t.Errorf("Expected 400 dropped events, got %d", b.GetDroppedCount())
	}
}
