package main

import (
	"image"
	"image/png"
	"os"
	"testing"

	"fyne.io/fyne/v2"
	fynetest "fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func TestGameWindowRendersAtDesktopSize(t *testing.T) {
	a := fynetest.NewApp()
	a.Settings().SetTheme(&anthemTheme{base: theme.DefaultTheme(), regular: fontData})
	w := newGameWindow(a)
	w.Resize(fyne.NewSize(1080, 720))
	t.Cleanup(w.Close)

	image := w.Canvas().Capture()
	if got := image.Bounds().Size(); got.X != 1080 || got.Y != 720 {
		t.Fatalf("captured window is %dx%d, want 1080x720", got.X, got.Y)
	}

	// Set MK01_CAPTURE_PATH when a human-readable render is useful during
	// visual regression work. Normal test runs never write an artifact.
	if path := os.Getenv("MK01_CAPTURE_PATH"); path != "" {
		writeCapture(t, path, image)
	}

	entry := findEntry(w.Content())
	if entry == nil {
		t.Fatal("practice entry was not found in the rendered window")
	}
	for _, line := range anthemLyrics {
		entry.SetText(line.Text)
		entry.OnSubmitted(entry.Text)
	}
	if !entry.Disabled() {
		t.Fatal("practice entry remains enabled after completing every line")
	}
	if path := os.Getenv("MK01_RESULT_CAPTURE_PATH"); path != "" {
		writeCapture(t, path, w.Canvas().Capture())
	}
}

func TestTargetFeedbackSegmentsStayOnOneLine(t *testing.T) {
	target := widget.NewRichText()
	setTargetSegments(target, "가나다", "가X")

	if len(target.Segments) != 3 {
		t.Fatalf("target has %d feedback segments, want 3", len(target.Segments))
	}
	wantColors := []fyne.ThemeColorName{
		theme.ColorNameSuccess,
		theme.ColorNameError,
		theme.ColorNameForeground,
	}
	for index, raw := range target.Segments {
		segment, ok := raw.(*widget.TextSegment)
		if !ok {
			t.Fatalf("segment %d has type %T, want TextSegment", index, raw)
		}
		if !segment.Style.Inline {
			t.Fatalf("segment %d is block-level and would wrap Korean text vertically", index)
		}
		if segment.Style.ColorName != wantColors[index] {
			t.Fatalf("segment %d color=%q, want %q", index, segment.Style.ColorName, wantColors[index])
		}
	}
}

func findEntry(object fyne.CanvasObject) *widget.Entry {
	if entry, ok := object.(*widget.Entry); ok {
		return entry
	}
	if group, ok := object.(*fyne.Container); ok {
		for _, child := range group.Objects {
			if entry := findEntry(child); entry != nil {
				return entry
			}
		}
	}
	return nil
}

func writeCapture(t *testing.T, path string, captured image.Image) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, captured); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
