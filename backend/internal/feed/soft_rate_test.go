package feed

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSoftRateDegradesOnAccessDeniedAndRecoversOnSuccess(t *testing.T) {
	ctrl := NewSoftRateController()
	domain := "feed.xyzfm.space"
	require.Equal(t, SoftRateNormal, ctrl.Tier(domain))

	ctrl.ObserveAccessDenied(domain)
	require.Equal(t, SoftRateCautious, ctrl.Tier(domain))
	require.Equal(t, softRateSpacingCautious, ctrl.Spacing(domain))

	ctrl.ObserveAccessDenied(domain)
	require.Equal(t, SoftRateSlow, ctrl.Tier(domain))
	require.Equal(t, softRateSpacingSlow, ctrl.Spacing(domain))

	// Floor: further 403s stay slow, never zero concurrency / never refuse.
	ctrl.ObserveAccessDenied(domain)
	require.Equal(t, SoftRateSlow, ctrl.Tier(domain))
	require.True(t, SoftRateNeverZero(ctrl.Tier(domain)))

	// Sustained success recovers one tier at a time.
	ctrl.ObserveSuccess(domain)
	require.Equal(t, SoftRateSlow, ctrl.Tier(domain), "single success does not recover")
	ctrl.ObserveSuccess(domain)
	require.Equal(t, SoftRateCautious, ctrl.Tier(domain))
	ctrl.ObserveSuccess(domain)
	ctrl.ObserveSuccess(domain)
	require.Equal(t, SoftRateNormal, ctrl.Tier(domain))
}

func TestSoftRateWaitSerializesWithoutHardBlock(t *testing.T) {
	ctrl := NewSoftRateController()
	domain := "feed.example.com"
	ctrl.ObserveAccessDenied(domain) // cautious → 2s spacing

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, ctrl.Wait(ctx, domain))
	start := time.Now()
	require.NoError(t, ctrl.Wait(ctx, domain))
	elapsed := time.Since(start)
	require.GreaterOrEqual(t, elapsed, softRateSpacingCautious-50*time.Millisecond)
}

func TestSoftRateWaitHonorsCancellation(t *testing.T) {
	ctrl := NewSoftRateController()
	domain := "feed.example.com"
	ctrl.ObserveAccessDenied(domain)
	ctrl.ObserveAccessDenied(domain) // slow

	require.NoError(t, ctrl.Wait(context.Background(), domain))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := ctrl.Wait(ctx, domain)
	require.ErrorIs(t, err, context.Canceled)
}

func TestSoftRateConcurrentWaitersDoNotDropToZero(t *testing.T) {
	ctrl := NewSoftRateController()
	domain := "feed.xyzfm.space"
	// Keep normal tier (0 spacing) so this is about admission, not delays.
	var admitted int32
	ctx := context.Background()
	done := make(chan struct{}, 8)
	for i := 0; i < 8; i++ {
		go func() {
			require.NoError(t, ctrl.Wait(ctx, domain))
			atomic.AddInt32(&admitted, 1)
			done <- struct{}{}
		}()
	}
	for i := 0; i < 8; i++ {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("soft rate waiter stalled — concurrency dropped to zero?")
		}
	}
	require.Equal(t, int32(8), atomic.LoadInt32(&admitted))
}
