# qcal

**qcal** fetches calendar events from multiple sources (Google Calendar, CalDAV) and displays them in your terminal — as JSON, colored text, or an interactive TUI.

## Features

- Google Calendar API support (OAuth2 refresh token)
- CalDAV support (Fastmail, iCloud, Nextcloud, etc.)
- Recurring event expansion (RRULE)
- Automatic conflict detection between overlapping meetings
- Three output modes: JSON, colored text, interactive TUI
- Attendee list with response status (accepted / declined / tentative / no reply)
- Source color-coding when using multiple calendar sources
- All-day event handling

## Installation

```bash
go install qcal@latest
```

Or clone and build:

```bash
git clone <repo-url> && cd qcal
go build -o qcal .
```

## Configuration

qcal reads `~/.config/qcal/config.json` (or `$XDG_CONFIG_HOME/qcal/config.json`), falling back to `./config.json` in the current directory.

Copy the example config and edit it:

```bash
cp config.example.json ~/.config/qcal/config.json
```

### Google Calendar source

```json
{
  "sources": [
    {
      "name": "work",
      "type": "google",
      "color": "63",
      "client_id": "YOUR_CLIENT_ID",
      "client_secret": "YOUR_CLIENT_SECRET",
      "refresh_token": "YOUR_REFRESH_TOKEN",
      "token_uri": "https://oauth2.googleapis.com/token"
    }
  ]
}
```

### CalDAV source (Fastmail, iCloud, Nextcloud, etc.)

```json
{
  "sources": [
    {
      "name": "personal",
      "type": "caldav",
      "color": "170",
      "url": "https://caldav.fastmail.com/",
      "username": "you@fastmail.com",
      "password": "APP_PASSWORD"
    }
  ]
}
```

### Secrets via a password command

Instead of writing the password (or Google refresh token) into the config, set
`passcmd` to a shell command whose stdout is the secret. It is only consulted
when the literal `password` / `refresh_token` field is empty.

macOS Keychain example — store the Fastmail app password once (it prompts, so
the secret never lands in your shell history or config):

```bash
security add-generic-password -a "you@fastmail.com" -s "qcal-fastmail" -U -w
```

```json
{
  "name": "personal",
  "type": "caldav",
  "url": "https://caldav.fastmail.com/",
  "username": "you@fastmail.com",
  "passcmd": "security find-generic-password -a you@fastmail.com -s qcal-fastmail -w"
}
```

`passcmd` works with any password manager (`pass`, `op`, `secret-tool`, ...).

### Per-calendar colors

Each CalDAV calendar is labelled in the output by its display name (e.g.
`Work`, `Personal`) and given a distinct color automatically. To pin specific
colors, add a `calendars` map to the source:

```json
{
  "name": "fastmail",
  "type": "caldav",
  "url": "https://caldav.fastmail.com/",
  "username": "you@fastmail.com",
  "passcmd": "security find-generic-password -a you@fastmail.com -s qcal-fastmail -w",
  "calendars": {
    "Work": "63",
    "Personal": "170"
  }
}
```

Calendars not listed fall back to the palette.

### Multiple sources

```json
{
  "sources": [
    {
      "name": "work",
      "type": "google",
      "color": "63",
      "client_id": "YOUR_CLIENT_ID",
      "client_secret": "YOUR_CLIENT_SECRET",
      "refresh_token": "YOUR_REFRESH_TOKEN",
      "token_uri": "https://oauth2.googleapis.com/token"
    },
    {
      "name": "personal",
      "type": "caldav",
      "color": "170",
      "url": "https://caldav.fastmail.com/",
      "username": "YOUR_USERNAME",
      "password": "YOUR_APP_PASSWORD"
    }
  ]
}
```

Colors can be any [lipgloss color](https://github.com/charmbracelet/lipgloss) value (ANSI number, hex, etc.). If omitted, a palette is auto-assigned.

## Usage

```
qcal [flags]
```

| Flag        | Default | Description                                     |
|-------------|---------|-------------------------------------------------|
| `-days`     | `1`     | Number of days to fetch (starting today)        |
| `-future`   | `false` | Only show meetings that haven't ended yet       |
| `-text`     | `false` | Show colored terminal output (instead of JSON)  |
| `-tui`      | `false` | Interactive split-pane TUI                      |

### JSON (default)

```bash
qcal -days 7
```

Pipes cleanly into `jq`:

```bash
qcal -days 7 | jq '.[] | {summary, start, end}'
```

### Text mode

```bash
qcal -days 3 -text
```

Grouped by date, color-coded by status:

| Indicator | Meaning    |
|-----------|------------|
| `✓`       | Past       |
| `▶`       | In progress|
| `○`       | Upcoming   |
| `◆`       | All day    |

### TUI mode

```bash
qcal -days 7 -tui
```

Split-pane interface:

- **Left** — scrollable meeting list grouped by date
- **Right** — detail pane showing time, status, location, attendees, description, and conflicts

| Key          | Action                                   |
|--------------|------------------------------------------|
| `←` / `→`   | Switch pane focus (list / detail)        |
| `↑` / `↓`   | Navigate (list items / detail scroll)    |
| `g` / `G`   | Go to top / bottom                       |
| `q` / `Esc` | Quit                                     |

## Configuration Reference

| Field            | Type     | Required | Description                                      |
|------------------|----------|----------|--------------------------------------------------|
| `name`           | string   | yes      | Unique label for this source                     |
| `type`           | string   | yes      | `"google"` or `"caldav"`                         |
| `color`          | string   | no       | Source color override (lipgloss color)           |
| `client_id`      | string   | google   | Google OAuth2 client ID                          |
| `client_secret`  | string   | google   | Google OAuth2 client secret                      |
| `refresh_token`  | string   | google   | Google OAuth2 refresh token                      |
| `token_uri`      | string   | google   | Google OAuth2 token endpoint                     |
| `url`            | string   | caldav   | CalDAV server URL                                |
| `username`       | string   | caldav   | CalDAV username                                  |
| `password`       | string   | caldav   | CalDAV password (app password recommended)       |
| `passcmd`        | string   | no       | Shell command whose stdout is used as the secret |
| `calendars`      | object   | no       | CalDAV calendar name → color override            |
