package gotify_test

import (
	"testing"

	"github.com/oowliq/acksy/internal/adapter/adaptertest"
	"github.com/oowliq/acksy/internal/adapter/gotify"
)

// Requests follow https://gotify.net/docs/pushmsg and /docs/msgextras
// (see testdata/*.http).
func TestGolden(t *testing.T) {
	adaptertest.Run(t, gotify.New())
}
