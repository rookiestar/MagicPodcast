package episode

import "testing"

func TestFromTitleRecognizesCommonEpisodeLabels(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  string
	}{
		{name: "episode marker", title: "E246 对话餐饮收尸人", want: "246"},
		{name: "volume marker", title: "Vol.228 对话单伟建", want: "228"},
		{name: "season episode marker", title: "节目 S10E24 特别篇", want: "S10E24"},
		{name: "hash prefix", title: "#664.Huberman Lab", want: "664"},
		{name: "full width separator", title: "150︳言语是种权力", want: "150"},
		{name: "no label", title: "China's return as a global study hub", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FromTitle(tt.title); got != tt.want {
				t.Fatalf("FromTitle(%q) = %q, want %q", tt.title, got, tt.want)
			}
		})
	}
}

func TestNormalizePrefersTitleAndHidesUnreliableStoredNumbers(t *testing.T) {
	tests := []struct {
		name   string
		title  string
		stored string
		want   string
	}{
		{name: "title overrides feed metadata", title: "节目 S10E24 特别篇", stored: "20240438", want: "S10E24"},
		{name: "invalid stored number is hidden", title: "无期号标题", stored: "20240438", want: ""},
		{name: "unmarked stored number is not reliable", title: "无期号标题", stored: "2665", want: ""},
		{name: "rss position is not reliable", title: "昆山杜克大学周忆粟：AI 来了，年轻人的梯子被抽掉了", stored: "1", want: ""},
		{name: "stored marker is canonicalized", title: "无期号标题", stored: "E246", want: "246"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Normalize(tt.title, tt.stored); got != tt.want {
				t.Fatalf("Normalize(%q, %q) = %q, want %q", tt.title, tt.stored, got, tt.want)
			}
		})
	}
}
