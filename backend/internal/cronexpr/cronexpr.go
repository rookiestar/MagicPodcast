package cronexpr

import (
	"fmt"
	"strings"

	"github.com/robfig/cron/v3"
)

var parser = cron.NewParser(
	cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow,
)

// Parse accepts the repository's five- or six-field cron syntax and returns
// the normalized six-field expression used for persisted schedule identity.
func Parse(expression string) (string, cron.Schedule, error) {
	normalized, err := Normalize(expression)
	if err != nil {
		return "", nil, err
	}
	schedule, err := parser.Parse(normalized)
	if err != nil {
		return "", nil, fmt.Errorf("invalid cron %q: %w", strings.TrimSpace(expression), err)
	}
	return normalized, schedule, nil
}

func Normalize(expression string) (string, error) {
	fields := strings.Fields(strings.TrimSpace(expression))
	switch len(fields) {
	case 5:
		return "0 " + strings.Join(fields, " "), nil
	case 6:
		return strings.Join(fields, " "), nil
	default:
		return "", fmt.Errorf("cron must have 5 or 6 fields")
	}
}
