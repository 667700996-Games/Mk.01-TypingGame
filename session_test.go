package main

import (
	"math"
	"testing"
	"time"
)

func testSession(texts ...string) *PracticeSession {
	lines := make([]LyricLine, 0, len(texts))
	for _, text := range texts {
		lines = append(lines, LyricLine{Verse: 1, Text: text})
	}
	return NewPracticeSession(lines)
}

func TestLyricsForVerse(t *testing.T) {
	all := lyricsForVerse(0)
	if len(all) != 16 {
		t.Fatalf("complete anthem has %d lines, want 16", len(all))
	}

	for verse := 1; verse <= 4; verse++ {
		lines := lyricsForVerse(verse)
		if len(lines) != 4 {
			t.Fatalf("verse %d has %d lines, want 4", verse, len(lines))
		}
		for _, line := range lines {
			if line.Verse != verse {
				t.Fatalf("verse %d contains line from verse %d", verse, line.Verse)
			}
		}
	}

	all[0].Text = "changed"
	if anthemLyrics[0].Text == "changed" {
		t.Fatal("lyricsForVerse returned backing storage instead of a copy")
	}
}

func TestSessionScoresUnicodeInputAndCorrections(t *testing.T) {
	session := testSession("가나다")
	start := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	session.UpdateInput("가나X", start)
	if session.TotalTyped != 3 || session.CorrectTyped != 2 || session.Mistakes != 1 {
		t.Fatalf("unexpected initial score: total=%d correct=%d mistakes=%d", session.TotalTyped, session.CorrectTyped, session.Mistakes)
	}
	session.UpdateInput("가나", start.Add(time.Second))
	session.UpdateInput("가나다", start.Add(2*time.Second))

	stats := session.Stats(start.Add(time.Minute))
	if session.TotalTyped != 4 || session.CorrectTyped != 3 {
		t.Fatalf("unexpected corrected score: total=%d correct=%d", session.TotalTyped, session.CorrectTyped)
	}
	if math.Abs(stats.Accuracy-75) > 0.001 {
		t.Fatalf("accuracy=%f, want 75", stats.Accuracy)
	}
	if stats.CharactersPM != 4 {
		t.Fatalf("characters/min=%d, want 4", stats.CharactersPM)
	}
}

func TestSessionRequiresExactSubmissionAndFinishesOnce(t *testing.T) {
	session := testSession("첫 줄", "둘째 줄")
	now := time.Now()

	session.UpdateInput("첫줄", now)
	if got := session.Submit(now); got != SubmitIncomplete {
		t.Fatalf("inexact input result=%v, want incomplete", got)
	}
	session.UpdateInput("첫 줄", now)
	if got := session.Submit(now); got != SubmitAdvanced {
		t.Fatalf("first line result=%v, want advanced", got)
	}
	if session.Index != 1 || session.Input != "" {
		t.Fatalf("session did not advance cleanly: index=%d input=%q", session.Index, session.Input)
	}

	session.UpdateInput("둘째 줄", now.Add(time.Second))
	if got := session.Submit(now.Add(2 * time.Second)); got != SubmitFinished {
		t.Fatalf("last line result=%v, want finished", got)
	}
	if session.State != SessionComplete || session.Stats(now).Progress != 1 {
		t.Fatal("completed session did not expose final state")
	}
	if got := session.Submit(now.Add(3 * time.Second)); got != SubmitIncomplete {
		t.Fatalf("repeated submission result=%v, want incomplete", got)
	}
}

func TestMismatchCountIncludesOverflow(t *testing.T) {
	if got := mismatchCount("대한X사람!", "대한 사람"); got != 3 {
		t.Fatalf("mismatchCount=%d, want 3", got)
	}
}
