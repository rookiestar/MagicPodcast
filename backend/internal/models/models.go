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
	// EpisodeCollectionItem 通过可空外键引用 Episode；清单数据独立于个人库。
	EpisodeCollection{},
	EpisodeCollectionItem{},
	// EpisodeExternalRef/EpisodeCollectionAdoption 依赖 Episode；由版本化迁移创建。
	EpisodeExternalRef{},
	EpisodeCollectionAdoption{},
	EpisodeCompletion{},
	EpisodeTriageDecision{},
	ConsumptionQueueOrder{},
	EpisodeProcessingRun{},
	ProcessingCheckpoint{},
	EpisodeArtifactSet{},
	// EpisodeArtifactAudioRecovery is created by its versioned migration after
	// the historical processing foundation is present.
	KnowledgeDelivery{},
	Job{},
	JobExecution{},
	JobFeedAttempt{},
	SchedulerRun{},
	Report{},
}
