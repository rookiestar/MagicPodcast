package main

import (
	"fmt"
	"path/filepath"
	"testing"

	"magicpodcast/internal/config"
	"magicpodcast/internal/models"
	"magicpodcast/internal/processing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
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

func TestValidatePersistedWorkflowLLMConfigsReadsStoredBudget(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:api_workflow_config_"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Workflow{}))

	workflow := &models.Workflow{
		Name:      "教育每周四精选",
		Schedule:  "0 0 * * *",
		ScopeType: models.ScopeTypeAllSubscribed,
		RulesConfig: models.RulesConfig{
			LLMEnabled:   true,
			LLMMaxTokens: 4000,
		},
	}
	require.NoError(t, db.Create(workflow).Error)

	err = validatePersistedWorkflowLLMConfigs(db, config.LLMConfig{Enabled: true, MaxTokensPerRequest: 2000})
	require.EqualError(t, err, fmt.Sprintf("workflow %d (\"教育每周四精选\"): llm_max_tokens必须在100-2000之间", workflow.ID))
}
