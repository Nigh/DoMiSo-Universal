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

func (e *Editor) ensurePracticeTimeline() {
	if e.practiceSpans == nil {
		e.practiceSpans = practiceNoteSpans(e.doc.Song)
		e.practiceChords = songChords(e.doc.Song, 0)
	}
}

const practiceTimelineLabel = "练习时间轴；所有轨道音符汇总，不可编辑。拖动定位，滚轮横移，Ctrl 滚轮缩放，Space 播放或暂停"

func (e *Editor) practiceTimeline(c *ui.Context, tick float64, playing bool) {
	e.ensurePracticeTimeline()
	ui.Row(c).Gap(8).Wrap().Children(func() {
		ui.Text(c, "全轨时间轴").FontSize(12).TextColor(palette.Muted)
		ui.Text(c, e.practiceTimelineStatus(tick, playing)).FontSize(12)
		ui.Spacer(c)
		ui.Text(c, "竖线：按键位置 · 拖动定位 · 滚轮横移 · Ctrl + 滚轮缩放").FontSize(12).TextColor(palette.Muted)
	})
	timeline := ui.Box(c).Height(76).Clip().Focusable().FocusRing(false).Label(practiceTimelineLabel).HandleInput(e.practiceTimelineInput)
	timeline.Draw(func(p *ui.Painter, r ui.Rect) {
		e.rollWidth, e.rollHeight = r.W, r.H
		e.drawPracticeTimeline(p, r, tick, playing)
		if timeline.FocusVisible() {
			p.Stroke(ui.Rect{X: r.X + 1, Y: r.Y + 1, W: max(0, r.W-2), H: max(0, r.H-2)}, palette.Selected, 0, 2)
		}
	})
	e.buildFocus = e.buildFocus || timeline.Focused()
}

func (e *Editor) practiceTimelineStatus(tick float64, playing bool) string {
	if len(e.practiceSpans) == 0 {
		return "暂无音符 · 打开曲谱，或切到编辑模式添加音符"
	}
	state := "已暂停"
	if e.audio.Waiting() {
		state = "等待按键"
	} else if playing {
		state = "播放中"
	}
	if tick < 0 {
		state += fmt.Sprintf(" · 预备 %.1f 拍", -tick/float64(e.doc.Song.PPQ))
	}
	if len(e.groups) > 0 && e.group >= len(e.groups) {
		return state + " · 本段练习已完成"
	}
	return state
}

func (e *Editor) drawPracticeTimeline(p *ui.Painter, r ui.Rect, tick float64, playing bool) {
	e.ensurePracticeTimeline()
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
		nextLabelX := r.X + 4
		barsInRange(e.meters, int64(math.Floor(left)), int64(math.Ceil(right)), func(tick, bar int64) {
			x := r.X + float32(tick)*e.zoom - e.scrollX
			p.Line(x, r.Y, x, r.Y+r.H, 1, palette.Grid)
			if x+4 >= nextLabelX {
				label := fmt.Sprint(bar)
				p.Text(x+4, r.Y+4, label, 11, palette.Muted)
				nextLabelX = x + 4 + max(40, float32(len(label))*7+12)
			}
		})
		laneY := r.Y + rollHeaderHeight + 8
		laneHeight := max(float32(1), r.H-rollHeaderHeight-16)
		first := sort.Search(len(e.practiceSpans), func(i int) bool { return float64(e.practiceSpans[i].end) >= left })
		for _, span := range e.practiceSpans[first:] {
			if float64(span.start) > right {
				break
			}
			x := r.X + float32(span.start)*e.zoom - e.scrollX
			end := min(r.X+r.W, max(x+2, r.X+float32(span.end)*e.zoom-e.scrollX))
			x = max(r.X, x)
			if end > x {
				p.Fill(ui.Rect{X: x, Y: laneY, W: end - x, H: laneHeight}, palette.Accent, 3)
			}
		}
		if len(e.practiceSpans) == 0 {
			p.Text(r.X+12, laneY+6, "暂无音符", 12, palette.Muted)
		}
		firstChord := sort.Search(len(e.practiceChords), func(i int) bool { return float64(e.practiceChords[i].Tick) >= left })
		for _, chord := range e.practiceChords[firstChord:] {
			if float64(chord.Tick) > right {
				break
			}
			x := r.X + float32(chord.Tick)*e.zoom - e.scrollX
			p.Line(x, laneY-4, x, laneY+laneHeight, 1, palette.Text)
		}
		x := r.X + float32(tick)*e.zoom - e.scrollX
		if x >= r.X && x <= r.X+r.W {
			x = max(r.X+1, min(r.X+r.W-1, x))
			p.Line(x, r.Y, x, r.Y+r.H, 2, palette.Selected)
			p.Fill(ui.Rect{X: x - 3, Y: r.Y, W: 6, H: 5}, palette.Selected, 1)
		}
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
