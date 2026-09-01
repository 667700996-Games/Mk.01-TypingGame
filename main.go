package main

import (
	_ "embed"
	"fmt"
	"image/color"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

//go:embed fonts/NanumGothic-Regular.ttf
var fontData []byte

const (
	appID    = "github.com/299-792-458/Mk.01-TypingGame"
	appTitle = "애국가 타자연습"
)

func main() {
	a := app.NewWithID(appID)
	a.Settings().SetTheme(&anthemTheme{base: theme.DefaultTheme(), regular: fontData})

	w := a.NewWindow(appTitle)
	w.Resize(fyne.NewSize(1080, 720))
	w.SetMaster()

	mode := practiceModes[0]
	session := NewPracticeSession(lyricsForVerse(mode.Verse))

	brandTitle := widget.NewLabel("애국가")
	brandTitle.TextStyle = fyne.TextStyle{Bold: true}
	brandTitle.Importance = widget.HighImportance
	brandSubtitle := widget.NewLabel("우리의 노랫말을, 한 글자씩 바르게")
	brandSubtitle.Importance = widget.LowImportance

	redMark := canvas.NewRectangle(color.NRGBA{R: 205, G: 45, B: 62, A: 255})
	redMark.SetMinSize(fyne.NewSize(42, 5))
	blueMark := canvas.NewRectangle(color.NRGBA{R: 25, G: 76, B: 145, A: 255})
	blueMark.SetMinSize(fyne.NewSize(42, 5))
	mark := container.NewHBox(redMark, blueMark)

	modeLabels := make([]string, 0, len(practiceModes))
	for _, item := range practiceModes {
		modeLabels = append(modeLabels, item.Label)
	}
	modeSelect := widget.NewSelect(modeLabels, nil)
	modeSelect.PlaceHolder = "연습 범위"
	modeSelect.SetSelected(mode.Label)
	modeBox := container.NewVBox(widget.NewLabel("연습 범위"), modeSelect)

	header := container.NewBorder(
		nil, nil,
		container.NewHBox(mark, container.NewVBox(brandTitle, brandSubtitle)),
		modeBox,
	)

	sectionLabel := widget.NewLabel("")
	sectionLabel.TextStyle = fyne.TextStyle{Bold: true}
	sectionLabel.Importance = widget.HighImportance
	countLabel := widget.NewLabel("")
	countLabel.Alignment = fyne.TextAlignTrailing
	countLabel.Importance = widget.LowImportance

	progress := widget.NewProgressBar()
	progress.Min = 0
	progress.Max = 1

	target := widget.NewRichText()
	target.Wrapping = fyne.TextWrapWord
	target.ParseMarkdown("")

	input := widget.NewEntry()
	input.SetPlaceHolder("위 문장을 그대로 입력하세요")

	feedbackIcon := widget.NewLabel("●")
	feedbackIcon.Importance = widget.LowImportance
	feedback := widget.NewLabel("첫 글자를 입력하면 기록이 시작됩니다")
	feedback.Importance = widget.LowImportance
	keyboardHint := widget.NewLabel("문장을 완성한 뒤  Enter ↵")
	keyboardHint.Alignment = fyne.TextAlignTrailing
	keyboardHint.Importance = widget.LowImportance

	accuracyValue, accuracyCard := metricCard("정확도")
	speedValue, speedCard := metricCard("타수 / 분")
	timeValue, timeCard := metricCard("경과 시간")
	comboValue, comboCard := metricCard("연속 정타")
	metrics := container.NewGridWithColumns(4, accuracyCard, speedCard, timeCard, comboCard)

	quoteMark := widget.NewLabel("“")
	quoteMark.Importance = widget.HighImportance
	quoteMark.TextStyle = fyne.TextStyle{Bold: true}

	cardBackground := canvas.NewRectangle(color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	cardBackground.CornerRadius = 18
	cardBackground.StrokeColor = color.NRGBA{R: 218, G: 216, B: 210, A: 255}
	cardBackground.StrokeWidth = 1

	practiceCard := container.NewStack(
		cardBackground,
		container.NewPadded(container.NewVBox(
			container.NewBorder(nil, nil, sectionLabel, countLabel),
			progress,
			layout.NewSpacer(),
			container.NewBorder(nil, nil, quoteMark, nil, container.NewPadded(target)),
			layout.NewSpacer(),
			input,
			container.NewBorder(nil, nil, container.NewHBox(feedbackIcon, feedback), keyboardHint),
		)),
	)
	practiceCard.Resize(fyne.NewSize(0, 330))

	restartButton := widget.NewButton("처음부터", nil)
	restartButton.Importance = widget.LowImportance
	footerNote := widget.NewLabel("공백까지 노랫말과 같아야 다음 소절로 넘어갑니다")
	footerNote.Importance = widget.LowImportance
	footer := container.NewBorder(nil, nil, footerNote, restartButton)

	mainContent := container.NewBorder(
		container.NewVBox(header, widget.NewSeparator()),
		container.NewVBox(widget.NewSeparator(), footer),
		nil, nil,
		container.NewVBox(layout.NewSpacer(), practiceCard, metrics, layout.NewSpacer()),
	)
	page := container.NewPadded(mainContent)

	resultTitle := widget.NewLabel("애국가 완주")
	resultTitle.Alignment = fyne.TextAlignCenter
	resultTitle.TextStyle = fyne.TextStyle{Bold: true}
	resultTitle.Importance = widget.HighImportance
	resultMessage := widget.NewLabel("")
	resultMessage.Alignment = fyne.TextAlignCenter
	resultMessage.Wrapping = fyne.TextWrapWord
	resultStats := widget.NewLabel("")
	resultStats.Alignment = fyne.TextAlignCenter
	resultStats.TextStyle = fyne.TextStyle{Bold: true}
	retryButton := widget.NewButton("같은 범위 다시 연습", nil)
	retryButton.Importance = widget.HighImportance
	closeResultButton := widget.NewButton("결과 닫기", nil)

	resultBackground := canvas.NewRectangle(color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	resultBackground.CornerRadius = 20
	resultBackground.SetMinSize(fyne.NewSize(500, 300))
	resultPanel := container.NewStack(
		resultBackground,
		container.NewPadded(container.NewVBox(
			markForResult(),
			resultTitle,
			resultMessage,
			widget.NewSeparator(),
			resultStats,
			layout.NewSpacer(),
			container.NewGridWithColumns(2, closeResultButton, retryButton),
		)),
	)
	overlayShade := canvas.NewRectangle(color.NRGBA{R: 15, G: 25, B: 44, A: 218})
	resultOverlay := container.NewStack(overlayShade, container.NewCenter(resultPanel))
	resultOverlay.Hide()

	root := container.NewStack(page, resultOverlay)
	w.SetContent(root)

	var suppressInputChange bool

	setInputText := func(value string) {
		suppressInputChange = true
		input.SetText(value)
		suppressInputChange = false
	}

	refresh := func(now time.Time) {
		line := session.CurrentLine()
		stats := session.Stats(now)

		sectionLabel.SetText(fmt.Sprintf("제 %d절  ·  %s", line.Verse, line.Part))
		countLabel.SetText(fmt.Sprintf("%02d / %02d", session.Index+1, len(session.Lines)))
		progress.SetValue(stats.Progress)
		setTargetSegments(target, line.Text, session.Input)

		accuracyValue.SetText(fmt.Sprintf("%.0f%%", stats.Accuracy))
		speedValue.SetText(fmt.Sprintf("%d타", stats.CharactersPM))
		timeValue.SetText(formatDuration(stats.Elapsed))
		comboValue.SetText(fmt.Sprintf("%d자", stats.Combo))

		wrong := mismatchCount(session.Input, line.Text)
		switch {
		case session.Input == line.Text:
			feedbackIcon.Importance = widget.SuccessImportance
			feedback.Importance = widget.SuccessImportance
			feedback.SetText("정확합니다. Enter를 눌러 다음 소절로 이동하세요")
		case wrong > 0:
			feedbackIcon.Importance = widget.DangerImportance
			feedback.Importance = widget.DangerImportance
			feedback.SetText(fmt.Sprintf("현재 %d곳이 다릅니다. 붉은 글자를 확인하세요", wrong))
		case session.State == SessionReady:
			feedbackIcon.Importance = widget.LowImportance
			feedback.Importance = widget.LowImportance
			feedback.SetText("첫 글자를 입력하면 기록이 시작됩니다")
		default:
			remaining := len([]rune(line.Text)) - len([]rune(session.Input))
			feedbackIcon.Importance = widget.LowImportance
			feedback.Importance = widget.LowImportance
			feedback.SetText(fmt.Sprintf("좋습니다. %d자 남았습니다", remaining))
		}
		feedbackIcon.Refresh()
		feedback.Refresh()
	}

	showResult := func(now time.Time) {
		stats := session.Stats(now)
		resultTitle.SetText(fmt.Sprintf("%s 완주", mode.Label))
		resultMessage.SetText("마지막 소절까지 또박또박 완성했습니다.\n오늘의 기록을 기억하고 한 번 더 도전해 보세요.")
		resultStats.SetText(fmt.Sprintf("정확도 %.0f%%    ·    %d타/분    ·    %s    ·    최고 연속 %d자",
			stats.Accuracy, stats.CharactersPM, formatDuration(stats.Elapsed), stats.BestCombo))
		input.Disable()
		resultOverlay.Show()
		resultOverlay.Refresh()
	}

	resetSession := func() {
		session = NewPracticeSession(lyricsForVerse(mode.Verse))
		setInputText("")
		input.Enable()
		resultOverlay.Hide()
		refresh(time.Now())
		w.Canvas().Focus(input)
	}

	restartButton.OnTapped = resetSession
	retryButton.OnTapped = resetSession
	closeResultButton.OnTapped = func() {
		resultOverlay.Hide()
	}

	modeSelect.OnChanged = func(selected string) {
		for _, candidate := range practiceModes {
			if candidate.Label == selected {
				mode = candidate
				resetSession()
				return
			}
		}
	}

	input.OnChanged = func(value string) {
		if suppressInputChange {
			return
		}
		now := time.Now()
		session.UpdateInput(value, now)
		refresh(now)
	}
	input.OnSubmitted = func(_ string) {
		now := time.Now()
		switch session.Submit(now) {
		case SubmitAdvanced:
			setInputText("")
			refresh(now)
			w.Canvas().Focus(input)
		case SubmitFinished:
			refresh(now)
			showResult(now)
		case SubmitIncomplete:
			refresh(now)
		}
	}

	refresh(time.Now())
	w.Canvas().Focus(input)
	w.ShowAndRun()
}

func metricCard(label string) (*widget.Label, fyne.CanvasObject) {
	value := widget.NewLabel("—")
	value.TextStyle = fyne.TextStyle{Bold: true}
	value.Alignment = fyne.TextAlignCenter
	value.Importance = widget.HighImportance

	caption := widget.NewLabel(label)
	caption.Alignment = fyne.TextAlignCenter
	caption.Importance = widget.LowImportance

	background := canvas.NewRectangle(color.NRGBA{R: 235, G: 238, B: 242, A: 255})
	background.CornerRadius = 12
	return value, container.NewStack(background, container.NewPadded(container.NewVBox(value, caption)))
}

func markForResult() fyne.CanvasObject {
	red := canvas.NewRectangle(color.NRGBA{R: 205, G: 45, B: 62, A: 255})
	red.SetMinSize(fyne.NewSize(50, 5))
	blue := canvas.NewRectangle(color.NRGBA{R: 25, G: 76, B: 145, A: 255})
	blue.SetMinSize(fyne.NewSize(50, 5))
	return container.NewCenter(container.NewHBox(red, blue))
}

func setTargetSegments(target *widget.RichText, targetText, input string) {
	targetRunes := []rune(targetText)
	inputRunes := []rune(input)
	segments := make([]widget.RichTextSegment, 0, 4)

	appendSegment := func(text string, colorName fyne.ThemeColorName) {
		if text == "" {
			return
		}
		segments = append(segments, &widget.TextSegment{
			Text: text,
			Style: widget.RichTextStyle{
				SizeName:  theme.SizeNameSubHeadingText,
				ColorName: colorName,
				TextStyle: fyne.TextStyle{Bold: true},
			},
		})
	}

	var builder strings.Builder
	currentColor := theme.ColorNameForeground
	for index, expected := range targetRunes {
		colorName := theme.ColorNameForeground
		if index < len(inputRunes) {
			if inputRunes[index] == expected {
				colorName = theme.ColorNameSuccess
			} else {
				colorName = theme.ColorNameError
			}
		}
		if builder.Len() > 0 && colorName != currentColor {
			appendSegment(builder.String(), currentColor)
			builder.Reset()
		}
		currentColor = colorName
		builder.WriteRune(expected)
	}
	appendSegment(builder.String(), currentColor)

	if len(inputRunes) > len(targetRunes) {
		appendSegment(string(inputRunes[len(targetRunes):]), theme.ColorNameError)
	}
	target.Segments = segments
	target.Refresh()
}

func formatDuration(duration time.Duration) string {
	totalSeconds := int(duration.Round(time.Second).Seconds())
	if totalSeconds < 0 {
		totalSeconds = 0
	}
	return fmt.Sprintf("%02d:%02d", totalSeconds/60, totalSeconds%60)
}

type anthemTheme struct {
	base    fyne.Theme
	regular []byte
}

func (t *anthemTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return color.NRGBA{R: 244, G: 242, B: 236, A: 255}
	case theme.ColorNameForeground:
		return color.NRGBA{R: 24, G: 34, B: 55, A: 255}
	case theme.ColorNameInputBackground:
		return color.NRGBA{R: 247, G: 248, B: 250, A: 255}
	case theme.ColorNameButton:
		return color.NRGBA{R: 228, G: 232, B: 238, A: 255}
	case theme.ColorNamePrimary:
		return color.NRGBA{R: 26, G: 72, B: 137, A: 255}
	case theme.ColorNameSuccess:
		return color.NRGBA{R: 26, G: 126, B: 84, A: 255}
	case theme.ColorNameError:
		return color.NRGBA{R: 190, G: 48, B: 67, A: 255}
	case theme.ColorNamePlaceHolder, theme.ColorNameDisabled:
		return color.NRGBA{R: 116, G: 123, B: 135, A: 255}
	case theme.ColorNameSeparator:
		return color.NRGBA{R: 212, G: 211, B: 207, A: 255}
	case theme.ColorNameHover:
		return color.NRGBA{R: 218, G: 224, B: 234, A: 255}
	case theme.ColorNameFocus:
		return color.NRGBA{R: 51, G: 100, B: 169, A: 255}
	default:
		return t.base.Color(name, variant)
	}
}

func (t *anthemTheme) Font(_ fyne.TextStyle) fyne.Resource {
	if len(t.regular) == 0 {
		return t.base.Font(fyne.TextStyle{})
	}
	return fyne.NewStaticResource("NanumGothic-Regular.ttf", t.regular)
}

func (t *anthemTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return t.base.Icon(name)
}

func (t *anthemTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNameHeadingText:
		return 32
	case theme.SizeNameSubHeadingText:
		return 22
	case theme.SizeNameText:
		return 15
	case theme.SizeNameInputBorder:
		return 1.5
	default:
		return t.base.Size(name)
	}
}
