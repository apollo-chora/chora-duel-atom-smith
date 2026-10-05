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
	// duel path's 10s ctx).
	smith, err := cfg.Sub("smith")
	require.NoError(t, err)
	assert.Equal(t, "cheap", smith.Tier)
	assert.Equal(t, "gemini-3.5-flash", smith.PrimaryModel)
	assert.Equal(t, []string{"gemini-2.5-flash"}, smith.FallbackModels)
	assert.Equal(t, "v1", smith.PromptVersion)
}

func TestSub_MissingSubAgentFailsLoud(t *testing.T) {
	cfg, err := agentconfig.DuelAtomSmith()
	require.NoError(t, err)
	_, err = cfg.Sub("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no sub-agent")
}
