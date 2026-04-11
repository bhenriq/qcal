package main

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Pane focus
const (
	focusList   = 0
	focusDetail = 1
)

// Styles
var (
	selectedStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("62"))

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("212"))

	sectionStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("117"))

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	acceptedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("114"))

	declinedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("210"))

	tentativeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("222"))

	needsActionStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("245"))

	nowStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("114"))

	upcomingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("222"))

	pastStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	allDayStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("170"))

	dateHeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("81"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	focusedBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder(), false, true, false, false).
				BorderForeground(lipgloss.Color("117"))

	unfocusedBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder(), false, true, false, false).
				BorderForeground(lipgloss.Color("240"))
)

var htmlTagRe = regexp.MustCompile(`<[^>]*>`)

type model struct {
	meetings   []Meeting
	cursor     int
	focus      int // focusList or focusDetail
	listScroll int // scroll offset for list pane
	detScroll  int // scroll offset for detail pane
	width      int
	height     int
}

func newModel(meetings []Meeting) model {
	return model{
		meetings: meetings,
		focus:    focusList,
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "left", "h":
			m.focus = focusList
		case "right", "l":
			m.focus = focusDetail
		case "up", "k":
			if m.focus == focusList {
				if m.cursor > 0 {
					m.cursor--
					m.detScroll = 0
				}
				// Adjust list scroll to keep cursor visible
				if m.cursor < m.listScroll {
					m.listScroll = m.cursor
				}
			} else {
				if m.detScroll > 0 {
					m.detScroll--
				}
			}
		case "down", "j":
			if m.focus == focusList {
				if m.cursor < len(m.meetings)-1 {
					m.cursor++
					m.detScroll = 0
				}
				// Adjust list scroll to keep cursor visible
				visibleLines := m.contentHeight()
				if m.cursor >= m.listScroll+visibleLines {
					m.listScroll = m.cursor - visibleLines + 1
				}
			} else {
				m.detScroll++
			}
		case "g":
			if m.focus == focusList {
				m.cursor = 0
				m.listScroll = 0
				m.detScroll = 0
			} else {
				m.detScroll = 0
			}
		case "G":
			if m.focus == focusList {
				m.cursor = len(m.meetings) - 1
				m.detScroll = 0
				visibleLines := m.contentHeight()
				if m.cursor >= visibleLines {
					m.listScroll = m.cursor - visibleLines + 1
				}
			}
			// Don't handle G for detail — no max known here
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}
	return m, nil
}

func (m model) contentHeight() int {
	return m.height - 4
}

func (m model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	listWidth := m.width*2/5 - 1
	detailWidth := m.width - listWidth - 3
	contentH := m.contentHeight()

	list := m.renderList(listWidth, contentH)
	detail := m.renderDetail(detailWidth, contentH)

	// Apply border style based on focus
	var listBorder lipgloss.Style
	if m.focus == focusList {
		listBorder = focusedBorderStyle
	} else {
		listBorder = unfocusedBorderStyle
	}

	listPane := listBorder.
		Width(listWidth).
		Height(m.height - 2).
		Render(list)

	detailPane := lipgloss.NewStyle().
		Width(detailWidth).
		Height(m.height - 2).
		Render(detail)

	main := lipgloss.JoinHorizontal(lipgloss.Top, listPane, detailPane)

	// Help bar
	var focusHint string
	if m.focus == focusList {
		focusHint = "list"
	} else {
		focusHint = "detail"
	}
	help := helpStyle.Render(
		fmt.Sprintf("  ←/→ switch pane (%s) • ↑/↓ navigate • g/G top/bottom • q quit", focusHint),
	)

	return main + "\n" + help
}

func (m model) renderList(width, maxHeight int) string {
	if len(m.meetings) == 0 {
		return dimStyle.Render("No meetings found.")
	}

	now := time.Now()

	// Build all lines first
	type listLine struct {
		text     string
		isHeader bool
	}
	var allLines []listLine
	currentDate := ""
	meetingIdx := -1

	for i, mtg := range m.meetings {
		dateStr := mtg.Start
		if t, err := time.Parse(time.RFC3339, mtg.Start); err == nil {
			dateStr = t.Format("2006-01-02")
		}
		if dateStr != currentDate {
			currentDate = dateStr
			if d, err := time.Parse("2006-01-02", dateStr); err == nil {
				label := d.Format("Mon, Jan 2")
				if d.Format("2006-01-02") == now.Format("2006-01-02") {
					label += " (today)"
				}
				if len(allLines) > 0 {
					allLines = append(allLines, listLine{text: "", isHeader: true})
				}
				allLines = append(allLines, listLine{
					text:     dateHeaderStyle.Render(label),
					isHeader: true,
				})
			}
		}

		meetingIdx = i
		startTime, err := time.Parse(time.RFC3339, mtg.Start)
		isAllDay := err != nil
		endTime, _ := time.Parse(time.RFC3339, mtg.End)

		var indicator string
		var style lipgloss.Style
		if isAllDay {
			indicator = "◆"
			style = allDayStyle
		} else if now.After(endTime) {
			indicator = "✓"
			style = pastStyle
		} else if now.After(startTime) && now.Before(endTime) {
			indicator = "▶"
			style = nowStyle
		} else {
			indicator = "○"
			style = upcomingStyle
		}

		timeStr := "all day"
		if !isAllDay {
			timeStr = startTime.Format("15:04")
		}

		maxSummaryW := width - 14
		summary := mtg.Summary
		if maxSummaryW > 0 && len(summary) > maxSummaryW {
			summary = summary[:maxSummaryW-1] + "…"
		}

		line := fmt.Sprintf(" %s %-7s %s", indicator, timeStr, summary)

		if meetingIdx == m.cursor {
			line = selectedStyle.Width(width - 2).Render(line)
		} else {
			line = style.Render(line)
		}

		allLines = append(allLines, listLine{text: line, isHeader: false})
	}

	// Apply scroll and truncate to maxHeight
	start := m.listScroll
	if start > len(allLines) {
		start = len(allLines)
	}
	visible := allLines[start:]
	if len(visible) > maxHeight {
		visible = visible[:maxHeight]
	}

	var b strings.Builder
	for _, l := range visible {
		b.WriteString(l.text + "\n")
	}
	return b.String()
}

func (m model) renderDetail(width, maxHeight int) string {
	if len(m.meetings) == 0 {
		return ""
	}

	mtg := m.meetings[m.cursor]
	now := time.Now()
	wrapWidth := width - 6
	if wrapWidth < 20 {
		wrapWidth = 20
	}

	var sections []string

	// Title
	sections = append(sections, titleStyle.Render(wrapText(mtg.Summary, wrapWidth)))

	// Time
	startTime, err := time.Parse(time.RFC3339, mtg.Start)
	isAllDay := err != nil
	if isAllDay {
		sections = append(sections, sectionStyle.Render("All day"))
	} else {
		endTime, _ := time.Parse(time.RFC3339, mtg.End)
		dur := endTime.Sub(startTime)
		durStr := formatDuration(dur)
		timeStr := fmt.Sprintf("%s  -  %s  (%s)",
			startTime.Format("3:04 PM"),
			endTime.Format("3:04 PM"),
			durStr,
		)
		if now.After(startTime) && now.Before(endTime) {
			remaining := endTime.Sub(now)
			timeStr += fmt.Sprintf("  •  %s left", formatDuration(remaining))
		}
		sections = append(sections, sectionStyle.Render("Time")+"\n"+timeStr)
	}

	// Status
	if !isAllDay {
		endTime, _ := time.Parse(time.RFC3339, mtg.End)
		var statusLine string
		if now.After(endTime) {
			statusLine = pastStyle.Render("✓ Ended")
		} else if now.After(startTime) && now.Before(endTime) {
			statusLine = nowStyle.Render("▶ In progress")
		} else {
			until := startTime.Sub(now)
			statusLine = upcomingStyle.Render(fmt.Sprintf("○ Starts in %s", formatDuration(until)))
		}
		sections = append(sections, statusLine)
	}

	// Location
	if mtg.Location != "" {
		sections = append(sections, sectionStyle.Render("Location")+"\n"+wrapText(mtg.Location, wrapWidth))
	}

	// Meet link
	if mtg.HangoutLink != "" {
		sections = append(sections, sectionStyle.Render("Meeting Link")+"\n"+mtg.HangoutLink)
	}

	// Organizer
	if mtg.OrganizerEmail != "" {
		org := mtg.OrganizerEmail
		if mtg.OrganizerName != "" {
			org = mtg.OrganizerName + "\n" + mtg.OrganizerEmail
		}
		sections = append(sections, sectionStyle.Render("Organizer")+"\n"+wrapText(org, wrapWidth))
	}

	// Attendees
	if len(mtg.Attendees) > 0 {
		sections = append(sections, renderAttendees(mtg.Attendees, wrapWidth))
	}

	// Description (strip HTML)
	if mtg.Description != "" {
		desc := stripHTML(mtg.Description)
		desc = strings.TrimSpace(desc)
		if desc != "" {
			sections = append(sections, sectionStyle.Render("Description")+"\n"+dimStyle.Render(wrapText(desc, wrapWidth)))
		}
	}

	content := strings.Join(sections, "\n\n")

	// Split into lines, apply scroll, truncate
	lines := strings.Split(content, "\n")

	// Clamp scroll
	if m.detScroll > len(lines)-1 {
		// handled in Update but be safe
	}
	if m.detScroll > 0 && m.detScroll < len(lines) {
		lines = lines[m.detScroll:]
	}
	if len(lines) > maxHeight {
		lines = lines[:maxHeight]
	}

	// Add consistent left padding
	pad := "   "
	for i, l := range lines {
		lines[i] = pad + l
	}

	return strings.Join(lines, "\n")
}

func renderAttendees(attendees []MeetingAttendee, wrapWidth int) string {
	var accepted, declined, tentative, noReply []string
	for _, a := range attendees {
		name := a.Email
		if a.Name != "" {
			name = a.Name
		}
		if a.Self {
			name += " (you)"
		}
		if a.Optional {
			name += " [optional]"
		}

		switch a.ResponseStatus {
		case "accepted":
			accepted = append(accepted, name)
		case "declined":
			declined = append(declined, name)
		case "tentative":
			tentative = append(tentative, name)
		default:
			noReply = append(noReply, name)
		}
	}

	var lines []string
	header := fmt.Sprintf("Attendees (%d)", len(attendees))
	lines = append(lines, sectionStyle.Render(header))

	if len(accepted) > 0 {
		lines = append(lines, acceptedStyle.Render(fmt.Sprintf("  ✓ Accepted (%d)", len(accepted))))
		for _, n := range accepted {
			lines = append(lines, "    "+wrapText(n, wrapWidth-4))
		}
	}
	if len(tentative) > 0 {
		lines = append(lines, tentativeStyle.Render(fmt.Sprintf("  ? Tentative (%d)", len(tentative))))
		for _, n := range tentative {
			lines = append(lines, "    "+wrapText(n, wrapWidth-4))
		}
	}
	if len(declined) > 0 {
		lines = append(lines, declinedStyle.Render(fmt.Sprintf("  ✗ Declined (%d)", len(declined))))
		for _, n := range declined {
			lines = append(lines, "    "+wrapText(n, wrapWidth-4))
		}
	}
	if len(noReply) > 0 {
		lines = append(lines, needsActionStyle.Render(fmt.Sprintf("  … No reply (%d)", len(noReply))))
		for _, n := range noReply {
			lines = append(lines, "    "+wrapText(n, wrapWidth-4))
		}
	}

	return strings.Join(lines, "\n")
}

// stripHTML removes HTML tags and decodes common entities.
func stripHTML(s string) string {
	// Replace <br> variants with newlines
	s = regexp.MustCompile(`<br\s*/?>|</p>|</div>|</li>`).ReplaceAllString(s, "\n")
	// Strip remaining tags
	s = htmlTagRe.ReplaceAllString(s, "")
	// Decode common HTML entities
	r := strings.NewReplacer(
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", `"`,
		"&#39;", "'",
		"&nbsp;", " ",
	)
	s = r.Replace(s)
	// Collapse multiple blank lines
	s = regexp.MustCompile(`\n{3,}`).ReplaceAllString(s, "\n\n")
	return s
}

// wrapText hard-wraps text to fit within maxWidth columns.
func wrapText(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return s
	}
	var result strings.Builder
	for _, inputLine := range strings.Split(s, "\n") {
		if len(inputLine) <= maxWidth {
			if result.Len() > 0 {
				result.WriteString("\n")
			}
			result.WriteString(inputLine)
			continue
		}
		for len(inputLine) > 0 {
			cut := maxWidth
			if cut > len(inputLine) {
				cut = len(inputLine)
			}
			// Try to break at a space
			if cut < len(inputLine) {
				if idx := strings.LastIndex(inputLine[:cut], " "); idx > maxWidth/3 {
					cut = idx + 1
				}
			}
			if result.Len() > 0 {
				result.WriteString("\n")
			}
			result.WriteString(strings.TrimRight(inputLine[:cut], " "))
			inputLine = inputLine[cut:]
		}
	}
	return result.String()
}

func formatDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 && m > 0 {
		return fmt.Sprintf("%dh%dm", h, m)
	}
	if h > 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dm", m)
}

func runTUI(meetings []Meeting) error {
	p := tea.NewProgram(
		newModel(meetings),
		tea.WithAltScreen(),
	)
	_, err := p.Run()
	return err
}
