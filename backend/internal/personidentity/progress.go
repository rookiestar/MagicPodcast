package personidentity

import "context"

// RuntimeProgress is the sanitized connection/activity state of the current
// Runtime execution. Phases describe provider activity only; metadata carries
// provider-confirmed facts (such as will_retry) and never payloads.
type RuntimeProgress struct {
	Phase             string `json:"phase"`
	WillRetry         *bool  `json:"will_retry,omitempty"`
	Classification    string `json:"classification,omitempty"`
	LastActivityAgeMS *int64 `json:"last_activity_age_ms,omitempty"`
}

// PreparationProgress describes observable work, never model reasoning or an
// estimated percentage. It is scoped to the caller, not stored on the Service.
type PreparationProgress struct {
	Stage         string           `json:"stage"`
	SourceVersion string           `json:"source_version,omitempty"`
	Runtime       *RuntimeProgress `json:"runtime,omitempty"`
}
type preparationProgressKey struct{}

func WithPreparationProgress(ctx context.Context, report func(PreparationProgress)) context.Context {
	return context.WithValue(ctx, preparationProgressKey{}, report)
}

func reportPreparation(ctx context.Context, stage, source string) {
	if report, ok := ctx.Value(preparationProgressKey{}).(func(PreparationProgress)); ok {
		report(PreparationProgress{Stage: stage, SourceVersion: source})
	}
}

func reportRuntimeProgress(ctx context.Context, progress RuntimeProgress) {
	if report, ok := ctx.Value(preparationProgressKey{}).(func(PreparationProgress)); ok {
		report(PreparationProgress{Runtime: &progress})
	}
}
