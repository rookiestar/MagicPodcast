package main

import (
	"path/filepath"
	"testing"

	"magicpodcast/internal/config"
	"magicpodcast/internal/processing"

	"github.com/stretchr/testify/require"
)

func TestNewProcessingBridgeBindingsWiresConfiguredIMAAdapter(t *testing.T) {
	disabled, err := newProcessingBridgeBindings(config.ProcessingConfig{})
	require.NoError(t, err)
	require.Empty(t, disabled)

	root := filepath.Join(t.TempDir(), "ima")
	bindings, err := newProcessingBridgeBindings(config.ProcessingConfig{
		IMA: config.ProcessingIMAConfig{
			Enabled:     true,
			PackageRoot: root,
			Destination: "manual-import",
		},
	})
	require.NoError(t, err)
	require.Len(t, bindings, 1)
	require.Equal(t, "manual-import", bindings[0].Destination)
	require.Equal(t, "ima", bindings[0].Adapter.Target())
	require.Equal(
		t,
		processing.IMAManualImportAdapterVersion,
		bindings[0].Adapter.AdapterVersion(),
	)
	require.DirExists(t, root)
}
