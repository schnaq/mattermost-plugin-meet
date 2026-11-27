package google

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHolderSwap(t *testing.T) {
	var h Holder
	_, err := h.CreateSpace(context.Background(), "o")
	require.ErrorIs(t, err, ErrNotConfigured)

	h.Set(&Fake{Space: Space{Name: "spaces/x"}})
	s, err := h.CreateSpace(context.Background(), "o")
	require.NoError(t, err)
	require.Equal(t, "spaces/x", s.Name)

	h.Set(nil)
	require.ErrorIs(t, h.EndActiveConference(context.Background(), "o", "s"), ErrNotConfigured)
}
