package live

import "sync"

const subscriberCapacity = 8

type Event struct {
	Type     string   `json:"type"`
	Snapshot Snapshot `json:"snapshot"`
}

type Hub struct {
	mu          sync.Mutex
	subscribers map[chan Event]struct{}
	closed      bool
}

func NewHub() *Hub {
	return &Hub{subscribers: make(map[chan Event]struct{})}
}

func (h *Hub) Publish(event Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	for subscriber := range h.subscribers {
		delivery := cloneEvent(event)
		select {
		case subscriber <- delivery:
		default:
			select {
			case <-subscriber:
			default:
			}
			select {
			case subscriber <- delivery:
			default:
			}
		}
	}
}

func cloneEvent(event Event) Event {
	event.Snapshot = cloneSnapshot(event.Snapshot)
	return event
}

func (h *Hub) Subscribe() (<-chan Event, func()) {
	subscriber := make(chan Event, subscriberCapacity)
	h.mu.Lock()
	if h.closed {
		close(subscriber)
		h.mu.Unlock()
		return subscriber, func() {}
	}
	h.subscribers[subscriber] = struct{}{}
	h.mu.Unlock()
	var once sync.Once
	return subscriber, func() {
		once.Do(func() {
			h.mu.Lock()
			if _, subscribed := h.subscribers[subscriber]; subscribed {
				delete(h.subscribers, subscriber)
				close(subscriber)
			}
			h.mu.Unlock()
		})
	}
}

func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.closed = true
	for subscriber := range h.subscribers {
		delete(h.subscribers, subscriber)
		close(subscriber)
	}
}
