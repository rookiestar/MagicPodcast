package personidentity

import "context"

// PreparationProgress describes observable work, never model reasoning or an
// estimated percentage. It is scoped to the caller, not stored on the Service.
type PreparationProgress struct {
	Stage         string `json:"stage"`
	SourceVersion string `json:"source_version,omitempty"`
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
