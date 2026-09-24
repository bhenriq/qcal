package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Google API response types (not exported — internal to this source).

type googleTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

type googleEventTime struct {
	DateTime string `json:"dateTime,omitempty"`
	Date     string `json:"date,omitempty"`
}

type googleAttendee struct {
	Email          string `json:"email"`
	DisplayName    string `json:"displayName,omitempty"`
	ResponseStatus string `json:"responseStatus,omitempty"`
	Organizer      bool   `json:"organizer,omitempty"`
	Self           bool   `json:"self,omitempty"`
	Optional       bool   `json:"optional,omitempty"`
}

type googleOrganizer struct {
	Email       string `json:"email"`
	DisplayName string `json:"displayName,omitempty"`
	Self        bool   `json:"self,omitempty"`
}

type googleEvent struct {
	Summary     string           `json:"summary"`
	Description string           `json:"description,omitempty"`
	Start       googleEventTime  `json:"start"`
	End         googleEventTime  `json:"end"`
	Location    string           `json:"location,omitempty"`
	Status      string           `json:"status,omitempty"`
	Organizer   googleOrganizer  `json:"organizer,omitempty"`
	Creator     googleOrganizer  `json:"creator,omitempty"`
	Attendees   []googleAttendee `json:"attendees,omitempty"`
	HangoutLink string           `json:"hangoutLink,omitempty"`
	HTMLLink    string           `json:"htmlLink,omitempty"`
}

type googleEventsResponse struct {
	Items []googleEvent `json:"items"`
}

// GoogleSource implements CalendarSource for Google Calendar REST API.
type GoogleSource struct {
	name         string
	clientID     string
	clientSecret string
	refreshToken string
	tokenURI     string
	passCmd      string
}

func NewGoogleSource(cfg SourceConfig) *GoogleSource {
	return &GoogleSource{
		name:         cfg.Name,
		clientID:     cfg.ClientID,
		clientSecret: cfg.ClientSecret,
		refreshToken: cfg.RefreshToken,
		tokenURI:     cfg.TokenURI,
		passCmd:      cfg.PassCmd,
	}
}

func (g *GoogleSource) Name() string { return g.name }

func (g *GoogleSource) FetchEvents(days int) ([]Meeting, error) {
	if g.refreshToken == "" && g.passCmd != "" {
		rt, err := runPassCmd(g.passCmd)
		if err != nil {
			return nil, err
		}
		g.refreshToken = rt
	}

	accessToken, err := g.getAccessToken()
	if err != nil {
		return nil, err
	}
	events, err := g.fetchEvents(accessToken, days)
	if err != nil {
		return nil, err
	}
	return g.formatEvents(events), nil
}

func (g *GoogleSource) getAccessToken() (string, error) {
	resp, err := http.PostForm(g.tokenURI, url.Values{
		"client_id":     {g.clientID},
		"client_secret": {g.clientSecret},
		"refresh_token": {g.refreshToken},
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

	var tok googleTokenResponse
	if err := json.Unmarshal(body, &tok); err != nil {
		return "", fmt.Errorf("parsing token response: %w", err)
	}
	return tok.AccessToken, nil
}

func (g *GoogleSource) fetchEvents(accessToken string, days int) ([]googleEvent, error) {
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

	var eventsResp googleEventsResponse
	if err := json.Unmarshal(body, &eventsResp); err != nil {
		return nil, fmt.Errorf("parsing events: %w", err)
	}
	return eventsResp.Items, nil
}

func (g *GoogleSource) formatEvents(events []googleEvent) []Meeting {
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
			Source:         g.name,
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
