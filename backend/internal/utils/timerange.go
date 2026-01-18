package utils

import (
	"fmt"
	"time"
)

// TimeRangeMode 时间范围模式
type TimeRangeMode string

const (
	TimeRangeModeDaily  TimeRangeMode = "daily"  // 自动触发：每天8:00执行，扫描昨天8:00到今天8:00
	TimeRangeModeManual TimeRangeMode = "manual" // 手动触发：扫描过去N天（从触发时刻往回推）
)

// GetTimeRangeWindow 获取扫描时间窗口
// mode: "daily" | "manual"
// days: 对于manual模式，表示扫描过去N天
// triggeredAt: 触发时间
func GetTimeRangeWindow(mode TimeRangeMode, days int, triggeredAt time.Time) (start, end time.Time, err error) {
	now := time.Now()

	switch mode {
	case TimeRangeModeDaily:
		// 自动触发模式：每天上午8:00执行
		// 扫描昨天8:00到今天8:00（24小时窗口）
		today8am := time.Date(now.Year(), now.Month(), now.Day(), 8, 0, 0, 0, now.Location())
		yesterday8am := today8am.Add(-24 * time.Hour)

		return yesterday8am, today8am, nil

	case TimeRangeModeManual:
		// 手动触发模式：扫描过去N天
		// 例如：16:35触发，2天范围 → 前天16:35到今天16:35（48小时窗口）
		end = now
		start = now.AddDate(0, 0, -days)

		return start, end, nil

	default:
		return time.Time{}, time.Time{}, fmt.Errorf("unsupported time range mode: %s", mode)
	}
}

// IsEpisodeInTimeRange 判断episode是否在时间范围内
// 使用 updated_date（更新时间）或published_date（发布时间）判断
// 如果updated_date为空，则使用published_date
func IsEpisodeInTimeRange(episode interface {
	GetUpdatedDate() *time.Time
	GetPublishedDate() time.Time
}, start, end time.Time) bool {
	var updateTime time.Time

	if episode.GetUpdatedDate != nil {
		updateTime = *episode.GetUpdatedDate()
	} else {
		updateTime = episode.GetPublishedDate()
	}

	return updateTime.After(start) && updateTime.Before(end)
}
