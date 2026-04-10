package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ANSI color codes
const (
	reset   = "\033[0m"
	bold    = "\033[1m"
	dim     = "\033[2m"
	red     = "\033[31m"
	green   = "\033[32m"
	yellow  = "\033[33m"
	blue    = "\033[34m"
	magenta = "\033[35m"
	cyan    = "\033[36m"
	white   = "\033[37m"
)

type Config struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RefreshToken string `json:"refresh_token"`
	TokenURI     string `json:"token_uri"`
}

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

type EventTime struct {
	DateTime string `json:"dateTime,omitempty"`
	Date     string `json:"date,omitempty"`
}

type Attendee struct {
	Email          string `json:"email"`
	DisplayName    string `json:"displayName,omitempty"`
	ResponseStatus string `json:"responseStatus,omitempty"`
	Organizer      bool   `json:"organizer,omitempty"`
	Self           bool   `json:"self,omitempty"`
	Optional       bool   `json:"optional,omitempty"`
}

type Organizer struct {
	Email       string `json:"email"`
	DisplayName string `json:"displayName,omitempty"`
	Self        bool   `json:"self,omitempty"`
}

type CalendarEvent struct {
	Summary     string     `json:"summary"`
	Description string     `json:"description,omitempty"`
	Start       EventTime  `json:"start"`
	End         EventTime  `json:"end"`
	Location    string     `json:"location,omitempty"`
	Status      string     `json:"status,omitempty"`
	Organizer   Organizer  `json:"organizer,omitempty"`
	Creator     Organizer  `json:"creator,omitempty"`
	Attendees   []Attendee `json:"attendees,omitempty"`
	HangoutLink string     `json:"hangoutLink,omitempty"`
	HTMLLink    string     `json:"htmlLink,omitempty"`
}

type EventsResponse struct {
	Items []CalendarEvent `json:"items"`
}

type MeetingAttendee struct {
	Email          string `json:"email"`
	Name           string `json:"name,omitempty"`
	ResponseStatus string `json:"response_status,omitempty"`
	Organizer      bool   `json:"organizer,omitempty"`
	Self           bool   `json:"self,omitempty"`
	Optional       bool   `json:"optional,omitempty"`
}

type Meeting struct {
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
}

func loadConfig() (*Config, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	configPath := filepath.Join(filepath.Dir(exe), "config.json")

	// Fall back to current directory if not found next to binary
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		configPath = "config.json"
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return &cfg, nil
}

func getAccessToken(cfg *Config) (string, error) {
	resp, err := http.PostForm(cfg.TokenURI, url.Values{
		"client_id":     {cfg.ClientID},
		"client_secret": {cfg.ClientSecret},
		"refresh_token": {cfg.RefreshToken},
		"grant_type":    {"refresh_token"},
	})
	if err != nil {
		return "", fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token refresh failed (%d): %s", resp.StatusCode, body)
	}

	var tok TokenResponse
	if err := json.Unmarshal(body, &tok); err != nil {
		return "", fmt.Errorf("parsing token response: %w", err)
	}
	return tok.AccessToken, nil
}

func fetchEvents(accessToken string, days int) ([]CalendarEvent, error) {
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	endOfRange := time.Date(now.Year(), now.Month(), now.Day()+days, 0, 0, 0, 0, now.Location())

	params := url.Values{
		"timeMin":      {startOfDay.Format(time.RFC3339)},
		"timeMax":      {endOfRange.Format(time.RFC3339)},
		"singleEvents": {"true"},
		"orderBy":      {"startTime"},
	}

	req, err := http.NewRequest("GET",
		"https://www.googleapis.com/calendar/v3/calendars/primary/events?"+params.Encode(),
		nil,
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching events: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading events response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("events request failed (%d): %s", resp.StatusCode, body)
	}

	var eventsResp EventsResponse
	if err := json.Unmarshal(body, &eventsResp); err != nil {
		return nil, fmt.Errorf("parsing events: %w", err)
	}
	return eventsResp.Items, nil
}

func formatEvents(events []CalendarEvent) []Meeting {
	meetings := make([]Meeting, 0, len(events))
	for _, e := range events {
		start := e.Start.DateTime
		if start == "" {
			start = e.Start.Date
		}
		end := e.End.DateTime
		if end == "" {
			end = e.End.Date
		}

		attendees := make([]MeetingAttendee, 0, len(e.Attendees))
		for _, a := range e.Attendees {
			attendees = append(attendees, MeetingAttendee{
				Email:          a.Email,
				Name:           a.DisplayName,
				ResponseStatus: a.ResponseStatus,
				Organizer:      a.Organizer,
				Self:           a.Self,
				Optional:       a.Optional,
			})
		}

		summary := e.Summary
		if summary == "" {
			summary = "(no title)"
		}

		meetings = append(meetings, Meeting{
			Summary:        summary,
			Description:    e.Description,
			Start:          start,
			End:            end,
			Location:       e.Location,
			Status:         e.Status,
			OrganizerEmail: e.Organizer.Email,
			OrganizerName:  e.Organizer.DisplayName,
			Attendees:      attendees,
			HangoutLink:    e.HangoutLink,
			HTMLLink:       e.HTMLLink,
		})
	}
	return meetings
}

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

func printText(meetings []Meeting) {
	now := time.Now()

	if len(meetings) == 0 {
		fmt.Printf("%s%sNo meetings found.%s\n", dim, white, reset)
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
			fmt.Printf("\n%s%s%s%s\n", bold, cyan, label, reset)
			fmt.Printf("%s%s%s\n", dim, strings.Repeat("─", len(label)+2), reset)
		}

		for _, m := range dayMeetings {
			startTime, err := time.Parse(time.RFC3339, m.Start)
			isAllDay := err != nil
			endTime, _ := time.Parse(time.RFC3339, m.End)

			// Determine color based on timing
			var statusColor string
			var indicator string
			if isAllDay {
				statusColor = magenta
				indicator = "◆"
			} else if now.After(endTime) {
				statusColor = dim
				indicator = "✓"
			} else if now.After(startTime) && now.Before(endTime) {
				statusColor = green
				indicator = "▶"
			} else {
				statusColor = yellow
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

			// Summary line
			fmt.Printf("  %s%s %s%-13s%s %s%s%s%s\n",
				statusColor, indicator, dim, timeStr, reset,
				statusColor, bold, m.Summary, reset,
			)

			// Location
			if m.Location != "" {
				fmt.Printf("    %s%s📍 %s%s\n", dim, white, m.Location, reset)
			}

			// Meet link
			if m.HangoutLink != "" {
				fmt.Printf("    %s%s🔗 %s%s\n", dim, blue, m.HangoutLink, reset)
			}
		}
	}

	// Legend
	fmt.Printf("\n%s%s  ✓ past  ▶ now  ○ upcoming  ◆ all day%s\n\n", dim, white, reset)
}

func main() {
	days := flag.Int("days", 1, "number of days to fetch (starting today)")
	future := flag.Bool("future", false, "only show meetings that haven't ended yet")
	text := flag.Bool("text", false, "show colored text output instead of JSON")
	tui := flag.Bool("tui", false, "interactive TUI with meeting list and detail pane")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: qcal [flags]\n\n")
		fmt.Fprintf(os.Stderr, "Fetch Google Calendar meetings and print them as JSON.\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	accessToken, err := getAccessToken(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	events, err := fetchEvents(accessToken, *days)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	meetings := formatEvents(events)

	if *future {
		meetings = filterFuture(meetings)
	}

	if *tui {
		if err := runTUI(meetings); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if *text {
		printText(meetings)
		return
	}

	output, err := json.MarshalIndent(meetings, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(string(output))
}
