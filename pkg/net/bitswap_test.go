package net

import (
	"testing"

	"github.com/data-preservation-programs/RetrievalBot/pkg/env"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/assert"
)

func TestInitializeCidsToRetrieve(t *testing.T) {
	challenge4 := initializeCidsToRetrieve(4, cid.Cid{})

	assert.Equal(t, 1, len(challenge4))
}

func TestValidateGetMaxChallengesPerLevel(t *testing.T) {
	t.Setenv("MAX_CHALLENGES_PER_LEVEL", "5")
	var challenges = env.GetInt(env.MaxChallengesPerLevel, 5)
	assert.Equal(t, 5, challenges)

	t.Setenv("MAX_CHALLENGES_PER_LEVEL", "50")
	challenges = env.GetInt(env.MaxChallengesPerLevel, 50)
	assert.Equal(t, 50, challenges)
}
