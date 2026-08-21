package models

import "testing"

func TestInferHomepageReportType(t *testing.T) {
	tests := []struct {
		name     string
		schedule string
		want     HomepageReportType
	}{
		{name: "six field daily", schedule: "0 0 8 * * *", want: HomepageReportTypeDaily},
		{name: "five field daily", schedule: "0 8 * * *", want: HomepageReportTypeDaily},
		{name: "single weekday weekly", schedule: "0 0 8 * * 1", want: HomepageReportTypeWeekly},
		{name: "named weekday weekly", schedule: "0 0 8 * * MON", want: HomepageReportTypeWeekly},
		{name: "multiple weekdays custom", schedule: "0 0 8 * * 1-5", want: HomepageReportTypeCustom},
		{name: "biweekly style custom", schedule: "0 0 8 * * 1/2", want: HomepageReportTypeCustom},
		{name: "monthly custom", schedule: "0 0 8 1 * *", want: HomepageReportTypeCustom},
		{name: "twice monthly custom", schedule: "0 0 8 2,16 * *", want: HomepageReportTypeCustom},
		{name: "invalid fields custom", schedule: "bad bad bad * * *", want: HomepageReportTypeCustom},
		{name: "invalid custom", schedule: "not-a-cron", want: HomepageReportTypeCustom},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := InferHomepageReportType(tt.schedule); got != tt.want {
				t.Fatalf("InferHomepageReportType(%q) = %q, want %q", tt.schedule, got, tt.want)
			}
		})
	}
}
