package models

import "time"

// ConsumptionQueueOrder stores the independently maintained order revision for
// each fixed consumption queue. Queue position belongs to an episode decision;
// this row lets clients detect a stale queue layout before changing it.
type ConsumptionQueueOrder struct {
	QueueState string    `gorm:"primaryKey;size:20" json:"queue_state"`
	Revision   int64     `gorm:"not null" json:"revision"`
	UpdatedAt  time.Time `gorm:"not null" json:"updated_at"`
}

func (ConsumptionQueueOrder) TableName() string {
	return "consumption_queue_orders"
}
