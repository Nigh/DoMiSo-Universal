package main

import (
	"fmt"
	"math"
	"sort"

	"github.com/egoist/mygo/ui"
)

// practiceSpan records note presence, without pitch, track or selection state.
type practiceSpan struct {
	start, end int64
}

func practiceNoteSpans(song Song) []practiceSpan {
	spans := make([]practiceSpan, 0)
	for _, track := range song.Tracks {
		for _, note := range track.Notes {
			spans = append(spans, practiceSpan{note.Tick, note.Tick + note.Duration})
		}
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
	merged := spans[:0]
	for _, span := range spans {
		if len(merged) > 0 && span.start <= merged[len(merged)-1].end {
			merged[len(merged)-1].end = max(merged[len(merged)-1].end, span.end)
		} else {
			merged = append(merged, span)
		}
	}
	return merged
}

func (e *Editor) practiceTimeline(c *ui.Context, tick float64, playing bool) {
	timeline := ui.Box(c).Height(76).Clip().Focusable().Label("练习时间轴；显示小节与所有轨道音符，拖动定位，滚轮横移，Ctrl 滚轮缩放").HandleInput(e.practiceTimelineInput).Draw(func(p *ui.Painter, r ui.Rect) {
		e.rollWidth, e.rollHeight = r.W, r.H
		e.drawPracticeTimeline(p, r, tick, playing)
	})
	e.buildFocus = e.buildFocus || timeline.Focused()
}

func (e *Editor) drawPracticeTimeline(p *ui.Painter, r ui.Rect, tick float64, playing bool) {
	if e.practiceSpans == nil {
		e.practiceSpans = practiceNoteSpans(e.doc.Song)
	}
	e.ensureRollIndex()
	if playing && e.drag == "" {
		head := float32(tick)*e.zoom - e.scrollX
		if e.follow || head < 0 || head >= r.W {
			e.scrollX = float32(tick)*e.zoom - r.W/2
		}
	}
	e.scrollX = max(-float32(e.doc.Song.BarTicks()*2)*e.zoom, e.scrollX)
	p.Fill(r, palette.Background, 0)
	p.Clip(r, 0, func() {
		p.Fill(ui.Rect{X: r.X, Y: r.Y, W: r.W, H: rollHeaderHeight}, palette.Surface, 0)
		left, right := float64(e.scrollX/e.zoom), float64((e.scrollX+r.W)/e.zoom)
		barsInRange(e.meters, int64(math.Floor(left)), int64(math.Ceil(right)), func(tick, bar int64) {
			x := r.X + float32(tick)*e.zoom - e.scrollX
			p.Line(x, r.Y, x, r.Y+r.H, 1, palette.Grid)
			p.Text(x+4, r.Y+4, fmt.Sprint(bar), 11, palette.Muted)
		})
		laneY := r.Y + rollHeaderHeight + 8
		laneHeight := max(float32(1), r.H-rollHeaderHeight-16)
		first := sort.Search(len(e.practiceSpans), func(i int) bool { return float64(e.practiceSpans[i].end) >= left })
		for _, span := range e.practiceSpans[first:] {
			if float64(span.start) > right {
				break
			}
			x := r.X + float32(span.start)*e.zoom - e.scrollX
			p.Fill(ui.Rect{X: x, Y: laneY, W: max(2, float32(span.end-span.start)*e.zoom), H: laneHeight}, palette.Accent, 3)
		}
		x := r.X + float32(tick)*e.zoom - e.scrollX
		p.Line(x, r.Y, x, r.Y+r.H, 2, palette.Selected)
		p.Line(r.X, r.Y+rollHeaderHeight, r.X+r.W, r.Y+rollHeaderHeight, 1, palette.Grid)
		p.Stroke(r, palette.Grid, 0, 1)
	})
}

func (e *Editor) practiceTimelineInput(ev ui.InputEvent) bool {
	if e.busy {
		return true
	}
	if ev.Kind == ui.InputKeyDown || ev.Kind == ui.InputKeyUp {
		return e.keyInput(ev)
	}
	switch ev.Kind {
	case ui.InputScroll:
		if ev.Mods&ui.Ctrl != 0 {
			tick := (ev.X + e.scrollX) / e.zoom
			e.zoom = max(0.02, min(4, e.zoom*float32(math.Exp(-float64(ev.DY)/250))))
			e.scrollX = tick*e.zoom - ev.X
		} else {
			e.scrollX += ev.DX + ev.DY
		}
		e.scrollX = max(-float32(e.doc.Song.BarTicks()*2)*e.zoom, e.scrollX)
		return true
	case ui.InputPointerDown:
		if ev.Button != 0 {
			return true
		}
		e.drag = "seek"
		e.beginScrub()
		e.seek(float64((ev.X + e.scrollX) / e.zoom))
		return true
	case ui.InputPointerMove:
		if e.drag == "seek" {
			e.seek(float64((ev.X + e.scrollX) / e.zoom))
			return true
		}
	case ui.InputPointerUp:
		if e.drag == "seek" {
			e.finishDrag()
			return true
		}
	}
	return false
}

func keyboardGeometry(r ui.Rect, practice bool) (bounds ui.Rect, unit, rowHeight float32) {
	unit, rowHeight = max(0, min(float32(52), (r.W-8)/15)), 30
	bounds = ui.Rect{X: r.X + 4, Y: r.Y, W: unit * 15, H: rowHeight * 5}
	if practice {
		scale := max(float32(0), min((r.W-16)/780, (r.H-16)/150))
		unit, rowHeight = 52*scale, 30*scale
		bounds.W, bounds.H = unit*15, rowHeight*5
		bounds.X, bounds.Y = r.X+(r.W-bounds.W)/2, r.Y+(r.H-bounds.H)/2
	}
	return
}
