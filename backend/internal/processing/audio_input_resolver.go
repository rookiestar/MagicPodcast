package processing

import (
	"context"
	"fmt"
	"strings"
)

type readyAudioResolver interface {
	ResolveReadyAudio(context.Context, uint) (ReadyAudio, error)
}

type ManagedAudioInputResolver struct {
	audio           readyAudioResolver
	pipelineVersion string
}

func (r *ManagedAudioInputResolver) PipelineVersion() string {
	return r.pipelineVersion
}

func NewManagedAudioInputResolver(
	audio readyAudioResolver,
	pipelineVersion string,
) (*ManagedAudioInputResolver, error) {
	pipelineVersion = strings.TrimSpace(pipelineVersion)
	if audio == nil || pipelineVersion == "" || len(pipelineVersion) > 100 {
		return nil, fmt.Errorf("managed processing input configuration is invalid")
	}
	return &ManagedAudioInputResolver{
		audio:           audio,
		pipelineVersion: pipelineVersion,
	}, nil
}

func (r *ManagedAudioInputResolver) ResolveProcessingInput(
	ctx context.Context,
	episodeID uint,
) (ProcessingInput, error) {
	audio, err := r.audio.ResolveReadyAudio(ctx, episodeID)
	if err != nil {
		return ProcessingInput{}, err
	}
	return ProcessingInput{
		AudioDigest:     audio.SHA256,
		PipelineVersion: r.pipelineVersion,
	}, nil
}
