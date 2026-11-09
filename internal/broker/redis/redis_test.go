package redis_test

import (
	"context"
	"os"
	"testing"

	"github.com/oowliq/acksy/internal/broker"
	"github.com/oowliq/acksy/internal/broker/brokertest"
	"github.com/oowliq/acksy/internal/broker/redis"
)

// Runs only when a Redis URL is provided (make test-deps / CI service).
func TestContract(t *testing.T) {
	url := os.Getenv("ACKSY_TEST_REDIS_URL")
	if url == "" {
		t.Skip("ACKSY_TEST_REDIS_URL not set")
	}
	brokertest.Run(t, func(t *testing.T) broker.Broker {
		b, err := redis.New(context.Background(), url)
		if err != nil {
			t.Fatal(err)
		}
		return b
	})
}
