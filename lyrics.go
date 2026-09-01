package main

// LyricLine keeps presentation metadata beside each practice sentence. The
// anthem is intentionally split into singable phrases instead of screen chunks.
type LyricLine struct {
	Verse int
	Part  string
	Text  string
}

type PracticeMode struct {
	Label string
	Verse int // 0 means the complete anthem.
}

var practiceModes = []PracticeMode{
	{Label: "전곡 · 1–4절", Verse: 0},
	{Label: "1절 집중", Verse: 1},
	{Label: "2절 집중", Verse: 2},
	{Label: "3절 집중", Verse: 3},
	{Label: "4절 집중", Verse: 4},
}

var anthemLyrics = []LyricLine{
	{Verse: 1, Part: "첫 소절", Text: "동해물과 백두산이 마르고 닳도록"},
	{Verse: 1, Part: "둘째 소절", Text: "하느님이 보우하사 우리나라 만세"},
	{Verse: 1, Part: "후렴", Text: "무궁화 삼천리 화려강산"},
	{Verse: 1, Part: "후렴", Text: "대한 사람 대한으로 길이 보전하세"},
	{Verse: 2, Part: "첫 소절", Text: "남산 위에 저 소나무 철갑을 두른 듯"},
	{Verse: 2, Part: "둘째 소절", Text: "바람서리 불변함은 우리 기상일세"},
	{Verse: 2, Part: "후렴", Text: "무궁화 삼천리 화려강산"},
	{Verse: 2, Part: "후렴", Text: "대한 사람 대한으로 길이 보전하세"},
	{Verse: 3, Part: "첫 소절", Text: "가을 하늘 공활한데 높고 구름 없이"},
	{Verse: 3, Part: "둘째 소절", Text: "밝은 달은 우리 가슴 일편단심일세"},
	{Verse: 3, Part: "후렴", Text: "무궁화 삼천리 화려강산"},
	{Verse: 3, Part: "후렴", Text: "대한 사람 대한으로 길이 보전하세"},
	{Verse: 4, Part: "첫 소절", Text: "이 기상과 이 맘으로 충성을 다하여"},
	{Verse: 4, Part: "둘째 소절", Text: "괴로우나 즐거우나 나라 사랑하세"},
	{Verse: 4, Part: "후렴", Text: "무궁화 삼천리 화려강산"},
	{Verse: 4, Part: "후렴", Text: "대한 사람 대한으로 길이 보전하세"},
}

func lyricsForVerse(verse int) []LyricLine {
	if verse == 0 {
		lines := make([]LyricLine, len(anthemLyrics))
		copy(lines, anthemLyrics)
		return lines
	}

	lines := make([]LyricLine, 0, 4)
	for _, line := range anthemLyrics {
		if line.Verse == verse {
			lines = append(lines, line)
		}
	}
	return lines
}
