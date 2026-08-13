package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunRejectsProductionProfile(t *testing.T) {
	err := run([]string{"use", "production"})
	require.ErrorContains(t, err, "cannot be selected")
}

func TestRunRejectsArbitraryProfile(t *testing.T) {
	err := run([]string{"use", "/tmp/production.db"})
	require.ErrorContains(t, err, "unsupported profile")
}
