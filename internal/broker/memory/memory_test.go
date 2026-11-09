package memory_test

import (
	"testing"

	"github.com/oowliq/acksy/internal/broker"
	"github.com/oowliq/acksy/internal/broker/brokertest"
	"github.com/oowliq/acksy/internal/broker/memory"
)

func TestContract(t *testing.T) {
	brokertest.Run(t, func(*testing.T) broker.Broker { return memory.New() })
}
