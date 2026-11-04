package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/ivansevill/sevipomodoro/internal/pomodoro"
)

const footer = "sevipomodoro - ivansevill"

// Every span paints its own background, including alignment and trailing spaces.
// Avoid nesting styled strings: an inner ANSI reset would expose terminal defaults.
type painter struct{ theme }

func (p painter) ink(text string, fg, bg lipgloss.Color, bold bool) string {
	return lipgloss.NewStyle().Foreground(fg).Background(bg).Bold(bold).Render(text)
}

func (p painter) line(text string, width int, fg, bg lipgloss.Color, center, bold bool) string {
	text = strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(ansi.Strip(text))
	text = ansi.Truncate(text, max(0, width), "")
	space := max(0, width-ansi.StringWidth(text))
	left := 0
	if center {
		left = space / 2
	}
	return p.ink(strings.Repeat(" ", left)+text+strings.Repeat(" ", space-left), fg, bg, bold)
}

func (m Model) View() string {
	now := m.now()
	m.now = func() time.Time { return now }
	snapshot := m.session.Snapshot(now)
	m.session = &snapshot
	w, h := m.width, m.height
	if w <= 0 || h <= 0 {
		return ""
	}
	t := currentTheme(m.session.Config.Theme)
	if m.settings {
		t = currentTheme(m.draft.Theme)
	}
	p := painter{t}
	panelWidth := min(w, 88)
	if w >= 44 {
		panelWidth = min(w-8, 88)
	}
	bordered := !m.settings && panelWidth >= 4 && h >= 7
	innerWidth, budget := panelWidth, max(0, h-2)
	if bordered {
		innerWidth -= 2
		budget -= 2
	}
	rows := m.timerRows(p, innerWidth, budget)
	if bordered {
		color, side := t.accent, "┃"
		topLeft, topRight, bottomLeft, bottomRight, horizontal := "┏", "┓", "┗", "┛", "━"
		if m.session.Phase != pomodoro.Focus {
			color, side = t.muted, "│"
			topLeft, topRight, bottomLeft, bottomRight, horizontal = "╭", "╮", "╰", "╯", "─"
		}
		framed := []string{p.line(topLeft+strings.Repeat(horizontal, innerWidth)+topRight, panelWidth, color, t.surface, false, true)}
		for _, row := range rows {
			framed = append(framed, p.ink(side, color, t.surface, true)+row+p.ink(side, color, t.surface, true))
		}
		rows = append(framed, p.line(bottomLeft+strings.Repeat(horizontal, innerWidth)+bottomRight, panelWidth, color, t.surface, false, true))
	}
	if m.settings {
		rows = m.settingsRows(p, panelWidth, max(0, h-2))
	}
	// Reserve the last row for attribution; even tiny terminals stay in bounds.
	rows = rows[:min(len(rows), max(0, h-1))]
	top := max(0, (h-2-len(rows))/2)
	result := make([]string, 0, h)
	blank := p.line("", w, t.text, t.canvas, false, false)
	for range top {
		result = append(result, blank)
	}
	left := (w - panelWidth) / 2
	for _, row := range rows {
		result = append(result, p.ink(strings.Repeat(" ", left), t.text, t.canvas, false)+row+
			p.ink(strings.Repeat(" ", w-panelWidth-left), t.text, t.canvas, false))
	}
	for len(result) < h-1 {
		result = append(result, blank)
	}
	result = append(result, p.line(footer, w, t.muted, t.canvas, true, false))
	return strings.Join(result, "\n")
}

func (m Model) timerRows(p painter, w, budget int) []string {
	now := m.now()
	remaining := max(0, m.session.Remaining(now)+time.Second-1) / time.Second * time.Second
	clock := fmt.Sprintf("%02d:%02d", int(remaining.Minutes()), int(remaining.Seconds())%60)
	state := "PAUSED"
	if m.session.Running {
		state = "RUNNING"
	}
	if m.session.Ended {
		state = "ENDED"
	}
	phase := strings.ToUpper(m.session.Phase.String())
	notice := "Open-ended"
	if m.session.Config.TotalMinutes > 0 {
		notice = "Total remaining " + compactDuration(m.session.TotalRemaining(now))
	}
	if m.session.Final(now) {
		notice = "Final session · " + notice
	}
	if m.session.FeedbackRemaining(now) > 0 {
		phase = "Time to focus"
		if m.session.Phase != pomodoro.Focus {
			phase = "Time for a break"
		}
	}
	if m.session.Ended {
		notice = m.session.EndReason
	}
	line := func(s string, fg lipgloss.Color, bold bool) string {
		return p.line(s, w, fg, p.surface, true, bold)
	}
	if budget < 5 {
		return []string{line(phase+" / "+state, p.accent, true), line(clock+" · "+notice, p.text, true), line("space toggle · t edit · q", p.muted, false)}
	}
	rows := []string{line(phase+"  /  "+state, p.accent, true), line(notice, p.muted, false)}
	large := bigClock(clock)
	if budget >= 18 && w >= ansi.StringWidth(large[0])+4 {
		rows = append(rows, line("", p.text, false))
		for _, row := range large {
			rows = append(rows, line(row, p.text, true))
		}
		rows = append(rows, line("", p.text, false))
	} else {
		rows = append(rows, line(clock, p.text, true))
	}
	barWidth := max(1, min(w-6, 64))
	rows = append(rows, p.progress(m.session.Progress(now), barWidth, w))
	if budget >= 10 {
		rows = append(rows, line(fmt.Sprintf("%d%% elapsed", int(m.session.Progress(now)*100)), p.muted, false))
	}
	if budget >= 14 {
		rows = append(rows, line("", p.text, false))
		stats := fmt.Sprintf("%d sessions  ·  %dh %02dm focused", m.session.Stats.CompletedFocus, m.session.Stats.FocusSeconds/3600, m.session.Stats.FocusSeconds/60%60)
		rows = append(rows, line(stats, p.text, true))
	}
	help := []string{"space/enter start/pause · r reset · s/n skip", "t/, settings · q quit · ctrl+c exit"}
	if w < 44 {
		help = []string{"space start/pause · r reset", "s skip · t settings · q quit"}
	}
	if budget < 6 {
		help = []string{"space toggle · t edit · q"}
	}
	if len(rows)+len(help)+2 <= budget {
		rows = append(rows, line("", p.text, false), line(m.status, p.muted, false))
	}
	for _, row := range help {
		rows = append(rows, line(row, p.muted, false))
	}
	if len(rows) < budget {
		rows = append(rows, line("", p.text, false))
	}
	return rows
}

func (p painter) progress(progress float64, width, fullWidth int) string {
	filled := int(max(0, min(1, progress)) * float64(width))
	left := (fullWidth - width) / 2
	return p.ink(strings.Repeat(" ", left), p.text, p.surface, false) +
		p.ink(strings.Repeat("━", filled), p.accent, p.surface, true) +
		p.ink(strings.Repeat("─", width-filled), p.track, p.surface, false) +
		p.ink(strings.Repeat(" ", fullWidth-width-left), p.text, p.surface, false)
}

func (m Model) settingsRows(p painter, w, budget int) []string {
	values := []struct{ label, value string }{
		{"Focus", fmt.Sprintf("%d min", m.draft.FocusMinutes)},
		{"Short break", fmt.Sprintf("%d min", m.draft.ShortBreakMinutes)},
		{"Long break", fmt.Sprintf("%d min", m.draft.LongBreakMinutes)},
		{"Long break every", fmt.Sprintf("%d focus blocks", m.draft.SessionsBeforeLong)},
		{"Total duration", totalLabel(m.draft.TotalMinutes)},
		{"Terminal bell", onOff(m.draft.Bell)},
		{"Theme", p.name},
	}
	if m.draft.SessionsBeforeLong == 1 {
		values[2].value = "Disabled"
		values[3].value = "Disabled"
	}
	line := func(s string, fg lipgloss.Color, bold bool) string {
		return p.line(s, w, fg, p.surface, true, bold)
	}
	if budget < 4 {
		item := values[m.settingRow]
		return []string{line(item.label+": "+item.value, p.accent, true), line("←/→ edit · enter apply · esc back", p.muted, false)}
	}
	rows := []string{line(fmt.Sprintf("SETTINGS  /  %d of %d", m.settingRow+1, settingCount), p.accent, true)}
	if budget >= 5 {
		live := m.session.Phase.String() + " · timer paused"
		if m.session.Running {
			live = m.session.Phase.String() + " · timer running"
		}
		if m.session.FeedbackRemaining(m.now()) > 0 {
			live = "Time to focus"
			if m.session.Phase != pomodoro.Focus {
				live = "Time for a break"
			}
		}
		if m.session.Ended {
			live = m.session.EndReason
		}
		rows = append(rows, line(live, p.accent, true))
	}
	help := []string{"↑/↓ select · ←/→ adjust", "enter apply · esc discard"}
	if w >= 60 {
		help = []string{"↑/↓ select · ←/→ adjust · enter apply · esc discard"}
	}
	room := max(1, budget-len(rows)-len(help))
	if room >= settingCount+4 {
		rows = append(rows, line("Tune your rhythm. Preview your palette.", p.muted, false), line("", p.text, false))
		room -= 4
	}
	visible := min(settingCount, room)
	start := min(max(0, m.settingRow-visible/2), settingCount-visible)
	for i := start; i < start+visible; i++ {
		item := values[i]
		fg, bg, marker := p.text, p.surface, "  "
		if i == m.settingRow {
			fg, bg, marker = p.canvas, p.accent, "› "
		}
		inner := max(0, w-4)
		gap := max(1, inner-ansi.StringWidth(marker+item.label+item.value))
		text := marker + item.label + strings.Repeat(" ", gap) + item.value
		if ansi.StringWidth(text) > inner {
			text = marker + item.label + ": " + item.value
		}
		rows = append(rows, p.line("  "+text, w, fg, bg, false, i == m.settingRow))
	}
	if len(rows)+len(help)+2 <= budget {
		rows = append(rows, line("", p.text, false), line(m.status, p.muted, false))
	}
	for _, s := range help {
		rows = append(rows, line(s, p.muted, false))
	}
	return rows
}

// Three-cell digits keep the clock legible without depending on terminal fonts.
func bigClock(clock string) []string {
	glyphs := map[rune][5]string{
		'0': {"┏━┓", "┃ ┃", "┃ ┃", "┃ ┃", "┗━┛"},
		'1': {" ╻ ", " ┃ ", " ┃ ", " ┃ ", " ╹ "},
		'2': {"┏━┓", "  ┃", "┏━┛", "┃  ", "┗━╸"},
		'3': {"┏━┓", "  ┃", " ━┫", "  ┃", "┗━┛"},
		'4': {"╻ ╻", "┃ ┃", "┗━┫", "  ┃", "  ╹"},
		'5': {"┏━╸", "┃  ", "┗━┓", "  ┃", "┗━┛"},
		'6': {"┏━┓", "┃  ", "┣━┓", "┃ ┃", "┗━┛"},
		'7': {"╺━┓", "  ┃", "  ┃", "  ┃", "  ╹"},
		'8': {"┏━┓", "┃ ┃", "┣━┫", "┃ ┃", "┗━┛"},
		'9': {"┏━┓", "┃ ┃", "┗━┫", "  ┃", "┗━┛"},
		':': {"   ", " ▪ ", "   ", " ▪ ", "   "},
	}
	rows := make([]string, 5)
	for i := range rows {
		parts := make([]string, 0, len(clock))
		for _, digit := range clock {
			parts = append(parts, glyphs[digit][i])
		}
		rows[i] = strings.Join(parts, "  ")
	}
	return rows
}

func compactDuration(duration time.Duration) string {
	minutes := int(duration.Round(time.Minute).Minutes())
	if minutes < 60 {
		return fmt.Sprintf("%dm", minutes)
	}
	return fmt.Sprintf("%dh %02dm", minutes/60, minutes%60)
}

func totalLabel(minutes int) string {
	if minutes == 0 {
		return "Open-ended"
	}
	return fmt.Sprintf("%d min", minutes)
}

func onOff(value bool) string {
	if value {
		return "On"
	}
	return "Off"
}
