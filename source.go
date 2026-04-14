package main

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// CalendarSource is the interface each calendar backend must implement.
type CalendarSource interface {
	Name() string
	FetchEvents(days int) ([]Meeting, error)
}

// MeetingAttendee represents a single attendee on a meeting.
type MeetingAttendee struct {
	Email          string `json:"email"`
	Name           string `json:"name,omitempty"`
	ResponseStatus string `json:"response_status,omitempty"`
	Organizer      bool   `json:"organizer,omitempty"`
	Self           bool   `json:"self,omitempty"`
	Optional       bool   `json:"optional,omitempty"`
}

// Conflict represents another meeting that overlaps with this one.
type Conflict struct {
	Summary string `json:"summary"`
	Start   string `json:"start"`
	End     string `json:"end"`
	Source  string `json:"source"`
}

// Meeting is the unified event representation across all sources.
type Meeting struct {
	Source         string            `json:"source"`
	Summary        string            `json:"summary"`
	Description    string            `json:"description,omitempty"`
	Start          string            `json:"start"`
	End            string            `json:"end"`
	Location       string            `json:"location,omitempty"`
	Status         string            `json:"status,omitempty"`
	OrganizerEmail string            `json:"organizer_email,omitempty"`
	OrganizerName  string            `json:"organizer_name,omitempty"`
	Attendees      []MeetingAttendee `json:"attendees,omitempty"`
	HangoutLink    string            `json:"hangout_link,omitempty"`
	HTMLLink       string            `json:"html_link,omitempty"`
	Conflicts      []Conflict        `json:"conflicts,omitempty"`
}

// Source color palette — auto-assigned in order, overridable in config.
var sourceColorPalette = []string{
	"63",  // blue-purple
	"170", // magenta
	"73",  // teal
	"114", // green
	"222", // yellow
	"210", // red
	"81",  // cyan
	"208", // orange
}

// SourceColorMap maps source name -> lipgloss color string.
type SourceColorMap map[string]string

// BuildSourceColors assigns colors to sources from palette, with config overrides.
func BuildSourceColors(sources []SourceConfig) SourceColorMap {
	m := make(SourceColorMap)
	paletteIdx := 0
	for _, s := range sources {
		if s.Color != "" {
			m[s.Name] = s.Color
		} else {
			m[s.Name] = sourceColorPalette[paletteIdx%len(sourceColorPalette)]
			paletteIdx++
		}
	}
	return m
}

// buildSources creates CalendarSource instances from config.
func buildSources(cfg *Config) ([]CalendarSource, error) {
	var sources []CalendarSource
	for _, sc := range cfg.Sources {
		switch sc.Type {
		case "google":
			sources = append(sources, NewGoogleSource(sc))
		case "caldav":
			sources = append(sources, NewCalDAVSource(sc))
		default:
			return nil, fmt.Errorf("unknown source type %q for source %q", sc.Type, sc.Name)
		}
	}
	return sources, nil
}

// fetchAllSources fetches events from all sources in parallel, merges and sorts.
// Returns meetings, any per-source warnings, and a fatal error if all sources failed.
func fetchAllSources(sources []CalendarSource, days int) ([]Meeting, []error, error) {
	type result struct {
		meetings []Meeting
		err      error
		name     string
	}

	results := make([]result, len(sources))
	var wg sync.WaitGroup

	for i, src := range sources {
		wg.Add(1)
		go func(idx int, s CalendarSource) {
			defer wg.Done()
			meetings, err := s.FetchEvents(days)
			results[idx] = result{meetings: meetings, err: err, name: s.Name()}
		}(i, src)
	}

	wg.Wait()

	var all []Meeting
	var errs []error
	for _, r := range results {
		if r.err != nil {
			errs = append(errs, fmt.Errorf("[%s] %w", r.name, r.err))
			continue
		}
		all = append(all, r.meetings...)
	}

	// Build the set of valid dates: today, today+1, ..., today+days-1
	now := time.Now()
	validDates := make(map[string]bool, days)
	for d := 0; d < days; d++ {
		date := time.Date(now.Year(), now.Month(), now.Day()+d, 0, 0, 0, 0, now.Location())
		validDates[date.Format("2006-01-02")] = true
	}

	// Filter out events whose local date falls outside the valid range
	filtered := make([]Meeting, 0, len(all))
	for _, m := range all {
		if validDates[localDate(m.Start)] {
			filtered = append(filtered, m)
		}
	}
	all = filtered

	// Sort by local date first, then all-day before timed, then by time.
	// This ensures all events for April 13 (including 17:00 PDT) come
	// before any April 14 events, even though 17:00 PDT == 00:00 UTC Apr 14.
	sort.SliceStable(all, func(i, j int) bool {
		di := localDate(all[i].Start)
		dj := localDate(all[j].Start)
		if di != dj {
			return di < dj
		}
		// Same date: all-day events first
		iAllDay := isAllDayString(all[i].Start)
		jAllDay := isAllDayString(all[j].Start)
		if iAllDay != jAllDay {
			return iAllDay
		}
		// Both timed: sort by actual time
		ti := parseStartTime(all[i].Start)
		tj := parseStartTime(all[j].Start)
		return ti.Before(tj)
	})

	// Detect overlapping events
	detectOverlaps(all)

	// If all sources failed, return fatal error
	if len(errs) > 0 && len(all) == 0 {
		return nil, errs, fmt.Errorf("all sources failed: %v", errs)
	}

	return all, errs, nil
}

// detectOverlaps marks meetings that overlap in time with each other.
// Two timed events overlap if one starts before the other ends.
// All-day events are excluded from overlap detection.
func detectOverlaps(meetings []Meeting) {
	for i := range meetings {
		if isAllDayString(meetings[i].Start) {
			continue
		}
		iStart := parseStartTime(meetings[i].Start)
		iEnd := parseStartTime(meetings[i].End)
		if iStart.IsZero() || iEnd.IsZero() {
			continue
		}

		for j := range meetings {
			if i == j {
				continue
			}
			if isAllDayString(meetings[j].Start) {
				continue
			}
			jStart := parseStartTime(meetings[j].Start)
			jEnd := parseStartTime(meetings[j].End)
			if jStart.IsZero() || jEnd.IsZero() {
				continue
			}

			// Two events overlap if one starts before the other ends
			if iStart.Before(jEnd) && jStart.Before(iEnd) {
				meetings[i].Conflicts = append(meetings[i].Conflicts, Conflict{
					Summary: meetings[j].Summary,
					Start:   meetings[j].Start,
					End:     meetings[j].End,
					Source:  meetings[j].Source,
				})
			}
		}
	}
}

// localDate extracts the local date as "2006-01-02" from an RFC3339 or date-only string.
func localDate(s string) string {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.Local().Format("2006-01-02")
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.Format("2006-01-02")
	}
	return s
}

// isAllDayString returns true if the start string is a date-only value (no time component).
func isAllDayString(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

// parseStartTime parses RFC3339 or date-only strings for sorting.
func parseStartTime(s string) time.Time {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t
	}
	return time.Time{}
}
