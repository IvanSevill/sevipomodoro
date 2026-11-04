package pomodoro

import (
	"math"
	"time"
)

type Phase int

const (
	Focus Phase = iota
	ShortBreak
	LongBreak
	FeedbackDuration = 3 * time.Second
	SafetyDuration   = 4 * time.Hour
)

func (p Phase) String() string {
	switch p {
	case ShortBreak:
		return "Short break"
	case LongBreak:
		return "Long break"
	default:
		return "Focus"
	}
}

type Stats struct {
	CompletedFocus int   `json:"completed_focus"`
	FocusSeconds   int64 `json:"focus_seconds"`
}

type Session struct {
	Config                                        Config
	Stats                                         Stats
	Phase                                         Phase
	Running                                       bool
	Ended                                         bool
	EndReason                                     string
	started                                       bool
	start, last                                   time.Time
	active, phaseElapsed, feedback, focusFraction time.Duration
	completed                                     int
}

func NewSession(config Config, stats Stats) *Session {
	if config.Validate() != nil {
		config = DefaultConfig()
	}
	stats.CompletedFocus = max(0, stats.CompletedFocus)
	stats.FocusSeconds = max(0, stats.FocusSeconds)
	return &Session{Config: config.NormalizeCadence(), Stats: stats}
}

// Snapshot projects to a timestamp without mutating the live session.
func (s *Session) Snapshot(now time.Time) Session { copy := *s; copy.Tick(now); return copy }

func (s *Session) phaseRemaining() time.Duration {
	if s.Ended {
		return 0
	}
	left := max(0, s.Config.Duration(s.Phase)-s.phaseElapsed)
	if s.Config.TotalMinutes > 0 {
		left = min(left, max(0, time.Duration(s.Config.TotalMinutes)*time.Minute-s.active))
	}
	return left
}

func (s *Session) Remaining(now time.Time) time.Duration {
	v := s.Snapshot(now)
	return v.phaseRemaining()
}
func (s *Session) Elapsed(now time.Time) time.Duration           { return s.Snapshot(now).phaseElapsed }
func (s *Session) ActiveElapsed(now time.Time) time.Duration     { return s.Snapshot(now).active }
func (s *Session) FeedbackRemaining(now time.Time) time.Duration { return s.Snapshot(now).feedback }
func (s *Session) TotalRemaining(now time.Time) time.Duration {
	v := s.Snapshot(now)
	return max(0, time.Duration(v.Config.TotalMinutes)*time.Minute-v.active)
}
func (s *Session) Final(now time.Time) bool {
	v := s.Snapshot(now)
	return !v.Ended && v.Config.TotalMinutes > 0 && time.Duration(v.Config.TotalMinutes)*time.Minute-v.active <= v.Config.Duration(v.Phase)-v.phaseElapsed
}
func (s *Session) Progress(now time.Time) float64 {
	v := s.Snapshot(now)
	total := v.phaseElapsed + v.phaseRemaining()
	if total == 0 {
		return 0
	}
	return min(1, float64(v.phaseElapsed)/float64(total))
}

func (s *Session) Toggle(now time.Time) {
	s.Tick(now)
	if s.Ended {
		return
	}
	if !s.started {
		s.started = true
		s.start = now
	}
	s.Running = !s.Running
	s.last = now
}

// Reset only resets the current phase; spent budget and safety origin survive.
// Once ended, reset prepares a genuinely new session, retaining lifetime stats.
func (s *Session) Reset(now time.Time) {
	s.Tick(now)
	if s.Ended {
		*s = *NewSession(s.Config, s.Stats)
		return
	}
	s.Running = false
	s.phaseElapsed = 0
	s.last = now
}

func (s *Session) Skip(now time.Time) Phase {
	s.Tick(now)
	if !s.Ended {
		s.advance(false)
	}
	return s.Phase
}

func (s *Session) finish(reason string) {
	s.Ended = true
	s.Running = false
	s.feedback = 0
	s.EndReason = reason
}

// Tick splits elapsed intervals at feedback, phase, total, and safety boundaries.
// Repeated or out-of-order timestamps consume nothing.
func (s *Session) Tick(now time.Time) int {
	if !s.started || s.Ended || now.Before(s.last) {
		return 0
	}
	until := now
	safety := s.Config.TotalMinutes == 0 && !now.Before(s.start.Add(SafetyDuration))
	if safety {
		until = s.start.Add(SafetyDuration)
	}
	left := max(0, until.Sub(s.last))
	transitions := 0
	for s.Running && !s.Ended && left > 0 {
		if s.feedback > 0 {
			spent := min(left, s.feedback)
			s.feedback -= spent
			left -= spent
			continue
		}
		spent := min(left, s.phaseRemaining())
		s.active += spent
		s.phaseElapsed += spent
		left -= spent
		if s.Phase == Focus {
			s.focusFraction += spent
			s.Stats.FocusSeconds += min(math.MaxInt64-s.Stats.FocusSeconds, int64(s.focusFraction/time.Second))
			s.focusFraction %= time.Second
		}
		full := s.phaseElapsed >= s.Config.Duration(s.Phase)
		if s.Config.TotalMinutes > 0 && s.active >= time.Duration(s.Config.TotalMinutes)*time.Minute {
			if full && s.Phase == Focus {
				s.completeFocus()
			}
			s.finish("Session complete")
		} else if full {
			s.advance(true)
			transitions++
		}
	}
	s.last = now
	if safety && !s.Ended {
		s.finish("Session ended — safety limit reached")
	}
	return transitions
}

func (s *Session) ApplyConfig(config Config, now time.Time) {
	s.Tick(now)
	if config.Validate() != nil {
		return
	}
	s.Config = config.NormalizeCadence()
	if s.Ended {
		return
	}
	if s.started && s.Config.TotalMinutes == 0 && !now.Before(s.start.Add(SafetyDuration)) {
		s.finish("Session ended — safety limit reached")
	} else if s.Config.TotalMinutes > 0 && s.active >= time.Duration(s.Config.TotalMinutes)*time.Minute {
		s.finish("Session complete")
	} else if s.phaseElapsed >= s.Config.Duration(s.Phase) {
		// Past time stays in its original phase, never in a newly created break.
		s.advance(true)
	}
}

func (s *Session) Checkpoint(now time.Time) { s.Tick(now); s.Running = false }

func (s *Session) advance(completed bool) {
	if s.Phase == Focus {
		if completed {
			s.completeFocus()
		}
		s.Phase = ShortBreak
		if completed && s.Config.SessionsBeforeLong > 1 && s.completed%s.Config.SessionsBeforeLong == 0 {
			s.Phase = LongBreak
		}
	} else {
		s.Phase = Focus
	}
	s.phaseElapsed = 0
	s.feedback = FeedbackDuration
}

func (s *Session) completeFocus() {
	if s.Stats.CompletedFocus < math.MaxInt {
		s.Stats.CompletedFocus++
	}
	s.completed++
}
