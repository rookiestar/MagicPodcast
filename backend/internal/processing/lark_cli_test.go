package processing

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

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
