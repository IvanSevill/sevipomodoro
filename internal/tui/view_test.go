package tui

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/ivansevill/sevipomodoro/internal/pomodoro"
	"github.com/ivansevill/sevipomodoro/internal/store"
	"github.com/muesli/termenv"
)

var update = flag.Bool("update", false, "Update deterministic view goldens")

func viewModel() Model {
	m := NewModel(store.DefaultState(), nil, "")
	m.now = func() time.Time { return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC) }
	return m
}

func colorProfile(t *testing.T, profile termenv.Profile) {
	t.Helper()
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(profile)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })
}

func TestViewResizeBounds(t *testing.T) {
	colorProfile(t, termenv.TrueColor)
	for _, settings := range []bool{false, true} {
		for _, size := range [][2]int{{0, 0}, {1, 1}, {12, 4}, {24, 7}, {28, 9}, {32, 16}, {40, 10}, {80, 24}, {120, 40}, {200, 12}, {28, 9}} {
			t.Run(fmt.Sprintf("settings=%t/%dx%d", settings, size[0], size[1]), func(t *testing.T) {
				m := viewModel()
				m.settings = settings
				m.settingRow = 6
				m.status = strings.Repeat("Long warning 界 ", 30) + "\nsecond line\tvalue"
				m.session.Config.FocusMinutes = pomodoro.MaxFocusMinutes
				m.session.Reset(m.now())
				updated, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
				view := updated.(Model).View()
				if size[0] == 0 {
					if view != "" {
						t.Fatal("unmeasured viewport must be empty")
					}
					return
				}
				lines := strings.Split(view, "\n")
				if len(lines) != size[1] {
					t.Fatalf("height = %d, want %d", len(lines), size[1])
				}
				for i, line := range lines {
					if got := ansi.StringWidth(line); got != size[0] {
						t.Fatalf("row %d width = %d, want %d", i, got, size[0])
					}
				}
				if size[0] >= len(footer) && strings.TrimSpace(ansi.Strip(lines[len(lines)-1])) != footer {
					t.Fatal("missing exact footer")
				}
				if strings.Contains(strings.ToLower(view), "sevi pomodoro") {
					t.Fatal("obsolete heading")
				}
				if settings && size[0] >= 24 && size[1] >= 7 && !strings.Contains(ansi.Strip(view), "› Theme") {
					t.Fatal("selected setting scrolled out of view")
				}
			})
		}
	}
}

func TestViewGolden(t *testing.T) {
	colorProfile(t, termenv.Ascii)
	for _, tt := range []struct {
		name     string
		w, h     int
		settings bool
		row      int
	}{
		{"timer-wide", 80, 24, false, 0},
		{"timer-narrow", 32, 16, false, 0},
		{"timer-short", 28, 9, false, 0},
		{"timer-progress", 80, 24, false, 0},
		{"timer-break", 80, 24, false, 0},
		{"timer-feedback", 32, 16, false, 0},
		{"timer-final", 80, 24, false, 0},
		{"timer-safety", 80, 24, false, 0},
		{"settings-wide", 80, 24, true, 6},
		{"settings-short", 40, 10, true, 6},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := viewModel()
			m.width, m.height, m.settings, m.settingRow = tt.w, tt.h, tt.settings, tt.row
			switch tt.name {
			case "timer-break", "timer-feedback":
				start := m.now()
				m.session.Toggle(start)
				m.session.Skip(start)
				if tt.name == "timer-break" {
					m.now = func() time.Time { return start.Add(pomodoro.FeedbackDuration) }
				}
			case "timer-final":
				c := m.session.Config
				c.TotalMinutes = 5
				m.session.ApplyConfig(c, m.now())
			case "timer-safety":
				start := m.now()
				m.session.Toggle(start)
				m.session.Toggle(start)
				m.now = func() time.Time { return start.Add(pomodoro.SafetyDuration) }
			}
			if tt.name == "timer-progress" {
				start := m.now()
				m.session.Toggle(start)
				m.now = func() time.Time { return start.Add(12*time.Minute + 30*time.Second) }
			}
			got := m.View() + "\n"
			path := filepath.Join("testdata", tt.name+".golden")
			if *update {
				if err := os.MkdirAll("testdata", 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(got), 0644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if got != string(want) {
				t.Fatalf("view differs from %s; inspect before using -update", path)
			}
		})
	}
}

func TestPhaseBorderAndBudgetLabels(t *testing.T) {
	colorProfile(t, termenv.TrueColor)
	m := viewModel()
	m.width, m.height = 80, 24
	focus := m.View()
	if !strings.Contains(ansi.Strip(focus), "┃") || !strings.Contains(focus, "Open-ended") {
		t.Fatal("missing focus halo/open-ended label")
	}
	if strings.Contains(focus, "Cycle") || strings.Contains(focus, "∞") {
		t.Fatal("false cycle limit/infinity")
	}
	m.session.Skip(m.now())
	rest := m.View()
	if !strings.Contains(ansi.Strip(rest), "│") || focus == rest {
		t.Fatal("break not visually distinct")
	}
	assertBackgrounds(t, rest, currentTheme("ember"))
	c := m.session.Config
	c.TotalMinutes = 1
	m.session.ApplyConfig(c, m.now())
	if !strings.Contains(m.View(), "Final session") {
		t.Fatal("missing final indicator in clipped break")
	}
}

// Inspect active SGR background at every cell, including the last space in a row.
// Plain-text snapshots cannot catch a terminal-default background leaking through.
func assertBackgrounds(t *testing.T, view string, palette theme) {
	t.Helper()
	canvas := termenv.RGBColor(palette.canvas).Sequence(true)
	surface := termenv.RGBColor(palette.surface).Sequence(true)
	accent := termenv.RGBColor(palette.accent).Sequence(true)
	sgr := regexp.MustCompile(`\x1b\[([0-9;]*)m`)
	for row, line := range strings.Split(view, "\n") {
		bg, col, panelBG := "", 0, ""
		for len(line) > 0 {
			if loc := sgr.FindStringSubmatchIndex(line); loc != nil && loc[0] == 0 {
				codes := strings.Split(line[loc[2]:loc[3]], ";")
				for i := 0; i < len(codes); i++ {
					switch codes[i] {
					case "", "0", "49":
						bg = ""
					case "38", "48":
						if i+4 < len(codes) && codes[i+1] == "2" {
							if codes[i] == "48" {
								bg = strings.Join(codes[i:i+5], ";")
							}
							i += 4
						}
					}
				}
				line = line[loc[1]:]
				continue
			}
			r := []rune(line)[0]
			if bg != canvas && bg != surface && bg != accent {
				t.Fatalf("unpainted/foreign background %q at row %d col %d", bg, row, col)
			}
			if (col < 4 || col >= 76 || row == 23) && bg != canvas {
				t.Fatalf("canvas mismatch row %d col %d: %s", row, col, bg)
			}
			if col == 4 {
				panelBG = bg
			}
			if col >= 4 && col < 76 && bg != panelBG {
				t.Fatalf("panel fill changes at row %d col %d", row, col)
			}
			col += ansi.StringWidth(string(r))
			line = strings.TrimPrefix(line, string(r))
		}
	}
}

func TestThemeBackgroundAndLivePreview(t *testing.T) {
	colorProfile(t, termenv.TrueColor)
	for _, name := range pomodoro.ThemeNames() {
		t.Run(name, func(t *testing.T) {
			m := viewModel()
			m.width, m.height = 80, 24
			m.session.Config.Theme = name
			assertBackgrounds(t, m.View(), currentTheme(name))
			updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
			m = updated.(Model)
			m.settingRow = 6
			updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
			m = updated.(Model)
			if m.draft.Theme == name || m.session.Config.Theme != name {
				t.Fatal("preview must change draft only")
			}
			assertBackgrounds(t, m.View(), currentTheme(m.draft.Theme))
			updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
			m = updated.(Model)
			assertBackgrounds(t, m.View(), currentTheme(name))
		})
	}
	if currentTheme("unknown") != currentTheme("ember") {
		t.Fatal("unknown theme must fall back to ember")
	}
}

func TestProgressBounds(t *testing.T) {
	colorProfile(t, termenv.Ascii)
	for _, value := range []float64{-1, 0, .5, 1, 2} {
		bar := (painter{currentTheme("ember")}).progress(value, 12, 20)
		if got := ansi.StringWidth(bar); got != 20 {
			t.Fatalf("progress %v width = %d", value, got)
		}
		if got := strings.Count(bar, "━"); got != int(max(0, min(1, value))*12) {
			t.Fatalf("progress %v has %d filled cells", value, got)
		}
	}
}

func TestResizePreservesRunningSession(t *testing.T) {
	m := viewModel()
	start := m.now()
	m.session.Toggle(start)
	m.now = func() time.Time { return start.Add(10 * time.Minute) }
	for _, size := range [][2]int{{120, 40}, {28, 9}, {1, 1}, {80, 24}} {
		updated, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m = updated.(Model)
		_ = m.View()
		if !m.session.Running || m.session.Remaining(m.now()) != 15*time.Minute || m.session.Phase != pomodoro.Focus {
			t.Fatalf("resize to %v changed the running session", size)
		}
	}
}

func TestColorCapabilityFallbacks(t *testing.T) {
	for _, profile := range []termenv.Profile{termenv.TrueColor, termenv.ANSI256, termenv.ANSI, termenv.Ascii} {
		t.Run(fmt.Sprint(profile), func(t *testing.T) {
			colorProfile(t, profile)
			m := viewModel()
			m.width, m.height = 32, 16
			view := m.View()
			if profile == termenv.Ascii && strings.Contains(view, "\x1b") {
				t.Fatal("plain terminal received ANSI")
			}
			for _, row := range strings.Split(view, "\n") {
				if ansi.StringWidth(row) != 32 {
					t.Fatal("color conversion changed render bounds")
				}
			}
		})
	}
}
