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
}

func NewHub() *Hub {
	return &Hub{subscribers: make(map[chan Event]struct{})}
}

func (h *Hub) Publish(event Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for subscriber := range h.subscribers {
		select {
		case subscriber <- event:
		default:
			select {
			case <-subscriber:
			default:
			}
			select {
			case subscriber <- event:
			default:
			}
		}
	}
}

func (h *Hub) Subscribe() (<-chan Event, func()) {
	subscriber := make(chan Event, subscriberCapacity)
	h.mu.Lock()
	h.subscribers[subscriber] = struct{}{}
	h.mu.Unlock()
	var once sync.Once
	return subscriber, func() {
		once.Do(func() {
			h.mu.Lock()
			delete(h.subscribers, subscriber)
			close(subscriber)
			h.mu.Unlock()
		})
	}
}
