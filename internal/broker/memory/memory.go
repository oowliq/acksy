// Package memory is an in-process Broker for single-node deployments and tests.
package memory

import (
	"context"
	"sync"

	"github.com/oowliq/acksy/internal/broker"
)

// bufferSize bounds per-subscriber queues; slow subscribers drop messages
// rather than blocking publishers (clients resync via ?since=<seq>).
const bufferSize = 64

// Broker is an in-memory broker.Broker.
type Broker struct {
	mu     sync.RWMutex
	subs   map[string]map[*sub]struct{}
	closed bool
}

var _ broker.Broker = (*Broker)(nil)

// New returns an empty in-memory broker.
func New() *Broker {
	return &Broker{subs: make(map[string]map[*sub]struct{})}
}

// Publish implements broker.Broker.
func (b *Broker) Publish(_ context.Context, channel string, payload []byte) error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.closed {
		return broker.ErrClosed
	}
	for s := range b.subs[channel] {
		msg := append([]byte(nil), payload...)
		select {
		case s.ch <- msg:
		default: // subscriber too slow: drop
		}
	}
	return nil
}

// Subscribe implements broker.Broker.
func (b *Broker) Subscribe(ctx context.Context, channel string) (broker.Subscription, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, broker.ErrClosed
	}
	s := &sub{b: b, channel: channel, ch: make(chan []byte, bufferSize), done: make(chan struct{})}
	if b.subs[channel] == nil {
		b.subs[channel] = make(map[*sub]struct{})
	}
	b.subs[channel][s] = struct{}{}

	go func() {
		select {
		case <-ctx.Done():
			_ = s.Close()
		case <-s.done:
		}
	}()
	return s, nil
}

// Close implements broker.Broker.
func (b *Broker) Close() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	var all []*sub
	for _, set := range b.subs {
		for s := range set {
			all = append(all, s)
		}
	}
	b.mu.Unlock()

	for _, s := range all {
		_ = s.Close()
	}
	return nil
}

func (b *Broker) remove(s *sub) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if set := b.subs[s.channel]; set != nil {
		delete(set, s)
		if len(set) == 0 {
			delete(b.subs, s.channel)
		}
	}
}

type sub struct {
	b       *Broker
	channel string
	ch      chan []byte
	done    chan struct{}
	once    sync.Once
}

func (s *sub) C() <-chan []byte { return s.ch }

func (s *sub) Close() error {
	s.once.Do(func() {
		// Remove first so no publisher sends on a closed channel: Publish holds
		// the read lock while sending, remove takes the write lock.
		s.b.remove(s)
		close(s.done)
		close(s.ch)
	})
	return nil
}
