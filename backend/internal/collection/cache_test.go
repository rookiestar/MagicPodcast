package collection

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPreviewStoreIsBounded(t *testing.T) {
	store := newPreviewStoreWithTTL(time.Now, time.Hour)
	t.Cleanup(store.stopTimers)

	tokens := make([]string, 0, previewStoreCapacity+1)
	for index := 0; index < previewStoreCapacity+1; index++ {
		token, err := store.put(&Draft{})
		require.NoError(t, err)
		tokens = append(tokens, token)
	}

	store.mu.Lock()
	count := len(store.entries)
	store.mu.Unlock()
	require.Equal(t, previewStoreCapacity, count)

	_, err := store.take(tokens[0])
	require.ErrorIs(t, err, ErrPreviewNotFound)
}

func TestPreviewStoreExpiresWithoutAnotherOperation(t *testing.T) {
	store := newPreviewStoreWithTTL(time.Now, 10*time.Millisecond)
	t.Cleanup(store.stopTimers)
	token, err := store.put(&Draft{})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		store.mu.Lock()
		defer store.mu.Unlock()
		_, exists := store.entries[token]
		return !exists
	}, time.Second, 5*time.Millisecond)
}

func TestRefreshStoreIsBounded(t *testing.T) {
	store := newRefreshStoreWithTTL(time.Now, time.Hour)
	t.Cleanup(store.stopTimers)

	for index := 0; index < previewStoreCapacity+1; index++ {
		_, err := store.put(1, index, &Draft{})
		require.NoError(t, err)
	}

	store.mu.Lock()
	count := len(store.entries)
	store.mu.Unlock()
	require.Equal(t, previewStoreCapacity, count)
}
