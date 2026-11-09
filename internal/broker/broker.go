// Package broker defines the pub/sub abstraction used to fan out committed
// events to realtime connections across server instances.
//
// The broker is only a delivery channel: events are persisted in the database
// first, so a subscriber that misses a message recovers via ?since=<seq>.
package broker

import (
	"context"
	"errors"
)

// ErrClosed is returned when operating on a closed broker or subscription.
var ErrClosed = errors.New("broker: closed")

// Broker publishes payloads to named channels and lets callers subscribe.
// Delivery is at-most-once and best-effort.
type Broker interface {
	// Publish sends payload to every current subscriber of channel.
	Publish(ctx context.Context, channel string, payload []byte) error
	// Subscribe starts receiving messages for channel. The subscription
	// ends when ctx is cancelled or Close is called.
	Subscribe(ctx context.Context, channel string) (Subscription, error)
	// Close shuts the broker down and ends all subscriptions.
	Close() error
}

// Subscription is a live stream of payloads for one channel.
type Subscription interface {
	// C yields payloads. It is closed when the subscription ends.
	C() <-chan []byte
	// Close ends the subscription. Safe to call multiple times.
	Close() error
}

// UserChannel returns the channel name carrying events for a user.
func UserChannel(userID string) string { return "acksy:user:" + userID }
