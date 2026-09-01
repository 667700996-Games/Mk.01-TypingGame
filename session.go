package main

import (
	"math"
	"time"
	"unicode/utf8"
)

type SessionState int

const (
	SessionReady SessionState = iota
	SessionActive
	SessionComplete
)

type SubmitResult int

const (
	SubmitIncomplete SubmitResult = iota
	SubmitAdvanced
	SubmitFinished
)

type SessionStats struct {
	Accuracy      float64
	CharactersPM int
	Elapsed       time.Duration
	Progress      float64
	Combo         int
	BestCombo     int
	Mistakes      int
}

// PracticeSession owns all gameplay state. Keeping it independent from Fyne
// makes scoring deterministic and lets the UI remain a thin view layer.
type PracticeSession struct {
	Lines          []LyricLine
	Index          int
	Input          string
	State          SessionState
	StartedAt      time.Time
	FinishedAt     time.Time
	TotalTyped     int
	CorrectTyped   int
	Mistakes       int
	Combo          int
	BestCombo      int
	completedLines int
	previousInput  string
}

func NewPracticeSession(lines []LyricLine) *PracticeSession {
	copyOfLines := make([]LyricLine, len(lines))
	copy(copyOfLines, lines)
	return &PracticeSession{Lines: copyOfLines, State: SessionReady}
}

func (s *PracticeSession) CurrentLine() LyricLine {
	if len(s.Lines) == 0 {
		return LyricLine{}
	}
	index := s.Index
	if index >= len(s.Lines) {
		index = len(s.Lines) - 1
	}
	return s.Lines[index]
}

// UpdateInput records only newly inserted Unicode characters. Deletions are
// free, so correcting a mistake never unfairly lowers a score twice.
func (s *PracticeSession) UpdateInput(value string, now time.Time) {
	if s.State == SessionComplete || len(s.Lines) == 0 || value == s.previousInput {
		return
	}

	previous := []rune(s.previousInput)
	next := []rune(value)
	target := []rune(s.CurrentLine().Text)
	prefix := commonPrefix(previous, next)
	previousEnd, nextEnd := len(previous), len(next)
	for previousEnd > prefix && nextEnd > prefix && previous[previousEnd-1] == next[nextEnd-1] {
		previousEnd--
		nextEnd--
	}

	inserted := next[prefix:nextEnd]
	if len(inserted) > 0 {
		if s.State == SessionReady {
			s.State = SessionActive
			s.StartedAt = now
		}
		for offset, typed := range inserted {
			targetIndex := prefix + offset
			s.TotalTyped++
			if targetIndex < len(target) && target[targetIndex] == typed {
				s.CorrectTyped++
				s.Combo++
				if s.Combo > s.BestCombo {
					s.BestCombo = s.Combo
				}
			} else {
				s.Mistakes++
				s.Combo = 0
			}
		}
	}

	s.Input = value
	s.previousInput = value
}

func (s *PracticeSession) Submit(now time.Time) SubmitResult {
	if s.State == SessionComplete || len(s.Lines) == 0 || s.Input != s.CurrentLine().Text {
		return SubmitIncomplete
	}

	s.completedLines++
	if s.Index+1 >= len(s.Lines) {
		s.State = SessionComplete
		s.FinishedAt = now
		return SubmitFinished
	}

	s.Index++
	s.Input = ""
	s.previousInput = ""
	return SubmitAdvanced
}

func (s *PracticeSession) Stats(now time.Time) SessionStats {
	accuracy := 100.0
	if s.TotalTyped > 0 {
		accuracy = float64(s.CorrectTyped) / float64(s.TotalTyped) * 100
	}

	elapsed := time.Duration(0)
	if !s.StartedAt.IsZero() {
		end := now
		if s.State == SessionComplete {
			end = s.FinishedAt
		}
		elapsed = end.Sub(s.StartedAt)
		if elapsed < 0 {
			elapsed = 0
		}
	}

	charactersPM := 0
	if elapsed > 0 {
		charactersPM = int(math.Round(float64(s.TotalTyped) / elapsed.Minutes()))
	}

	progress := 0.0
	if len(s.Lines) > 0 {
		lineProgress := 0.0
		targetLength := utf8.RuneCountInString(s.CurrentLine().Text)
		if targetLength > 0 && s.State != SessionComplete {
			lineProgress = float64(correctPrefixLength(s.Input, s.CurrentLine().Text)) / float64(targetLength)
		}
		progress = (float64(s.completedLines) + lineProgress) / float64(len(s.Lines))
		if s.State == SessionComplete {
			progress = 1
		}
	}

	return SessionStats{
		Accuracy:      accuracy,
		CharactersPM: charactersPM,
		Elapsed:       elapsed,
		Progress:      progress,
		Combo:         s.Combo,
		BestCombo:     s.BestCombo,
		Mistakes:      s.Mistakes,
	}
}

func commonPrefix(a, b []rune) int {
	limit := len(a)
	if len(b) < limit {
		limit = len(b)
	}
	for i := 0; i < limit; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return limit
}

func correctPrefixLength(input, target string) int {
	return commonPrefix([]rune(input), []rune(target))
}

func mismatchCount(input, target string) int {
	inputRunes, targetRunes := []rune(input), []rune(target)
	count := 0
	for index, typed := range inputRunes {
		if index >= len(targetRunes) || targetRunes[index] != typed {
			count++
		}
	}
	return count
}
