package core

import "sync"

// ChanEventBus is a small in-memory fan-out event bus for tests and local runs.
type ChanEventBus struct {
	mu   sync.RWMutex
	subs []chan<- Event
}

// NewChanEventBus returns an empty bus.
func NewChanEventBus() *ChanEventBus {
	return &ChanEventBus{}
}

// Publish delivers e to every subscriber; slow consumers may drop when buffer is full.
func (b *ChanEventBus) Publish(e Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subs {
		select {
		case ch <- e:
		default:
		}
	}
}

// Subscribe registers a new buffered subscriber channel.
func (b *ChanEventBus) Subscribe(buffer int) <-chan Event {
	ch := make(chan Event, buffer)
	b.mu.Lock()
	b.subs = append(b.subs, ch)
	b.mu.Unlock()
	return ch
}
