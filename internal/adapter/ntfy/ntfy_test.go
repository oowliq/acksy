package ntfy_test

import (
	"testing"

	"github.com/oowliq/acksy/internal/adapter/adaptertest"
	"github.com/oowliq/acksy/internal/adapter/ntfy"
)

// Requests come from https://docs.ntfy.sh/publish/ (see testdata/*.http).
func TestGolden(t *testing.T) {
	a := ntfy.New()
	a.NewID = func() string { return "GeneratedID1" }
	adaptertest.Run(t, a)
}
