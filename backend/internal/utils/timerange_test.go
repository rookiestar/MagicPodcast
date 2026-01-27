package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestGetTimeRangeWindow_DailyMode 测试daily模式的时间范围计算
func TestGetTimeRangeWindow_DailyMode(t *testing.T) {
	// 设置一个固定的时间：2024-01-15 08:35:00
	loc, _ := time.LoadLocation("Asia/Shanghai")
	triggeredAt := time.Date(2024, 1, 15, 8, 35, 0, 0, loc)

	tests := []struct {
		name         string
		days         int
		expectedDiff time.Duration // 期望的时间差
	}{
		{
			name:         "1天范围",
			days:         1,
			expectedDiff: 24 * time.Hour,
		},
		{
			name:         "2天范围",
			days:         2,
			expectedDiff: 48 * time.Hour,
		},
		{
			name:         "7天范围",
			days:         7,
			expectedDiff: 7 * 24 * time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, err := GetTimeRangeWindow(TimeRangeModeDaily, tt.days, triggeredAt)

			// 验证没有错误
			assert.NoError(t, err)

			// 验证结束时间等于触发时间
			assert.Equal(t, triggeredAt, end, "结束时间应该等于触发时间")

			// 验证时间范围大小
			actualDiff := end.Sub(start)
			assert.Equal(t, tt.expectedDiff, actualDiff, "时间范围应该为%d天", tt.days)

			// 验证开始时间 = 触发时间 - N天
			expectedStart := triggeredAt.AddDate(0, 0, -tt.days)
			assert.Equal(t, expectedStart, start, "开始时间应该等于触发时间减去%d天", tt.days)

			// 打印结果便于调试
			t.Logf("触发时间: %s", triggeredAt.Format("2006-01-02 15:04:05"))
			t.Logf("时间窗口: %s ~ %s", start.Format("2006-01-02 15:04:05"), end.Format("2006-01-02 15:04:05"))
		})
	}
}

// TestGetTimeRangeWindow_DailyMode_835Trigger 测试用户报告的场景：8:35触发
func TestGetTimeRangeWindow_DailyMode_835Trigger(t *testing.T) {
	// 场景：每天8:35触发，抓取最近1天
	loc := time.FixedZone("CST", 8*3600) // 东八区
	triggeredAt := time.Date(2024, 1, 15, 8, 35, 0, 0, loc)

	start, end, err := GetTimeRangeWindow(TimeRangeModeDaily, 1, triggeredAt)

	assert.NoError(t, err)

	// 验证时间窗口
	expectedEnd := time.Date(2024, 1, 15, 8, 35, 0, 0, loc)
	expectedStart := time.Date(2024, 1, 14, 8, 35, 0, 0, loc)

	assert.Equal(t, expectedEnd, end, "结束时间应该是2024-01-15 08:35:00")
	assert.Equal(t, expectedStart, start, "开始时间应该是2024-01-14 08:35:00")

	// 验证是24小时窗口
	diff := end.Sub(start)
	assert.Equal(t, 24*time.Hour, diff, "时间范围应该是24小时")

	t.Logf("✅ 场景验证通过：8:35触发，1天范围")
	t.Logf("   时间窗口: %s ~ %s", start.Format("2006-01-02 15:04:05"), end.Format("2006-01-02 15:04:05"))
}

// TestGetTimeRangeWindow_DailyMode_ZeroTriggerTime 测试触发时间为零值的情况
func TestGetTimeRangeWindow_DailyMode_ZeroTriggerTime(t *testing.T) {
	beforeTime := time.Now().Add(-1 * time.Second)

	// 传入零值触发时间
	start, end, err := GetTimeRangeWindow(TimeRangeModeDaily, 1, time.Time{})

	assert.NoError(t, err)
	assert.False(t, end.IsZero(), "结束时间不应该为零值")
	assert.False(t, start.IsZero(), "开始时间不应该为零值")
	assert.True(t, end.After(beforeTime), "结束时间应该在当前时间之后")

	// 验证时间范围是24小时
	diff := end.Sub(start)
	assert.Equal(t, 24*time.Hour, diff)

	t.Logf("✅ 零值触发时间测试通过，使用当前时间")
}

// TestGetTimeRangeWindow_ManualMode 测试manual模式的时间范围计算
func TestGetTimeRangeWindow_ManualMode(t *testing.T) {
	beforeTime := time.Now()

	tests := []struct {
		name         string
		days         int
		expectedDiff time.Duration
	}{
		{
			name:         "1天范围",
			days:         1,
			expectedDiff: 24 * time.Hour,
		},
		{
			name:         "3天范围",
			days:         3,
			expectedDiff: 72 * time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, err := GetTimeRangeWindow(TimeRangeModeManual, tt.days, time.Time{})

			assert.NoError(t, err)

			// 验证结束时间接近当前时间
			assert.True(t, end.After(beforeTime) || end.Equal(beforeTime),
				"结束时间应该在当前时间之后或等于当前时间")

			// 验证时间范围大小
			actualDiff := end.Sub(start)
			assert.Equal(t, tt.expectedDiff, actualDiff, "时间范围应该为%d天", tt.days)

			// 验证开始时间 = 当前时间 - N天
			expectedStart := end.AddDate(0, 0, -tt.days)
			assert.WithinDuration(t, expectedStart, start, time.Second, "开始时间计算应该正确")

			t.Logf("手动触发 %d天范围: %s ~ %s", tt.days,
				start.Format("2006-01-02 15:04:05"), end.Format("2006-01-02 15:04:05"))
		})
	}
}

// TestGetTimeRangeWindow_UnsupportedMode 测试不支持的模式
func TestGetTimeRangeWindow_UnsupportedMode(t *testing.T) {
	_, _, err := GetTimeRangeWindow(TimeRangeMode("invalid"), 1, time.Now())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported time range mode")
}

// TestGetTimeRangeWindow_DailyMode_DifferentTriggerTimes 测试不同触发时间
func TestGetTimeRangeWindow_DailyMode_DifferentTriggerTimes(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")

	triggerTimes := []struct {
		timeStr string
		hour    int
		minute  int
	}{
		{"00:00", 0, 0},
		{"06:30", 6, 30},
		{"12:00", 12, 0},
		{"18:45", 18, 45},
		{"23:59", 23, 59},
	}

	for _, tt := range triggerTimes {
		t.Run(tt.timeStr, func(t *testing.T) {
			triggeredAt := time.Date(2024, 1, 15, tt.hour, tt.minute, 0, 0, loc)
			start, end, err := GetTimeRangeWindow(TimeRangeModeDaily, 1, triggeredAt)

			assert.NoError(t, err)

			// 验证结束时间的时间部分与触发时间一致
			assert.Equal(t, tt.hour, end.Hour(), "结束时间的小时应该等于触发时间")
			assert.Equal(t, tt.minute, end.Minute(), "结束时间的分钟应该等于触发时间")

			// 验证开始时间的时间部分也与触发时间一致（只是日期不同）
			assert.Equal(t, tt.hour, start.Hour(), "开始时间的小时应该等于触发时间")
			assert.Equal(t, tt.minute, start.Minute(), "开始时间的分钟应该等于触发时间")

			// 验证日期相差1天
			dayDiff := int(end.Sub(start).Hours() / 24)
			assert.Equal(t, 1, dayDiff, "日期应该相差1天")

			t.Logf("✅ %s触发: %s ~ %s", tt.timeStr,
				start.Format("2006-01-02 15:04:05"), end.Format("2006-01-02 15:04:05"))
		})
	}
}
