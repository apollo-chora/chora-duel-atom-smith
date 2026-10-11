package agentconfig_test

import (
	"testing"

	"github.com/apollo-chora/chora-duel-atom-smith/internal/agentconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDuelAtomSmith_TierLadders(t *testing.T) {
	cfg, err := agentconfig.DuelAtomSmith()
	require.NoError(t, err)
	assert.Equal(t, "duel_atom_smith", cfg.Agent)

	// CHEAP tier — smith (MCQ selection/generation; bounded task inside the
	// duel path's 10s ctx). The fallback repeats the primary on purpose: the
	// deployment has one text route (LongCat-2.5-Preview), so there is nothing
	// else to degrade to.
	smith, err := cfg.Sub("smith")
	require.NoError(t, err)
	assert.Equal(t, "cheap", smith.Tier)
	assert.Equal(t, "longcat-2.5-preview", smith.PrimaryModel)
	assert.Equal(t, []string{"longcat-2.5-preview"}, smith.FallbackModels)
	assert.Equal(t, "v1", smith.PromptVersion)
}

func TestSub_MissingSubAgentFailsLoud(t *testing.T) {
	cfg, err := agentconfig.DuelAtomSmith()
	require.NoError(t, err)
	_, err = cfg.Sub("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no sub-agent")
}
