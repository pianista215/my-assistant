package display

import (
	"image"
	"strings"

	"golang.org/x/image/font"
)

// EventLine is one bulleted, time-labeled entry in a Section.Events list —
// the time drawn bold, the text word-wrapped in the regular weight.
type EventLine struct {
	Time string
	Text string
}

// eventTimeGap is the gap between an EventLine's bold time label and its
// wrapped text.
const eventTimeGap = 8

// drawEventLines renders events as a bulleted list, like drawBulletedLines,
// but for content that needs real word-wrapping instead of packing
// several short items onto one line: each entry gets a bullet, its Time
// in bold, then Text in the regular weight, wrapped within width with
// continuation lines hanging-indented under where Text started — so a
// long summary stays inside its column instead of spilling into the next
// one (drawText itself still doesn't wrap or clip on its own; this is
// what replaces the unbounded-overflow behavior for the agenda
// specifically). Returns the y immediately below the last line drawn.
func drawEventLines(canvas *image.Gray, boldFace, regularFace font.Face, x, y, width int, events []EventLine) int {
	for _, ev := range events {
		drawBullet(canvas, boldFace, x, y)
		drawText(canvas, boldFace, ev.Time, x+bulletUnitWidth, y)
		indent := x + bulletUnitWidth + measureWidth(boldFace, ev.Time) + eventTimeGap

		lines := wrapText(regularFace, ev.Text, x+width-indent)
		if len(lines) == 0 {
			y += rowHeight
			continue
		}
		drawText(canvas, regularFace, lines[0], indent, y)
		for _, line := range lines[1:] {
			y += rowHeight
			drawText(canvas, regularFace, line, indent, y)
		}
		y += rowHeight
	}
	return y
}

// wrapText greedily packs text's words onto lines no wider than maxWidth.
// A single word wider than maxWidth on its own is left unsplit rather
// than broken mid-word — a rare, accepted residual case for calendar
// summaries, much narrower in practice than the unbounded overflow this
// replaces.
func wrapText(face font.Face, text string, maxWidth int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}

	lines := make([]string, 0, 1)
	current := words[0]
	for _, word := range words[1:] {
		candidate := current + " " + word
		if measureWidth(face, candidate) > maxWidth {
			lines = append(lines, current)
			current = word
			continue
		}
		current = candidate
	}
	return append(lines, current)
}
