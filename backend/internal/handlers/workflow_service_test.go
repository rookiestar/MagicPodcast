package handlers_test

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	apperrors "magicpodcast/internal/errors"
	"magicpodcast/internal/models"
	"magicpodcast/internal/repository"
	"magicpodcast/internal/services"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestWorkflowServiceIntegration 测试WorkflowService集成
func TestWorkflowServiceIntegration(t *testing.T) {
	// 设置测试数据库
	db, err := gorm.Open(sqlite.Open("file:service_test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	// 自动迁移
	err = db.AutoMigrate(&models.Workflow{}, &models.Job{}, &models.JobExecution{})
	if err != nil {
		t.Fatalf("Failed to migrate database: %v", err)
	}

	// 创建Repository容器
	repos := repository.NewRepositoriesWithDB(db)
	workflowService := services.NewWorkflowService(repos)

	t.Run("CreateWorkflow", func(t *testing.T) {
		req := &services.CreateWorkflowRequest{
			Name:        "Integration Test Workflow",
			Description: "Testing Service Layer",
			Schedule:    "0 2 * * *", // 每天凌晨2点
			ScopeType:   models.ScopeTypeSpecificPodcasts,
			ScopeConfig: models.ScopeConfig{
				PodcastIDs: []int{1, 2, 3},
			},
			RulesConfig: models.RulesConfig{
				TimeRange:     30,
				TimeRangeMode: "days",
			},
			IsEnabled: true,
		}

		response, err := workflowService.CreateWorkflow(req)
		if err != nil {
			t.Fatalf("Failed to create workflow: %v", err)
		}

		if response.Name != req.Name {
			t.Errorf("Expected name %s, got %s", req.Name, response.Name)
		}

		if response.ID == 0 {
			t.Error("Expected non-zero ID")
		}

		if !response.IsEnabled {
			t.Error("Expected IsEnabled to be true")
		}
	})

	t.Run("GetWorkflow", func(t *testing.T) {
		// 先创建一个工作流
		createReq := &services.CreateWorkflowRequest{
			Name:      "Get Test Workflow",
			Schedule:  "0 3 * * *",
			ScopeType: models.ScopeTypeAllSubscribed,
			ScopeConfig: models.ScopeConfig{
				PodcastIDs: []int{},
			},
			RulesConfig: models.RulesConfig{},
			IsEnabled:   false,
		}

		created, err := workflowService.CreateWorkflow(createReq)
		if err != nil {
			t.Fatalf("Failed to create workflow: %v", err)
		}

		// 获取工作流
		fetched, err := workflowService.GetWorkflow(created.ID)
		if err != nil {
			t.Fatalf("Failed to get workflow: %v", err)
		}

		if fetched.Name != created.Name {
			t.Errorf("Expected name %s, got %s", created.Name, fetched.Name)
		}

		if fetched.ID != created.ID {
			t.Errorf("Expected ID %d, got %d", created.ID, fetched.ID)
		}
	})

	t.Run("UpdateWorkflow", func(t *testing.T) {
		// 先创建一个工作流
		createReq := &services.CreateWorkflowRequest{
			Name:        "Update Test Workflow",
			Description: "Original Description",
			Schedule:    "0 4 * * *",
			ScopeType:   models.ScopeTypeSpecificPodcasts,
			ScopeConfig: models.ScopeConfig{
				PodcastIDs: []int{1},
			},
			RulesConfig: models.RulesConfig{},
			IsEnabled:   false,
		}

		created, err := workflowService.CreateWorkflow(createReq)
		if err != nil {
			t.Fatalf("Failed to create workflow: %v", err)
		}

		// 更新工作流
		newName := "Updated Workflow Name"
		newDesc := "Updated Description"
		updateReq := &services.UpdateWorkflowRequest{
			Name:        &newName,
			Description: &newDesc,
			IsEnabled:   func() *bool { b := true; return &b }(),
		}

		updated, err := workflowService.UpdateWorkflow(created.ID, updateReq)
		if err != nil {
			t.Fatalf("Failed to update workflow: %v", err)
		}

		if updated.Name != newName {
			t.Errorf("Expected name %s, got %s", newName, updated.Name)
		}

		if updated.Description != newDesc {
			t.Errorf("Expected description %s, got %s", newDesc, updated.Description)
		}

		if !updated.IsEnabled {
			t.Error("Expected IsEnabled to be true after update")
		}
	})

	t.Run("ToggleWorkflow", func(t *testing.T) {
		// 先创建一个禁用的工作流
		createReq := &services.CreateWorkflowRequest{
			Name:        "Toggle Test Workflow",
			Schedule:    "0 5 * * *",
			ScopeType:   models.ScopeTypeAllSubscribed,
			ScopeConfig: models.ScopeConfig{},
			RulesConfig: models.RulesConfig{},
			IsEnabled:   false,
		}

		created, err := workflowService.CreateWorkflow(createReq)
		if err != nil {
			t.Fatalf("Failed to create workflow: %v", err)
		}

		if created.IsEnabled {
			t.Error("Expected workflow to be disabled initially")
		}

		// 切换状态
		toggled, err := workflowService.ToggleWorkflow(created.ID)
		if err != nil {
			t.Fatalf("Failed to toggle workflow: %v", err)
		}

		if !toggled.IsEnabled {
			t.Error("Expected workflow to be enabled after toggle")
		}
	})

	t.Run("DeleteWorkflow", func(t *testing.T) {
		// 先创建一个工作流
		createReq := &services.CreateWorkflowRequest{
			Name:        "Delete Test Workflow",
			Schedule:    "0 6 * * *",
			ScopeType:   models.ScopeTypeAllSubscribed,
			ScopeConfig: models.ScopeConfig{},
			RulesConfig: models.RulesConfig{},
			IsEnabled:   true,
		}

		created, err := workflowService.CreateWorkflow(createReq)
		if err != nil {
			t.Fatalf("Failed to create workflow: %v", err)
		}

		// 删除工作流
		err = workflowService.DeleteWorkflow(created.ID)
		if err != nil {
			t.Fatalf("Failed to delete workflow: %v", err)
		}

		// 验证已删除
		_, err = workflowService.GetWorkflow(created.ID)
		if err == nil {
			t.Error("Expected error when fetching deleted workflow")
		}

		// 验证错误类型
		var appErr apperrors.AppError
		if err != nil && errors.As(err, &appErr) {
			if appErr.StatusCode() != http.StatusNotFound {
				t.Errorf("Expected 404 error, got %d", appErr.StatusCode())
			}
		}
	})

	t.Run("ListWorkflows", func(t *testing.T) {
		// 清理现有数据
		db.Where("1 = 1").Delete(&models.Workflow{})

		// 创建多个工作流
		for i := 1; i <= 5; i++ {
			req := &services.CreateWorkflowRequest{
				Name:        fmt.Sprintf("Workflow %d", i),
				Schedule:    fmt.Sprintf("0 %d * * *", i),
				ScopeType:   models.ScopeTypeAllSubscribed,
				ScopeConfig: models.ScopeConfig{},
				RulesConfig: models.RulesConfig{},
				IsEnabled:   i%2 == 0, // 偶数启用
			}

			_, err := workflowService.CreateWorkflow(req)
			if err != nil {
				t.Fatalf("Failed to create workflow %d: %v", i, err)
			}
		}

		// 测试获取所有工作流
		result, err := workflowService.ListWorkflows(1, 10, false)
		if err != nil {
			t.Fatalf("Failed to list workflows: %v", err)
		}

		if result.Total != 5 {
			t.Errorf("Expected total 5, got %d", result.Total)
		}

		if len(result.Workflows) != 5 {
			t.Errorf("Expected 5 workflows, got %d", len(result.Workflows))
		}

		// 测试仅获取已启用的工作流
		resultEnabled, err := workflowService.ListWorkflows(1, 10, true)
		if err != nil {
			t.Fatalf("Failed to list enabled workflows: %v", err)
		}

		if resultEnabled.Total != 2 {
			t.Errorf("Expected 2 enabled workflows, got %d", resultEnabled.Total)
		}
	})

	t.Run("ValidationError - Invalid Cron", func(t *testing.T) {
		req := &services.CreateWorkflowRequest{
			Name:        "Invalid Cron Workflow",
			Schedule:    "invalid-cron",
			ScopeType:   models.ScopeTypeAllSubscribed,
			ScopeConfig: models.ScopeConfig{},
			RulesConfig: models.RulesConfig{},
			IsEnabled:   true,
		}

		_, err := workflowService.CreateWorkflow(req)
		if err == nil {
			t.Error("Expected error for invalid cron expression")
		}

		// 验证错误类型
		var appErr apperrors.AppError
		if errors.As(err, &appErr) {
			if appErr.Code() != "INVALID_CRON_EXPRESSION" {
				t.Errorf("Expected INVALID_CRON_EXPRESSION error, got %s", appErr.Code())
			}
		}
	})
}
