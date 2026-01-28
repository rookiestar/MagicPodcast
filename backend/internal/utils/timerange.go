package utils

import (
	"fmt"
	"log"
	"time"
)

// TimeRangeMode 时间范围模式
type TimeRangeMode string

const (
	TimeRangeModeDaily  TimeRangeMode = "daily"  // 自动触发：使用实际触发时间，扫描过去24小时（例如8:35触发，则扫描昨天8:35到今天8:35）
	TimeRangeModeManual TimeRangeMode = "manual" // 手动触发：扫描过去N天（从触发时刻往回推）
)

// GetTimeRangeWindow 获取扫描时间窗口
// mode: "daily" | "manual"
// days: 扫描过去N天（对于 daily 和 manual 模式都适用）
// triggeredAt: 触发时间（对于 daily 模式应该传入实际触发时间）
func GetTimeRangeWindow(mode TimeRangeMode, days int, triggeredAt time.Time) (start, end time.Time, err error) {
	now := time.Now()

	switch mode {
	case TimeRangeModeDaily:
		// 自动触发模式：使用实际触发时间，扫描过去N天
		// 例如：8:35触发，1天范围 → 昨天8:35到今天8:35（24小时窗口）
		//       8:35触发，2天范围 → 前天8:35到今天8:35（48小时窗口）
		if triggeredAt.IsZero() {
			// 如果未提供触发时间，回退到使用当前时间
			triggeredAt = now
		}
		end := triggeredAt
		start := triggeredAt.AddDate(0, 0, -days)

		log.Printf("[GetTimeRangeWindow] Daily Mode: mode=%s, days=%d, triggeredAt=%s", mode, days, triggeredAt.Format("2006-01-02 15:04:05"))
		log.Printf("[GetTimeRangeWindow] Time Window: start=%s, end=%s", start.Format("2006-01-02 15:04:05"), end.Format("2006-01-02 15:04:05"))

		return start, end, nil

	case TimeRangeModeManual:
		// 手动触发模式：扫描过去N天
		// 例如：16:35触发，2天范围 → 前天16:35到今天16:35（48小时窗口）
		end = now
		start = now.AddDate(0, 0, -days)

		log.Printf("[GetTimeRangeWindow] Manual Mode: mode=%s, days=%d", mode, days)
		log.Printf("[GetTimeRangeWindow] Time Window: start=%s, end=%s", start.Format("2006-01-02 15:04:05"), end.Format("2006-01-02 15:04:05"))

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

	if episode.GetUpdatedDate() != nil {
		updateTime = *episode.GetUpdatedDate()
	} else {
		updateTime = episode.GetPublishedDate()
	}

	return updateTime.After(start) && updateTime.Before(end)
}
