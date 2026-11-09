// Package redis implements broker.Broker on top of Redis Pub/Sub, allowing
// several Acksy instances to share realtime fan-out.
package redis

import (
	"context"
	"fmt"
	"sync"

	goredis "github.com/redis/go-redis/v9"

	"github.com/oowliq/acksy/internal/broker"
)

const bufferSize = 64

// Broker is a Redis-backed broker.Broker.
type Broker struct {
	client *goredis.Client

	mu     sync.Mutex
	subs   map[*sub]struct{}
	closed bool
}

var _ broker.Broker = (*Broker)(nil)

// New connects to Redis at url (redis://[:password@]host:port/db) and pings it.
func New(ctx context.Context, url string) (*Broker, error) {
	opts, err := goredis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	client := goredis.NewClient(opts)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	return &Broker{client: client, subs: make(map[*sub]struct{})}, nil
}

// Ping checks Redis is reachable.
func (b *Broker) Ping(ctx context.Context) error { return b.client.Ping(ctx).Err() }

// Publish implements broker.Broker.
func (b *Broker) Publish(ctx context.Context, channel string, payload []byte) error {
	b.mu.Lock()
	closed := b.closed
	b.mu.Unlock()
	if closed {
		return broker.ErrClosed
	}
	return b.client.Publish(ctx, channel, payload).Err()
}

// Subscribe implements broker.Broker. It returns once Redis has confirmed
// the subscription, so messages published afterwards are not missed.
func (b *Broker) Subscribe(ctx context.Context, channel string) (broker.Subscription, error) {
	b.mu.Lock()
	closed := b.closed
	b.mu.Unlock()
	if closed {
		return nil, broker.ErrClosed
	}

	// go-redis sends SUBSCRIBE without waiting for the reply; Receive blocks
	// until the confirmation. It runs outside b.mu so a slow Redis does not
	// stall Publish and Close.
	ps := b.client.Subscribe(context.WithoutCancel(ctx), channel)
	if _, err := ps.Receive(ctx); err != nil {
		_ = ps.Close()
		return nil, fmt.Errorf("redis subscribe %s: %w", channel, err)
	}
	s := &sub{b: b, ps: ps, ch: make(chan []byte, bufferSize), done: make(chan struct{})}

	b.mu.Lock()
	if b.closed { // closed while we waited for Redis
		b.mu.Unlock()
		_ = ps.Close()
		return nil, broker.ErrClosed
	}
	b.subs[s] = struct{}{}
	b.mu.Unlock()

	go s.pump(ctx)
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
	all := make([]*sub, 0, len(b.subs))
	for s := range b.subs {
		all = append(all, s)
	}
	b.mu.Unlock()

	for _, s := range all {
		_ = s.Close()
	}
	return b.client.Close()
}

type sub struct {
	b    *Broker
	ps   *goredis.PubSub
	ch   chan []byte
	done chan struct{}
	once sync.Once
}

func (s *sub) C() <-chan []byte { return s.ch }

func (s *sub) pump(ctx context.Context) {
	defer close(s.ch)
	in := s.ps.Channel()
	for {
		select {
		case <-ctx.Done():
			_ = s.Close()
			return
		case <-s.done:
			return
		case msg, ok := <-in:
			if !ok {
				return
			}
			select {
			case s.ch <- []byte(msg.Payload):
			default: // slow consumer: drop, client resyncs via seq
			}
		}
	}
}

func (s *sub) Close() error {
	var err error
	s.once.Do(func() {
		close(s.done)
		err = s.ps.Close()
		s.b.mu.Lock()
		delete(s.b.subs, s)
		s.b.mu.Unlock()
	})
	return err
}
