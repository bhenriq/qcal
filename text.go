package main

import (
	"fmt"
	"strings"
	"time"
)

// ANSI color codes for text output.
const (
	ansiReset   = "\033[0m"
	ansiBold    = "\033[1m"
	ansiDim     = "\033[2m"
	ansiRed     = "\033[31m"
	ansiGreen   = "\033[32m"
	ansiYellow  = "\033[33m"
	ansiBlue    = "\033[34m"
	ansiMagenta = "\033[35m"
	ansiCyan    = "\033[36m"
	ansiWhite   = "\033[37m"
)

func filterFuture(meetings []Meeting) []Meeting {
	now := time.Now()
	filtered := make([]Meeting, 0, len(meetings))
	for _, m := range meetings {
		t, err := time.Parse(time.RFC3339, m.End)
		if err != nil {
			// All-day events have date-only strings; keep them
			filtered = append(filtered, m)
			continue
		}
		if t.After(now) {
			filtered = append(filtered, m)
		}
	}
	return filtered
}

func printText(meetings []Meeting, colors SourceColorMap) {
	multiSource := len(colors) > 1
	now := time.Now()

	if len(meetings) == 0 {
		fmt.Printf("%s%sNo meetings found.%s\n", ansiDim, ansiWhite, ansiReset)
		return
	}

	// Group meetings by date
	groups := make(map[string][]Meeting)
	var dateOrder []string
	for _, m := range meetings {
		dateStr := m.Start
		if t, err := time.Parse(time.RFC3339, m.Start); err == nil {
			dateStr = t.Format("2006-01-02")
		}
		if _, exists := groups[dateStr]; !exists {
			dateOrder = append(dateOrder, dateStr)
		}
		groups[dateStr] = append(groups[dateStr], m)
	}

	for _, date := range dateOrder {
		dayMeetings := groups[date]

		// Date header
		d, err := time.Parse("2006-01-02", date)
		if err == nil {
			label := d.Format("Monday, January 2")
			if d.Format("2006-01-02") == now.Format("2006-01-02") {
				label += " (today)"
			}
			fmt.Printf("\n%s%s%s%s\n", ansiBold, ansiCyan, label, ansiReset)
			fmt.Printf("%s%s%s\n", ansiDim, strings.Repeat("─", len(label)+2), ansiReset)
		}

		for _, m := range dayMeetings {
			startTime, err := time.Parse(time.RFC3339, m.Start)
			isAllDay := err != nil
			endTime, _ := time.Parse(time.RFC3339, m.End)

			// Determine color based on timing
			var statusColor string
			var indicator string
			if isAllDay {
				statusColor = ansiMagenta
				indicator = "◆"
			} else if now.After(endTime) {
				statusColor = ansiDim
				indicator = "✓"
			} else if now.After(startTime) && now.Before(endTime) {
				statusColor = ansiGreen
				indicator = "▶"
			} else {
				statusColor = ansiYellow
				indicator = "○"
			}

			// Time range
			timeStr := "  all day  "
			if !isAllDay {
				timeStr = fmt.Sprintf("%s - %s",
					startTime.Format("15:04"),
					endTime.Format("15:04"),
				)
			}

			// Conflict warning (always reserve 2 chars for alignment)
			conflictWarn := "  "
			if len(m.Conflicts) > 0 {
				conflictWarn = "\033[38;5;208m⚠ \033[0m"
			}

			// Source tag (only show when multiple sources)
			sourceTag := ""
			if multiSource && m.Source != "" {
				sourceTag = fmt.Sprintf("  %s[%s]%s", ansiDim, m.Source, ansiReset)
			}

			// Summary line
			fmt.Printf("  %s%s %s%-13s%s %s%s%s%s%s%s\n",
				statusColor, indicator, ansiDim, timeStr, ansiReset,
				conflictWarn,
				statusColor, ansiBold, m.Summary, ansiReset,
				sourceTag,
			)

			// Location
			if m.Location != "" {
				fmt.Printf("    %s%s📍 %s%s\n", ansiDim, ansiWhite, m.Location, ansiReset)
			}

			// Meet link
			if m.HangoutLink != "" {
				fmt.Printf("    %s%s🔗 %s%s\n", ansiDim, ansiBlue, m.HangoutLink, ansiReset)
			}
		}
	}

	// Legend
	fmt.Printf("\n%s%s  ✓ past  ▶ now  ○ upcoming  ◆ all day%s\n\n", ansiDim, ansiWhite, ansiReset)
}
