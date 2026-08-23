package models

// AllModels 返回所有需要迁移的模型
// 注意：按依赖关系排序，避免外键约束错误
var AllModels = []interface{}{
	// 基础模型（无依赖）
	Tag{},
	Workflow{},
	SyncConfig{},

	// 依赖基础模型的
	Podcast{},
	PodcastAlternativeFeed{},
	Episode{},
	EpisodeCompletion{},
	EpisodeTriageDecision{},
	ConsumptionQueueOrder{},
	EpisodeProcessingRun{},
	ProcessingCheckpoint{},
	EpisodeArtifactSet{},
	KnowledgeDelivery{},
	Job{},
	JobExecution{},
	JobFeedAttempt{},
	SchedulerRun{},
	Report{},
}
