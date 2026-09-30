//go:build smoke

package aim

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLiveModelsDevSchema hits live models.dev and verifies basic invariants.
func TestLiveModelsDevSchema(t *testing.T) {
	src := &ModelsDevSource{}
	providers, err := src.Fetch(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, providers, "expected >0 providers from live models.dev")

	for key, p := range providers {
		require.NotNil(t, p, "provider %q must not be nil", key)
		assert.Equal(t, key, p.ID,
			"provider map key %q must equal provider.id %q", key, p.ID)
	}
}
