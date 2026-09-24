package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/caldav"
	"github.com/teambition/rrule-go"
)

func mustParseURL(rawURL string) *url.URL {
	u, _ := url.Parse(rawURL)
	return u
}

// CalDAVSource implements CalendarSource for CalDAV servers (Fastmail, iCloud, Nextcloud, etc.).
type CalDAVSource struct {
	name     string
	url      string
	username string
	password string
	passCmd  string
	// calLabel is the display name of the calendar currently being parsed.
	calLabel string
}

func NewCalDAVSource(cfg SourceConfig) *CalDAVSource {
	return &CalDAVSource{
		name:     cfg.Name,
		url:      cfg.URL,
		username: cfg.Username,
		password: cfg.Password,
		passCmd:  cfg.PassCmd,
	}
}

func (c *CalDAVSource) Name() string { return c.name }

func (c *CalDAVSource) FetchEvents(days int) ([]Meeting, error) {
	ctx := context.Background()

	password := c.password
	if password == "" && c.passCmd != "" {
		pw, err := runPassCmd(c.passCmd)
		if err != nil {
			return nil, err
		}
		password = pw
	}

	httpClient := webdav.HTTPClientWithBasicAuth(http.DefaultClient, c.username, password)

	// Try .well-known discovery to find the real CalDAV endpoint
	endpoint := c.url
	wellKnown := strings.TrimRight(endpoint, "/") + "/.well-known/caldav"
	resp, err := httpClient.Do(&http.Request{
		Method: "PROPFIND",
		URL:    mustParseURL(wellKnown),
		Header: http.Header{"Depth": {"0"}},
	})
	if err == nil {
		resp.Body.Close()
		if resp.Request != nil && resp.Request.URL != nil {
			finalURL := resp.Request.URL
			endpoint = finalURL.Scheme + "://" + finalURL.Host + finalURL.Path
		}
	}

	client, err := caldav.NewClient(httpClient, endpoint)
	if err != nil {
		return nil, fmt.Errorf("creating caldav client: %w", err)
	}

	principal, err := client.FindCurrentUserPrincipal(ctx)
	if err != nil {
		return nil, fmt.Errorf("finding principal: %w", err)
	}

	homeSet, err := client.FindCalendarHomeSet(ctx, principal)
	if err != nil {
		return nil, fmt.Errorf("finding calendar home set: %w", err)
	}

	calendars, err := client.FindCalendars(ctx, homeSet)
	if err != nil {
		return nil, fmt.Errorf("finding calendars: %w", err)
	}

	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	endOfRange := time.Date(now.Year(), now.Month(), now.Day()+days, 0, 0, 0, 0, now.Location())

	var allMeetings []Meeting
	for _, cal := range calendars {
		if !supportsEvents(cal) {
			continue
		}

		c.calLabel = calendarLabel(cal, c.name)

		query := &caldav.CalendarQuery{
			CompRequest: caldav.CalendarCompRequest{
				Name: "VCALENDAR",
				Comps: []caldav.CalendarCompRequest{{
					Name: "VEVENT",
					Props: []string{
						"SUMMARY", "DTSTART", "DTEND", "DURATION",
						"LOCATION", "DESCRIPTION", "STATUS", "UID",
						"ORGANIZER", "ATTENDEE", "RRULE", "RECURRENCE-ID",
					},
				}},
			},
			CompFilter: caldav.CompFilter{
				Name: "VCALENDAR",
				Comps: []caldav.CompFilter{{
					Name:  "VEVENT",
					Start: startOfDay.UTC(),
					End:   endOfRange.UTC(),
				}},
			},
		}

		objects, err := client.QueryCalendar(ctx, cal.Path, query)
		if err != nil {
			continue
		}

		for _, obj := range objects {
			meetings := c.parseCalendarObject(obj, startOfDay, endOfRange)
			allMeetings = append(allMeetings, meetings...)
		}
	}

	return allMeetings, nil
}

func supportsEvents(cal caldav.Calendar) bool {
	if len(cal.SupportedComponentSet) == 0 {
		return true
	}
	for _, comp := range cal.SupportedComponentSet {
		if comp == "VEVENT" {
			return true
		}
	}
	return false
}

// sourceLabel returns the label to attach to meetings: the current calendar's
// display name if known, otherwise the configured source name.
func (c *CalDAVSource) sourceLabel() string {
	if c.calLabel != "" {
		return c.calLabel
	}
	return c.name
}

// calendarLabel derives a human-readable label for a calendar, falling back to
// the last path segment and then to the configured source name.
func calendarLabel(cal caldav.Calendar, fallback string) string {
	if cal.Name != "" {
		return cal.Name
	}
	path := strings.TrimRight(cal.Path, "/")
	if i := strings.LastIndex(path, "/"); i >= 0 && i+1 < len(path) {
		return path[i+1:]
	}
	return fallback
}

func (c *CalDAVSource) parseCalendarObject(obj caldav.CalendarObject, queryStart, queryEnd time.Time) []Meeting {
	if obj.Data == nil {
		return nil
	}

	// Collect the RECURRENCE-ID of every instance override in this resource.
	// The recurring master must not also expand those instances, or they'd be
	// counted twice (once from the RRULE, once from the override).
	overrides := make(map[int64]bool)
	for _, event := range obj.Data.Events() {
		if event.Props.Get("RECURRENCE-ID") == nil {
			continue
		}
		if t := parseICalTimeProp(event.Props, "RECURRENCE-ID"); !t.IsZero() {
			overrides[t.Unix()] = true
		}
	}

	var meetings []Meeting
	for _, event := range obj.Data.Events() {
		ms := c.parseEvent(event, queryStart, queryEnd, overrides)
		meetings = append(meetings, ms...)
	}
	return meetings
}

// parseEvent handles three cases:
// 1. Event with RECURRENCE-ID: it's a modified instance — use DTSTART directly
// 2. Event with RRULE: expand occurrences within the query range
// 3. Plain event: use DTSTART as-is
func (c *CalDAVSource) parseEvent(event ical.Event, queryStart, queryEnd time.Time, overrides map[int64]bool) []Meeting {
	hasRRule := event.Props.Get("RRULE") != nil
	hasRecurrenceID := event.Props.Get("RECURRENCE-ID") != nil

	// Parse the base event data (shared across all occurrences)
	base := c.buildMeetingBase(event)

	// Parse DTSTART into a time.Time for calculations
	dtstart := parseICalTimeProp(event.Props, "DTSTART")
	isAllDay := isDateOnly(event.Props, "DTSTART")

	// Calculate event duration
	dur := c.getEventDuration(event, isAllDay)

	if hasRecurrenceID {
		// Case 1: Modified instance — DTSTART is the actual occurrence date
		// Filter: must overlap with query range
		eventEnd := dtstart.Add(dur)
		if eventEnd.Before(queryStart) || dtstart.After(queryEnd) {
			return nil
		}
		base.Start = formatTime(dtstart, isAllDay)
		base.End = formatTime(dtstart.Add(dur), isAllDay)
		return []Meeting{base}
	}

	if hasRRule {
		// Case 2: Recurring event — expand occurrences within query range
		return c.expandRecurring(event, base, dtstart, dur, isAllDay, queryStart, queryEnd, overrides)
	}

	// Case 3: One-off event
	eventEnd := dtstart.Add(dur)
	if eventEnd.Before(queryStart) || dtstart.After(queryEnd) {
		return nil
	}
	base.Start = formatTime(dtstart, isAllDay)
	base.End = formatTime(dtstart.Add(dur), isAllDay)
	return []Meeting{base}
}

// expandRecurring uses rrule-go to compute occurrences within the query range.
func (c *CalDAVSource) expandRecurring(event ical.Event, base Meeting, dtstart time.Time, dur time.Duration, isAllDay bool, queryStart, queryEnd time.Time, overrides map[int64]bool) []Meeting {
	rruleProp := event.Props.Get("RRULE")
	if rruleProp == nil {
		return nil
	}

	// rrule-go works best with an ROption struct so we control the DTSTART timezone
	opt, err := rrule.StrToROption(fmt.Sprintf("RRULE:%s", rruleProp.Value))
	if err != nil {
		// Can't parse — fall back
		if !overrides[dtstart.Unix()] && !dtstart.Before(queryStart) && dtstart.Before(queryEnd) {
			base.Start = formatTime(dtstart, isAllDay)
			base.End = formatTime(dtstart.Add(dur), isAllDay)
			return []Meeting{base}
		}
		return nil
	}

	// Set DTSTART in the original timezone so DST transitions are handled correctly
	opt.Dtstart = dtstart
	rule, err := rrule.NewRRule(*opt)
	if err != nil {
		if !overrides[dtstart.Unix()] && !dtstart.Before(queryStart) && dtstart.Before(queryEnd) {
			base.Start = formatTime(dtstart, isAllDay)
			base.End = formatTime(dtstart.Add(dur), isAllDay)
			return []Meeting{base}
		}
		return nil
	}

	// Get occurrences within the query range
	// Use a small buffer before queryStart to catch events that straddle the boundary
	occurrences := rule.Between(queryStart.Add(-1*time.Second), queryEnd, true)

	var meetings []Meeting
	for _, occ := range occurrences {
		// Skip instances that have an explicit override; those are emitted
		// separately when their RECURRENCE-ID event is parsed.
		if overrides[occ.Unix()] {
			continue
		}
		m := base // copy
		m.Start = formatTime(occ, isAllDay)
		m.End = formatTime(occ.Add(dur), isAllDay)
		meetings = append(meetings, m)
	}

	return meetings
}

// buildMeetingBase extracts all non-time fields from an event into a Meeting.
func (c *CalDAVSource) buildMeetingBase(event ical.Event) Meeting {
	summary := propText(event.Props, "SUMMARY")
	if summary == "" {
		summary = "(no title)"
	}

	location := propText(event.Props, "LOCATION")
	description := propText(event.Props, "DESCRIPTION")
	status := strings.ToLower(propText(event.Props, "STATUS"))

	var orgEmail, orgName string
	if orgProp := event.Props.Get("ORGANIZER"); orgProp != nil {
		orgEmail = strings.TrimPrefix(orgProp.Value, "mailto:")
		orgEmail = strings.TrimPrefix(orgEmail, "MAILTO:")
		if cn := orgProp.Params.Get("CN"); cn != "" {
			orgName = cn
		}
	}

	var attendees []MeetingAttendee
	for _, prop := range event.Props.Values("ATTENDEE") {
		email := strings.TrimPrefix(prop.Value, "mailto:")
		email = strings.TrimPrefix(email, "MAILTO:")

		name := prop.Params.Get("CN")
		partstat := strings.ToLower(prop.Params.Get("PARTSTAT"))
		role := prop.Params.Get("ROLE")

		responseStatus := "needsAction"
		switch partstat {
		case "accepted":
			responseStatus = "accepted"
		case "declined":
			responseStatus = "declined"
		case "tentative":
			responseStatus = "tentative"
		}

		attendees = append(attendees, MeetingAttendee{
			Email:          email,
			Name:           name,
			ResponseStatus: responseStatus,
			Organizer:      strings.EqualFold(email, orgEmail),
			Self:           strings.EqualFold(email, c.username),
			Optional:       strings.EqualFold(role, "OPT-PARTICIPANT"),
		})
	}

	return Meeting{
		Source:         c.sourceLabel(),
		Summary:        summary,
		Description:    description,
		Location:       location,
		Status:         status,
		OrganizerEmail: orgEmail,
		OrganizerName:  orgName,
		Attendees:      attendees,
	}
}

// getEventDuration returns the event duration from DURATION prop, or from DTEND - DTSTART.
func (c *CalDAVSource) getEventDuration(event ical.Event, isAllDay bool) time.Duration {
	// Try DURATION property first
	if durProp := event.Props.Get("DURATION"); durProp != nil {
		if d, err := parseICalDuration(durProp.Value); err == nil {
			return d
		}
	}

	// Fall back to DTEND - DTSTART
	dtstart := parseICalTimeProp(event.Props, "DTSTART")
	dtend := parseICalTimeProp(event.Props, "DTEND")
	if !dtend.IsZero() && !dtstart.IsZero() {
		return dtend.Sub(dtstart)
	}

	// Default: 1 day for all-day events, 1 hour for timed events
	if isAllDay {
		return 24 * time.Hour
	}
	return time.Hour
}

// parseICalDuration parses an iCalendar DURATION value like "PT1H30M", "P1D", etc.
func parseICalDuration(s string) (time.Duration, error) {
	neg := false
	if strings.HasPrefix(s, "-") {
		neg = true
		s = s[1:]
	}
	if !strings.HasPrefix(s, "P") {
		return 0, fmt.Errorf("invalid duration: %s", s)
	}
	s = s[1:]

	var d time.Duration
	inTime := false
	for len(s) > 0 {
		if s[0] == 'T' {
			inTime = true
			s = s[1:]
			continue
		}
		// Read number
		i := 0
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		if i == 0 || i >= len(s) {
			break
		}
		n := 0
		for _, c := range s[:i] {
			n = n*10 + int(c-'0')
		}
		unit := s[i]
		s = s[i+1:]

		switch {
		case unit == 'D':
			d += time.Duration(n) * 24 * time.Hour
		case unit == 'H' && inTime:
			d += time.Duration(n) * time.Hour
		case unit == 'M' && inTime:
			d += time.Duration(n) * time.Minute
		case unit == 'S' && inTime:
			d += time.Duration(n) * time.Second
		case unit == 'W':
			d += time.Duration(n) * 7 * 24 * time.Hour
		}
	}
	if neg {
		d = -d
	}
	return d, nil
}

// formatTime formats a time.Time as RFC3339 (timed) or "2006-01-02" (all-day).
func formatTime(t time.Time, allDay bool) string {
	if allDay {
		return t.Format("2006-01-02")
	}
	return t.Format(time.RFC3339)
}

// isDateOnly checks if a DTSTART/DTEND property has VALUE=DATE.
func isDateOnly(props ical.Props, name string) bool {
	prop := props.Get(name)
	if prop == nil {
		return false
	}
	if prop.Params.Get("VALUE") == "DATE" {
		return true
	}
	// Also detect by length: "20260413" (8 chars) vs "20260413T100000" (15+ chars)
	return len(prop.Value) == 8
}

// parseICalTimeProp parses a DTSTART/DTEND/RECURRENCE-ID property into a time.Time.
func parseICalTimeProp(props ical.Props, name string) time.Time {
	prop := props.Get(name)
	if prop == nil {
		return time.Time{}
	}

	tzid := prop.Params.Get("TZID")
	var loc *time.Location
	if tzid != "" {
		loc, _ = time.LoadLocation(tzid)
	}

	// VALUE=DATE (all-day)
	if isDateOnly(props, name) {
		t, err := time.Parse("20060102", prop.Value)
		if err == nil {
			return t
		}
		return time.Time{}
	}

	// UTC datetime (ends with Z)
	if t, err := time.Parse("20060102T150405Z", prop.Value); err == nil {
		if loc != nil {
			return t.In(loc)
		}
		return t
	}

	// Local datetime with optional TZID
	if t, err := time.Parse("20060102T150405", prop.Value); err == nil {
		if loc != nil {
			return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, loc)
		}
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, time.Local)
	}

	return time.Time{}
}

// propText returns the text value of a named iCalendar property.
func propText(props ical.Props, name string) string {
	if p := props.Get(name); p != nil {
		return p.Value
	}
	return ""
}
