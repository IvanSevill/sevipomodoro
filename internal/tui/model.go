package tui

import (
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ivansevill/sevipomodoro/internal/pomodoro"
	"github.com/ivansevill/sevipomodoro/internal/store"
)

const tickInterval = 200 * time.Millisecond

type stateSaver interface {
	Save(store.State) error
}

type tickMsg time.Time
type bellMsg struct{}

type Model struct {
	session    *pomodoro.Session
	saver      stateSaver
	width      int
	height     int
	settings   bool
	settingRow int
	draft      pomodoro.Config
	status     string
	now        func() time.Time
}

func NewModel(state store.State, saver stateSaver, warning string) Model {
	return Model{
		session: pomodoro.NewSession(state.Config, state.Stats),
		saver:   saver,
		draft:   state.Config,
		status:  warning,
		now:     time.Now,
	}
}

func (m Model) Init() tea.Cmd {
	return tickCmd()
}

func tickCmd() tea.Cmd {
	return tea.Tick(tickInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func bellCmd() tea.Cmd {
	return func() tea.Msg {
		_, _ = os.Stdout.WriteString("\a")
		return bellMsg{}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tickMsg:
		bell := m.tick(time.Time(msg))
		return m, tea.Batch(tickCmd(), bell)
	case tea.KeyMsg:
		bell := m.tick(m.now())
		var updated tea.Model
		var command tea.Cmd
		if m.settings {
			updated, command = m.updateSettings(msg)
		} else {
			updated, command = m.updateTimer(msg)
		}
		return updated, tea.Batch(bell, command)
	}
	return m, nil
}

// Keys can reach a deadline before the scheduled tick message does. Account for
// those transitions through the same persistence and notification boundary.
func (m *Model) tick(now time.Time) tea.Cmd {
	wasEnded := m.session.Ended
	transitions := m.session.Tick(now)
	if transitions > 0 || (!wasEnded && m.session.Ended) {
		_ = m.persist()
		if transitions > 0 && !m.session.Ended && m.bellEnabled() {
			return bellCmd()
		}
	}
	return nil
}

func (m Model) updateTimer(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	now := m.now()
	switch msg.String() {
	case "ctrl+c", "q":
		m.session.Checkpoint(now)
		_ = m.persist()
		return m, tea.Quit
	case " ", "enter":
		m.session.Toggle(now)
		if m.session.Running {
			m.status = "Timer running"
		} else {
			m.status = "Timer paused"
		}
	case "r":
		m.session.Reset(now)
		if err := m.persist(); err == nil {
			m.status = "Current phase reset"
		}
	case "s", "n":
		m.session.Skip(now)
		if err := m.persist(); err == nil {
			m.status = "Skipped to " + m.session.Phase.String()
		}
		if !m.session.Ended && m.bellEnabled() {
			return m, bellCmd()
		}
	case "t", ",":
		m.settings = true
		m.settingRow = 0
		m.draft = m.session.Config
		m.status = "Adjust with left/right or -/+"
	}
	return m, nil
}

func (m Model) updateSettings(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.session.Checkpoint(m.now())
		_ = m.persist()
		return m, tea.Quit
	case "esc", "q", "t":
		m.settings = false
		m.status = "Settings discarded"
	case "up", "k":
		m.settingRow = (m.settingRow + settingCount - 1) % settingCount
	case "down", "j", "tab":
		m.settingRow = (m.settingRow + 1) % settingCount
	case "left", "h", "-":
		m.adjustSetting(-1)
	case "right", "l", "+", "=":
		m.adjustSetting(1)
	case "enter":
		if err := m.draft.Validate(); err != nil {
			m.status = err.Error()
			return m, nil
		}
		phase := m.session.Phase
		m.session.ApplyConfig(m.draft, m.now())
		m.settings = false
		if err := m.persist(); err == nil {
			m.status = "Settings applied; progress preserved"
		}
		if phase != m.session.Phase && !m.session.Ended && m.bellEnabled() {
			return m, bellCmd()
		}
	}
	return m, nil
}

const settingCount = 7

func (m *Model) adjustSetting(delta int) {
	switch m.settingRow {
	case 0:
		m.draft.FocusMinutes = clamp(m.draft.FocusMinutes+delta, pomodoro.MinFocusMinutes, pomodoro.MaxFocusMinutes)
	case 1:
		m.draft.ShortBreakMinutes = clamp(m.draft.ShortBreakMinutes+delta, pomodoro.MinBreakMinutes, pomodoro.MaxBreakMinutes)
	case 2:
		if m.draft.SessionsBeforeLong > 1 {
			m.draft.LongBreakMinutes = clamp(m.draft.LongBreakMinutes+delta, pomodoro.MinBreakMinutes, pomodoro.MaxBreakMinutes)
		}
	case 3:
		m.draft.SessionsBeforeLong = clamp(m.draft.SessionsBeforeLong+delta, pomodoro.MinSessionsBeforeLong, pomodoro.MaxSessionsBeforeLong)
	case 4:
		m.draft.TotalMinutes = clamp(m.draft.TotalMinutes+delta*5, 0, pomodoro.MaxTotalMinutes)
	case 5:
		m.draft.Bell = !m.draft.Bell
	case 6:
		names := pomodoro.ThemeNames()
		index := 0
		for i, name := range names {
			if name == m.draft.Theme {
				index = i
			}
		}
		m.draft.Theme = names[(index+delta+len(names))%len(names)]
	}
	m.draft = m.draft.NormalizeCadence()
	m.status = "Press Enter to apply or Esc to discard"
}

func (m Model) bellEnabled() bool {
	if m.settings {
		return m.draft.Bell
	}
	return m.session.Config.Bell
}

func (m *Model) persist() error {
	if m.saver == nil {
		return nil
	}
	snapshot := m.session.Snapshot(m.now())
	if err := m.saver.Save(store.State{Config: m.session.Config, Stats: snapshot.Stats}); err != nil {
		m.status = fmt.Sprintf("Could not save state: %v", err)
		return err
	}
	return nil
}

func clamp(value, low, high int) int {
	return min(max(value, low), high)
}
