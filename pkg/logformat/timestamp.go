package logformat

import (
	"regexp"
	"time"
)

// TimestampParser detects and parses timestamps from log lines
type TimestampParser struct {
	patterns []timestampPattern
}

type timestampPattern struct {
	regex  *regexp.Regexp
	layout string
}

// NewTimestampParser creates a parser with common timestamp formats
func NewTimestampParser() *TimestampParser {
	return &TimestampParser{
		patterns: []timestampPattern{
			// ISO 8601 / RFC 3339 variants
			// 2024-01-15T10:30:45.123Z
			// 2024-01-15T10:30:45.123+00:00
			{
				regex:  regexp.MustCompile(`(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{3})?(?:Z|[+-]\d{2}:\d{2})?)`),
				layout: time.RFC3339,
			},
			// Common log format with milliseconds
			// 2024-01-15 10:30:45.123
			{
				regex:  regexp.MustCompile(`(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{3})`),
				layout: "2006-01-02 15:04:05.000",
			},
			// Common log format without milliseconds
			// 2024-01-15 10:30:45
			{
				regex:  regexp.MustCompile(`(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})`),
				layout: "2006-01-02 15:04:05",
			},
			// Syslog format
			// Jan 15 10:30:45
			{
				regex:  regexp.MustCompile(`([A-Z][a-z]{2} \d{1,2} \d{2}:\d{2}:\d{2})`),
				layout: "Jan 2 15:04:05",
			},
			// Apache/nginx common log format
			// 15/Jan/2024:10:30:45 +0000
			{
				regex:  regexp.MustCompile(`(\d{2}/[A-Z][a-z]{2}/\d{4}:\d{2}:\d{2}:\d{2} [+-]\d{4})`),
				layout: "02/Jan/2006:15:04:05 -0700",
			},
			// Unix timestamp (seconds)
			// 1705315845
			{
				regex:  regexp.MustCompile(`^(\d{10})(?:\D|$)`),
				layout: "unix",
			},
			// Unix timestamp with milliseconds
			// 1705315845123
			{
				regex:  regexp.MustCompile(`^(\d{13})(?:\D|$)`),
				layout: "unix_ms",
			},
			// Bracket format common in many loggers
			// [2024-01-15 10:30:45.123]
			{
				regex:  regexp.MustCompile(`\[(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}(?:\.\d{3})?)\]`),
				layout: "2006-01-02 15:04:05.000",
			},
			// Time only (assume today)
			// 10:30:45.123
			{
				regex:  regexp.MustCompile(`^(\d{2}:\d{2}:\d{2}(?:\.\d{3})?)`),
				layout: "15:04:05.000",
			},
		},
	}
}

// Parse attempts to extract a timestamp from a log line
func (p *TimestampParser) Parse(content []byte) *time.Time {
	line := string(content)

	for _, pattern := range p.patterns {
		matches := pattern.regex.FindStringSubmatch(line)
		if len(matches) < 2 {
			continue
		}

		timeStr := matches[1]

		// Handle unix timestamps specially
		if pattern.layout == "unix" {
			var ts int64
			if _, err := parseUnixTimestamp(timeStr, &ts); err == nil {
				t := time.Unix(ts, 0)
				return &t
			}
			continue
		}

		if pattern.layout == "unix_ms" {
			var ts int64
			if _, err := parseUnixTimestamp(timeStr, &ts); err == nil {
				t := time.UnixMilli(ts)
				return &t
			}
			continue
		}

		// Try parsing with the pattern's layout
		// Try with milliseconds first, then without
		layouts := []string{pattern.layout}
		if pattern.layout == "2006-01-02 15:04:05.000" {
			layouts = append(layouts, "2006-01-02 15:04:05")
		}
		if pattern.layout == "15:04:05.000" {
			layouts = append(layouts, "15:04:05")
		}

		for _, layout := range layouts {
			t, err := time.Parse(layout, timeStr)
			if err == nil {
				// For time-only formats, use today's date
				if layout == "15:04:05" || layout == "15:04:05.000" {
					now := time.Now()
					t = time.Date(now.Year(), now.Month(), now.Day(),
						t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.Local)
				}
				// For syslog format without year, use current year
				if layout == "Jan 2 15:04:05" {
					t = time.Date(time.Now().Year(), t.Month(), t.Day(),
						t.Hour(), t.Minute(), t.Second(), 0, time.Local)
				}
				return &t
			}
		}
	}

	return nil
}

// parseUnixTimestamp parses a string as a unix timestamp
func parseUnixTimestamp(s string, result *int64) (int, error) {
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int64(c-'0')
	}
	*result = n
	return 0, nil
}

// FormatTime formats a timestamp for display
func FormatTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("15:04:05")
}

// FormatTimeWithDate formats a timestamp with date for display
func FormatTimeWithDate(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}
