package main

import (
	_ "embed"
	"fmt"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	_ "golang.org/x/image/font/sfnt"
)

//go:embed fonts/NanumGothic-Regular.ttf
var fontData []byte

const (
	targetText = "" +
		"동해물과 백두산이 마르고 닳도록\n" +
		"하느님이 보우하사 우리나라 만세\n" +
		"무궁화 삼천리 화려강산\n" +
		"대한사람 대한으로 길이 보전하세\n" +
		"\n" +
		"남산 위에 저 소나무 철갑을 두른 듯\n" +
		"바람 서리 불변함은 우리 기상일세\n" +
		"무궁화 삼천리 화려강산\n" +
		"대한사람 대한으로 길이 보전하세\n" +
		"\n" +
		"가을 하늘 공활한데 높고 구름 없이\n" +
		"밝은 달은 우리 가슴 일편단심일세\n" +
		"무궁화 삼천리 화려강산\n" +
		"대한사람 대한으로 길이 보전하세\n" +
		"\n" +
		"이 기상과 이 맘으로 충성을 다하여\n" +
		"괴로우나 즐거우나 나라 사랑하세\n" +
		"무궁화 삼천리 화려강산\n" +
		"대한사람 대한으로 길이 보전하세"
	inputHint       = "현재 문장을 입력하세요."
	headerTitle     = "Mk.01-TypingGame"
	resetButtonText = "초기화"
)

func main() {
	a := app.New()

	customTheme := &fontTheme{base: theme.DefaultTheme(), regular: fontData}
	a.Settings().SetTheme(customTheme)

	w := a.NewWindow(headerTitle)
	w.Resize(fyne.NewSize(1000, 600))

	targetLines := normalizeLines(targetText)

	// --- UI Components ---

	// 1. Progress Label (Subtle, Top)
	currentLabel := widget.NewLabel(fmt.Sprintf("LEVEL %d / %d", 1, len(targetLines)))
	currentLabel.Alignment = fyne.TextAlignCenter
	currentLabel.TextStyle = fyne.TextStyle{Monospace: true}

	// 2. RichText for Target (The "Iconic" Centerpiece)
	currentTarget := widget.NewRichTextFromMarkdown("")
	currentTarget.Wrapping = fyne.TextWrapOff
	// We will center the *container* holding this, not the text segments themselves, to avoid breaking lines.

	// Helper to set initial text
	setTargetText := func(text string) {
		currentTarget.Segments = []widget.RichTextSegment{
			&widget.TextSegment{
				Text: text,
				Style: widget.RichTextStyle{
					SizeName:  theme.SizeNameHeadingText, // Large text
					ColorName: theme.ColorNameForeground,
				},
			},
		}
		currentTarget.Refresh()
	}
	setTargetText(targetLines[0])

	// 3. Input (Minimalist)
	input := widget.NewEntry()
	input.SetPlaceHolder(inputHint)
	input.TextStyle = fyne.TextStyle{Monospace: true} 
	
	// Input Styling: Remove border if possible or make it look clean. 
	// Fyne's Entry is standard. We can wrap it.

	currentIdx := 0

	update := func(text string) {
		originalText := text
		if strings.Contains(text, "\n") {
			text = strings.ReplaceAll(text, "\n", "")
			input.SetText(text)
			input.CursorColumn = len([]rune(text))
		}

		cleanInput := text 
		targetLine := targetLines[currentIdx]

		// --- Rich Text Coloring Logic ---
		var segments []widget.RichTextSegment
		inputRunes := []rune(cleanInput)
		targetRunes := []rune(targetLine)

		var currentSegmentText strings.Builder
		var currentColor fyne.ThemeColorName

		flushSegment := func() {
			if currentSegmentText.Len() > 0 {
				segments = append(segments, &widget.TextSegment{
					Text: currentSegmentText.String(),
					Style: widget.RichTextStyle{
						// CRITICAL FIX: Do NOT set Alignment here. It breaks the line.
						SizeName:  theme.SizeNameHeadingText,
						ColorName: currentColor,
						TextStyle: fyne.TextStyle{Monospace: true}, // Align with input
					},
				})
				currentSegmentText.Reset()
			}
		}

		for i, r := range targetRunes {
			var nextColor fyne.ThemeColorName
			if i < len(inputRunes) {
				if inputRunes[i] == r {
					nextColor = theme.ColorNameSuccess
				} else {
					nextColor = theme.ColorNameError
				}
			} else {
				nextColor = theme.ColorNameForeground
			}

			if i == 0 {
				currentColor = nextColor
			}

			if nextColor != currentColor {
				flushSegment()
				currentColor = nextColor
			}
			currentSegmentText.WriteRune(r)
		}
		flushSegment()

		currentTarget.Segments = segments
		currentTarget.Refresh()

		// Check completion
		trimmedInput := strings.TrimSpace(cleanInput)
		trimmedTarget := strings.TrimSpace(targetLine)

		if trimmedInput == trimmedTarget {
			currentLabel.SetText(fmt.Sprintf("LEVEL %d / %d - PRESS [SPACE] or [ENTER]", currentIdx+1, len(targetLines)))
			
			isSpaceTrigger := strings.HasSuffix(text, " ")
			isEnterTrigger := strings.Contains(originalText, "\n")

			if isSpaceTrigger || isEnterTrigger {
				advance(&currentIdx, targetLines, currentLabel, input, setTargetText)
			}
		} else {
			currentLabel.SetText(fmt.Sprintf("LEVEL %d / %d", currentIdx+1, len(targetLines)))
		}
	}

	input.OnChanged = update

	reset := widget.NewButton("RESET", func() {
		input.SetText("")
		currentIdx = 0
		currentLabel.SetText(fmt.Sprintf("LEVEL 1 / %d", len(targetLines)))
		setTargetText(targetLines[0])
		update("")
	})

	// --- Iconic Layout ---

	// A clean separator line
	separator := canvas.NewRectangle(theme.DefaultTheme().Color(theme.ColorNameForeground, theme.VariantDark))
	separator.SetMinSize(fyne.NewSize(0, 1))

	// The Typing Area
	typingArea := container.NewVBox(
		layout.NewSpacer(),
		container.NewCenter(currentTarget), // Centered Text
		container.NewPadded(separator),     // Decorative Line
		input,                              // Input Field
		layout.NewSpacer(),
	)

	// Background Card (Dark Panel)
	bg := canvas.NewRectangle(theme.DefaultTheme().Color(theme.ColorNameInputBackground, theme.VariantDark))
	bg.CornerRadius = 16
	
	// Composite Panel
	panel := container.NewStack(
		bg,
		container.NewPadded(typingArea),
	)

	// Main Layout
	mainContainer := container.NewBorder(
		container.NewVBox(currentLabel, widget.NewSeparator()), // Top
		container.NewVBox(widget.NewSeparator(), reset),        // Bottom
		nil, nil, // Left, Right
		container.NewPadded(panel), // Center
	)

	w.SetContent(container.NewPadded(mainContainer))
	
	update("")
	w.ShowAndRun()
}
func advance(currentIdx *int, targetLines []string, currentLabel *widget.Label, input *widget.Entry, setTargetText func(string)) {
	if *currentIdx+1 >= len(targetLines) {
		// Completed all lines.
		input.SetText("")
		currentLabel.SetText("완료! (모든 문장 입력)")
		setTargetText(targetLines[len(targetLines)-1])
		return
	}
	*currentIdx++
	input.SetText("")
	currentLabel.SetText(fmt.Sprintf("현재 문장 (%d/%d)", *currentIdx+1, len(targetLines)))
	setTargetText(targetLines[*currentIdx])
}

type fontTheme struct {
	base    fyne.Theme
	regular []byte
}

func normalizeLines(text string) []string {
	raw := strings.Split(text, "\n")
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func (f *fontTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	return f.base.Color(n, v)
}

func (f *fontTheme) Font(s fyne.TextStyle) fyne.Resource {
	if len(f.regular) == 0 {
		return f.base.Font(s)
	}
	return fyne.NewStaticResource("NanumGothic-Regular.ttf", f.regular)
}

func (f *fontTheme) Icon(n fyne.ThemeIconName) fyne.Resource {
	return f.base.Icon(n)
}

func (f *fontTheme) Size(n fyne.ThemeSizeName) float32 {
	return f.base.Size(n)
}
