package processing

import (
	"context"
	"fmt"
	"time"
)

type testLarkRunFunc func(
	context.Context,
	string,
	...string,
) ([]byte, error)

func (f testLarkRunFunc) Run(
	ctx context.Context,
	cwd string,
	args ...string,
) ([]byte, error) {
	return f(ctx, cwd, args...)
}

func NewFeishuMinutesAdapterForTest(
	run func(context.Context, string, ...string) ([]byte, error),
	workRoot string,
	readyAudio ReadyAudioLookup,
	now func() time.Time,
) (*FeishuMinutesAdapter, error) {
	if run == nil {
		return nil, fmt.Errorf("Feishu Minutes CLI runner is required")
	}
	adapter, err := newFeishuMinutesAdapterWithRunner(
		testLarkRunFunc(run),
		workRoot,
		readyAudio,
	)
	if err != nil {
		return nil, err
	}
	adapter.now = now
	return adapter, nil
}
