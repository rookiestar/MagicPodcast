package main

import (
	"fmt"

	"magicpodcast/internal/config"
	"magicpodcast/internal/models"

	"gorm.io/gorm"
)

// validatePersistedWorkflowLLMConfigs checks the database-backed workflow
// overrides before the scheduler and HTTP handlers are initialized. Existing
// rows may predate the global budget validation applied by the API, so they
// must be checked explicitly at startup.
func validatePersistedWorkflowLLMConfigs(db *gorm.DB, llmConfig config.LLMConfig) error {
	if db == nil {
		return fmt.Errorf("database is nil")
	}

	var workflows []models.Workflow
	if err := db.Select("id", "name", "rules_config").Find(&workflows).Error; err != nil {
		return fmt.Errorf("failed to load workflow LLM configurations: %w", err)
	}

	configs := make([]config.WorkflowLLMConfig, len(workflows))
	for i, workflow := range workflows {
		configs[i] = config.WorkflowLLMConfig{
			WorkflowID:   workflow.ID,
			WorkflowName: workflow.Name,
			LLMEnabled:   workflow.RulesConfig.LLMEnabled,
			LLMMaxTokens: workflow.RulesConfig.LLMMaxTokens,
		}
	}
	return config.ValidateWorkflowLLMConfigs(llmConfig.MaxTokensPerRequest, configs)
}
