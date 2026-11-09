// Package brokertest holds contract tests every broker.Broker must pass.
package brokertest

import (
	"context"
	"testing"
	"time"

	"github.com/oowliq/acksy/internal/broker"
)

// Factory returns a fresh broker; the suite closes it.
type Factory func(t *testing.T) broker.Broker

const wait = 2 * time.Second

// Run executes the contract suite.
func Run(t *testing.T, newBroker Factory) {
	t.Run("delivers to subscriber", func(t *testing.T) {
		b := newBroker(t)
		defer b.Close()
		ctx := context.Background()
		ch := uniq(t, "deliver")

		sub := mustSub(t, b, ctx, ch)
		defer sub.Close()
		publishUntilReceived(t, b, sub, ch, "hello")
	})

	t.Run("fans out to all subscribers", func(t *testing.T) {
		b := newBroker(t)
		defer b.Close()
		ctx := context.Background()
		ch := uniq(t, "fanout")

		s1, s2 := mustSub(t, b, ctx, ch), mustSub(t, b, ctx, ch)
		defer s1.Close()
		defer s2.Close()

		publishUntilReceived(t, b, s1, ch, "x")
		expect(t, s2, "x")
	})

	t.Run("isolates channels", func(t *testing.T) {
		b := newBroker(t)
		defer b.Close()
		ctx := context.Background()
		a, other := uniq(t, "a"), uniq(t, "b")

		sa, sb := mustSub(t, b, ctx, a), mustSub(t, b, ctx, other)
		defer sa.Close()
		defer sb.Close()

		publishUntilReceived(t, b, sa, a, "only-a")
		select {
		case msg := <-sb.C():
			t.Fatalf("other channel received %q", msg)
		case <-time.After(100 * time.Millisecond):
		}
	})

	t.Run("context cancel ends subscription", func(t *testing.T) {
		b := newBroker(t)
		defer b.Close()
		ctx, cancel := context.WithCancel(context.Background())
		sub := mustSub(t, b, ctx, uniq(t, "cancel"))
		cancel()
		expectClosed(t, sub)
	})

	t.Run("close ends subscriptions and rejects publish", func(t *testing.T) {
		b := newBroker(t)
		sub := mustSub(t, b, context.Background(), uniq(t, "close"))
		if err := b.Close(); err != nil {
			t.Fatal(err)
		}
		expectClosed(t, sub)
		if err := b.Publish(context.Background(), "x", []byte("y")); err == nil {
			t.Fatal("publish after close must fail")
		}
		if err := sub.Close(); err != nil {
			t.Fatalf("double close: %v", err)
		}
	})
}

func uniq(t *testing.T, s string) string {
	return "test:" + t.Name() + ":" + s + ":" + time.Now().Format("150405.000000000")
}

func mustSub(t *testing.T, b broker.Broker, ctx context.Context, ch string) broker.Subscription {
	t.Helper()
	s, err := b.Subscribe(ctx, ch)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// publishUntilReceived publishes repeatedly until s receives msg. Brokers
// must make a subscription active before Subscribe returns; the retry only
// keeps the suite tolerant of slow delivery.
func publishUntilReceived(t *testing.T, b broker.Broker, s broker.Subscription, ch, msg string) {
	t.Helper()
	deadline := time.After(wait)
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		if err := b.Publish(context.Background(), ch, []byte(msg)); err != nil {
			t.Fatal(err)
		}
		select {
		case got := <-s.C():
			if string(got) != msg {
				t.Fatalf("got %q, want %q", got, msg)
			}
			return
		case <-tick.C:
		case <-deadline:
			t.Fatalf("message %q not received", msg)
		}
	}
}

func expect(t *testing.T, s broker.Subscription, msg string) {
	t.Helper()
	select {
	case got := <-s.C():
		if string(got) != msg {
			t.Fatalf("got %q, want %q", got, msg)
		}
	case <-time.After(wait):
		t.Fatalf("message %q not received", msg)
	}
}

func expectClosed(t *testing.T, s broker.Subscription) {
	t.Helper()
	deadline := time.After(wait)
	for {
		select {
		case _, ok := <-s.C():
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("subscription channel not closed")
		}
	}
}
