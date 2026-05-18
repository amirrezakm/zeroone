// Package events is a small in-process pub/sub used for SSE.
// Subscribers receive a buffered channel; slow subscribers are dropped
// rather than blocking publishers.
package events

import (
	"sync"
	"time"
)

type Event struct {
	Time int64          `json:"t"`
	Kind string         `json:"kind"`
	Data map[string]any `json:"data,omitempty"`
}

type Broker struct {
	mu     sync.Mutex
	subs   map[chan Event]struct{}
	bufLen int
}

func NewBroker() *Broker {
	return &Broker{subs: map[chan Event]struct{}{}, bufLen: 32}
}

func (b *Broker) Subscribe() chan Event {
	ch := make(chan Event, b.bufLen)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

func (b *Broker) Unsubscribe(ch chan Event) {
	b.mu.Lock()
	if _, ok := b.subs[ch]; ok {
		delete(b.subs, ch)
		close(ch)
	}
	b.mu.Unlock()
}

// Publish a fire-and-forget event. Slow subscribers drop the event
// rather than blocking the publisher.
func (b *Broker) Publish(kind string, data map[string]any) {
	ev := Event{Time: time.Now().Unix(), Kind: kind, Data: data}
	b.mu.Lock()
	for ch := range b.subs {
		select {
		case ch <- ev:
		default:
		}
	}
	b.mu.Unlock()
}
