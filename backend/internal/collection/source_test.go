package collection

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCollectionURL_AcceptsVerifiedHostAndPath(t *testing.T) {
	parsed, err := ParseCollectionURL("https://www.xiaoyuzhoufm.com/collection/episode/" + sampleCollectionID)
	require.NoError(t, err)
	assert.Equal(t, sampleCollectionID, parsed.ExternalID)
	assert.Equal(t, "https://www.xiaoyuzhoufm.com/collection/episode/"+sampleCollectionID, parsed.Raw)
}

func TestParseCollectionURL_RejectsAnythingBeyondNarrowScope(t *testing.T) {
	cases := []struct {
		name string
		url  string
	}{
		{"空", ""},
		{"http降级", "http://www.xiaoyuzhoufm.com/collection/episode/" + sampleCollectionID},
		{"其他主机", "https://evil.example.com/collection/episode/" + sampleCollectionID},
		{"子域伪装", "https://www.xiaoyuzhoufm.com.evil.example.com/collection/episode/" + sampleCollectionID},
		{"IP直连", "https://127.0.0.1/collection/episode/" + sampleCollectionID},
		{"本地端口", "http://localhost:8080/collection/episode/" + sampleCollectionID},
		{"带端口", "https://www.xiaoyuzhoufm.com:8443/collection/episode/" + sampleCollectionID},
		{"带用户信息", "https://user@www.xiaoyuzhoufm.com/collection/episode/" + sampleCollectionID},
		{"错误路径", "https://www.xiaoyuzhoufm.com/podcast/" + sampleCollectionID},
		{"非十六进制ID", "https://www.xiaoyuzhoufm.com/collection/episode/zzzzzzzzzzzzzzzzzzzzzzzz"},
		{"短ID", "https://www.xiaoyuzhoufm.com/collection/episode/abc123"},
		{"单集页而非清单", "https://www.xiaoyuzhoufm.com/episode/" + sampleCollectionID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseCollectionURL(tc.url)
			require.ErrorIs(t, err, ErrInvalidCollectionURL)
		})
	}
}

// stubFetcher 返回可控来源响应的抓取函数，替代真实网络；状态映射与生产抓取器一致。
func stubFetcher(response func(pageURL string) (int, string), calls *[]string) fetchFunc {
	return func(ctx context.Context, pageURL string) ([]byte, error) {
		if calls != nil {
			*calls = append(*calls, pageURL)
		}
		status, body := response(pageURL)
		switch {
		case status == 0:
			return nil, ErrSourceUnavailable
		case status == http.StatusForbidden || status == http.StatusUnauthorized:
			return nil, ErrSourceForbidden
		case status != http.StatusOK:
			return nil, ErrSourceUnavailable
		}
		return []byte(body), nil
	}
}

func newStubService(t *testing.T, fetch fetchFunc) *Service {
	t.Helper()
	service := NewService(newTestDB(t))
	service.fetch = fetch
	return service
}

func TestPreview_FetchesCanonicalURLAndBindsPreview(t *testing.T) {
	var calls []string
	service := newStubService(t, stubFetcher(func(pageURL string) (int, string) {
		return http.StatusOK, loadSampleHTML(t)
	}, &calls))

	result, err := service.Preview(context.Background(), "https://www.xiaoyuzhoufm.com/collection/episode/"+sampleCollectionID+"?utm=x")
	require.NoError(t, err)
	require.Len(t, calls, 1)
	// 抓取始终使用规范化地址，查询参数不进入抓取。
	assert.Equal(t, "https://www.xiaoyuzhoufm.com/collection/episode/"+sampleCollectionID, calls[0])
	assert.Equal(t, 8, result.ReadCount)
	assert.False(t, result.TotalKnown)
	assert.Equal(t, sampleAuthor, result.Author)
	assert.Len(t, result.Items, 8)
	assert.Equal(t, sampleFirstRecommendations[3], result.Items[3].Recommendation)
}

func TestPreview_MapsSourceFailures(t *testing.T) {
	cases := []struct {
		name    string
		fetch   fetchFunc
		wantErr error
	}{
		{"403", stubFetcher(func(string) (int, string) { return http.StatusForbidden, "" }, nil), ErrSourceForbidden},
		{"网络失败", func(context.Context, string) ([]byte, error) {
			return nil, fmt.Errorf("%w: %v", ErrSourceUnavailable, errors.New("dial timeout"))
		}, ErrSourceUnavailable},
		{"超限大小", func(context.Context, string) ([]byte, error) { return nil, ErrSourceTooLarge }, ErrSourceTooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service := newStubService(t, tc.fetch)
			_, err := service.Preview(context.Background(), "https://www.xiaoyuzhoufm.com/collection/episode/"+sampleCollectionID)
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestPreview_RejectsUnsupportedURLBeforeFetching(t *testing.T) {
	var calls []string
	service := newStubService(t, stubFetcher(func(string) (int, string) {
		return http.StatusOK, loadSampleHTML(t)
	}, &calls))

	_, err := service.Preview(context.Background(), "https://evil.example.com/collection/episode/"+sampleCollectionID)
	require.ErrorIs(t, err, ErrInvalidCollectionURL)
	assert.Empty(t, calls, "不支持的主机不发起任何请求")
}
