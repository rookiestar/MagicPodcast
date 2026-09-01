package processing

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type recordingLarkRunner struct {
	args    []string
	mode    os.FileMode
	payload []byte
}

func (r *recordingLarkRunner) Run(
	_ context.Context,
	directory string,
	args ...string,
) ([]byte, error) {
	r.args = append([]string(nil), args...)
	output := ""
	for index, arg := range args {
		if arg == "--output" && index+1 < len(args) {
			output = strings.TrimPrefix(args[index+1], "./")
			break
		}
	}
	if output == "" {
		return nil, errors.New("missing output")
	}
	mode := r.mode
	if mode == 0 {
		mode = 0o600
	}
	return nil, os.WriteFile(filepath.Join(directory, output), r.payload, mode)
}

func TestClassifyLarkCommandErrorDistinguishesPendingFromProcessingFailure(t *testing.T) {
	pending, recognized := classifyLarkCommandError(&larkCommandError{
		stderr: []byte(`{"error":{"code":123,"message":"transcript not ready"}}`),
	})
	require.True(t, recognized)
	require.ErrorIs(t, pending, errLarkMinutesPending)

	failed, recognized := classifyLarkCommandError(&larkCommandError{
		stderr: []byte(`{"error":{"type":"processing_failed","code":123,"message":"transcript processing failed"}}`),
	})
	require.False(t, recognized)
	require.False(t, errors.Is(failed, errLarkMinutesPending))
	var adapterErr *AdapterError
	require.ErrorAs(t, failed, &adapterErr)
	require.Equal(t, "lark_result_unknown", adapterErr.ErrorCode)
}

func TestFeishuDriveAudioDownloaderUsesProtectedUserScopedDownload(t *testing.T) {
	directory, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, os.Chmod(directory, 0o700))
	runner := &recordingLarkRunner{
		mode:    0o644,
		payload: []byte("managed audio"),
	}
	downloader, err := newFeishuDriveAudioDownloaderWithRunner(runner)
	require.NoError(t, err)

	require.NoError(t, downloader.Download(
		context.Background(),
		"boxcn_protected_audio_1234",
		directory,
		"recovered.mp3",
	))
	require.Equal(t, []string{
		"drive", "+download", "--file-token", "boxcn_protected_audio_1234",
		"--output", "./recovered.mp3", "--overwrite", "--as", "user",
		"--format", "json",
	}, runner.args)
	info, err := os.Lstat(filepath.Join(directory, "recovered.mp3"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}
