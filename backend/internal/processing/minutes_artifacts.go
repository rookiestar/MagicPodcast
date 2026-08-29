package processing

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var transcriptSegmentHeaderPattern = regexp.MustCompile(
	`^(.+?)\s+((?:\d{1,3}:)?[0-5]\d:[0-5]\d(?:\.\d{1,9})?)$`,
)

func normalizeMinutesSummary(raw string) (string, error) {
	summary := strings.TrimSpace(strings.ReplaceAll(raw, "\r\n", "\n"))
	if summary == "" {
		return "", NewAdapterError(
			"summary_empty",
			"Feishu Minutes summary is empty; retry after the Minute finishes processing",
			false,
		)
	}
	if !strings.HasPrefix(summary, "#") {
		summary = "# 纪要\n\n" + summary
	}
	return summary + "\n", nil
}

func normalizeTranscript(
	raw string,
) (string, []TranscriptSegment, error) {
	text := strings.TrimSpace(strings.ReplaceAll(raw, "\r\n", "\n"))
	if text == "" {
		return "", nil, NewAdapterError(
			"transcript_empty",
			"Feishu transcript is empty; retry after the Minute finishes processing",
			false,
		)
	}
	segments, err := parseTranscriptSegments(text)
	if err != nil {
		return "", nil, NewAdapterError(
			"transcript_timeline_invalid",
			"Feishu transcript timestamps could not be parsed; inspect the Minute transcript format",
			false,
		)
	}
	if !strings.HasPrefix(text, "#") {
		text = "# 逐字稿\n\n" + text
	}
	return text + "\n", segments, nil
}

func parseTranscriptSegments(raw string) ([]TranscriptSegment, error) {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	segments := make([]TranscriptSegment, 0)
	var (
		speaker           string
		startMS           int64
		body              []string
		previousLineBlank = true
	)
	flush := func() error {
		if speaker == "" {
			return nil
		}
		text := strings.TrimSpace(strings.Join(body, "\n"))
		if text == "" {
			return fmt.Errorf("transcript segment %d has no text", len(segments)+1)
		}
		if len(segments) > 0 && startMS < segments[len(segments)-1].StartMS {
			return fmt.Errorf("transcript timestamps are not monotonic")
		}
		segments = append(segments, TranscriptSegment{
			Order:   len(segments) + 1,
			Speaker: speaker,
			StartMS: startMS,
			Text:    text,
		})
		return nil
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		match := transcriptSegmentHeaderPattern.FindStringSubmatch(trimmed)
		// Minutes separates speaker blocks with a blank line. Restricting
		// headers to block boundaries keeps spoken lines ending in times intact.
		if len(match) == 3 && (speaker == "" || previousLineBlank) {
			if err := flush(); err != nil {
				return nil, err
			}
			parsedStart, err := parseRelativeTimestampMS(match[2])
			if err != nil {
				return nil, err
			}
			speaker = strings.TrimSpace(match[1])
			if speaker == "" {
				return nil, fmt.Errorf("transcript speaker is empty")
			}
			startMS = parsedStart
			body = body[:0]
			previousLineBlank = false
			continue
		}
		if speaker == "" {
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				previousLineBlank = trimmed == ""
				continue
			}
			return nil, fmt.Errorf("transcript content precedes the first timestamp")
		}
		body = append(body, line)
		previousLineBlank = trimmed == ""
	}
	if err := flush(); err != nil {
		return nil, err
	}
	if len(segments) == 0 {
		return nil, fmt.Errorf("transcript has no timestamped segments")
	}
	return segments, nil
}

func parseRelativeTimestampMS(value string) (int64, error) {
	parts := strings.SplitN(value, ".", 2)
	clock := strings.Split(parts[0], ":")
	if len(clock) != 2 && len(clock) != 3 {
		return 0, fmt.Errorf("invalid relative timestamp")
	}
	values := make([]int64, len(clock))
	for index, part := range clock {
		number, err := strconv.ParseInt(part, 10, 64)
		if err != nil || number < 0 {
			return 0, fmt.Errorf("invalid relative timestamp")
		}
		values[index] = number
	}
	var hours, minutes, seconds int64
	if len(values) == 3 {
		hours, minutes, seconds = values[0], values[1], values[2]
	} else {
		minutes, seconds = values[0], values[1]
	}
	if minutes > 59 || seconds > 59 {
		return 0, fmt.Errorf("invalid relative timestamp")
	}
	milliseconds := int64(0)
	if len(parts) == 2 {
		fraction := parts[1]
		if len(fraction) > 3 {
			fraction = fraction[:3]
		}
		for len(fraction) < 3 {
			fraction += "0"
		}
		number, err := strconv.ParseInt(fraction, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid relative timestamp")
		}
		milliseconds = number
	}
	return ((hours*60+minutes)*60+seconds)*1000 + milliseconds, nil
}
