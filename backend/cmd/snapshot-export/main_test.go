package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunRequiresExplicitProductionReadConfirmation(t *testing.T) {
	err := run([]string{"--source", "production.db", "--output", "staging"})
	require.ErrorContains(t, err, "explicit confirmation")
}
