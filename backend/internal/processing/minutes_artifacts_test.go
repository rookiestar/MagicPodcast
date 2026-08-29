package processing

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseTranscriptSegmentsSupportsMinutesAndHours(t *testing.T) {
	segments, err := parseTranscriptSegments(`
张三 00:00
开场

李四 01:02.345
中段第一行
中段第二行

王五 02:03:04.500
尾段
`)
	require.NoError(t, err)
	require.Equal(t, []TranscriptSegment{
		{Order: 1, Speaker: "张三", StartMS: 0, Text: "开场"},
		{Order: 2, Speaker: "李四", StartMS: 62345, Text: "中段第一行\n中段第二行"},
		{Order: 3, Speaker: "王五", StartMS: 7384500, Text: "尾段"},
	}, segments)
}

func TestParseTranscriptSegmentsRejectsMissingOrRegressingTimeline(t *testing.T) {
	for _, transcript := range []string{
		"没有时间轴",
		"张三 00:02\n后段\n李四 00:01\n前段",
		"张三 00:00\n\n李四 00:01\n有内容",
	} {
		_, err := parseTranscriptSegments(transcript)
		require.Error(t, err, transcript)
	}
}
