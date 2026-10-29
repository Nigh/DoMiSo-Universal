package main

import (
	"github.com/egoist/mygo/ui"
	"gitlab.com/gomidi/midi/v2/smf"
	"math"
	"testing"
)

func TestRollWheelZoomAnchors(t *testing.T) {
	e := newEditor()
	e.doc.Song.Tracks[0].Notes = []Note{{ID: 1, Tick: 20000, Duration: 480, Key: 60, Velocity: 100}}
	tester := ui.NewTester(e.view, 1120, 760)
	e.scrollX, e.scrollY, e.focusRoll = 300, 400, false
	x := rollKeysWidth + 180
	before := e.tickAtX(x)
	oldHeight := e.rowHeight
	e.rollInput(ui.InputEvent{Kind: ui.InputScroll, Mods: ui.Ctrl, X: x, Y: 100, DY: -80})
	tester.Frame()
	if math.Abs(e.tickAtX(x)-before) > 0.01 || e.rowHeight != oldHeight {
		t.Fatalf("horizontal anchor changed: before=%g after=%g", before, e.tickAtX(x))
	}
	center := (e.rollHeight - rollHeaderHeight) / 2
	pitch := (e.scrollY + center) / e.rowHeight
	oldZoom, oldScroll := e.zoom, e.scrollX
	e.rollInput(ui.InputEvent{Kind: ui.InputScroll, Mods: ui.Ctrl | ui.Shift, X: x, Y: 40, DY: -80})
	tester.Frame()
	if math.Abs(float64((e.scrollY+center)/e.rowHeight-pitch)) > 0.0001 || e.zoom != oldZoom || e.scrollX != oldScroll {
		t.Fatal("vertical zoom changed viewport center or horizontal axis")
	}
	e.rollInput(ui.InputEvent{Kind: ui.InputScroll, Mods: ui.Ctrl | ui.Shift, DY: 10000})
	tester.Frame()
	if e.rowHeight != 10 || e.scrollY < 0 {
		t.Fatal("vertical zoom escaped bounds")
	}
}

func TestRollRightScrollKeepsOccupiedBar(t *testing.T) {
	e := newEditor()
	e.doc.Song.Tracks[0].Notes = []Note{{ID: 1, Tick: 100, Duration: 3740, Key: 60, Velocity: 100}}
	e.doc.Song.Tracks[0].Events = []MIDIEvent{{Tick: 1920, Data: smf.MetaMeter(3, 4)}, {Tick: 50000, Data: smf.MetaTempo(120)}}
	// A sustained note ends inside the second 3/4 bar, whose start is tick 3360.
	tester := ui.NewTester(e.view, 1120, 760)
	e.rollInput(ui.InputEvent{Kind: ui.InputScroll, Mods: ui.Shift, DY: 100000})
	tester.Frame()
	if e.scrollX != 3360*e.zoom {
		t.Fatalf("right limit=%g, want=%g", e.scrollX, 3360*e.zoom)
	}
	e.rollInput(ui.InputEvent{Kind: ui.InputScroll, Mods: ui.Shift, DY: -100000})
	tester.Frame()
	if e.scrollX != -float32(e.doc.Song.BarTicks()*2)*e.zoom {
		t.Fatal("lost two-bar preroll limit")
	}
	e.doc.Song.Tracks[0].Notes = nil
	e.rollNotes = nil
	e.rollInput(ui.InputEvent{Kind: ui.InputScroll, Mods: ui.Shift, DY: 100000})
	tester.Frame()
	if e.scrollX != 0 {
		t.Fatal("empty roll scrolls beyond origin")
	}
}

func TestRollGridKeepsPhysicalWidthWhileScrolling(t *testing.T) {
	e := newEditor()
	e.follow, e.focusRoll = false, false
	e.rowHeight, e.zoom = 20, 0.15
	e.doc.Song.Tracks[0].Notes = []Note{{ID: 1, Tick: 20000, Duration: 480, Key: 60, Velocity: 100}}
	tester := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Grow(1).Draw(func(p *ui.Painter, r ui.Rect) {
			e.drawRoll(p, r, 0, false)
		})
	}, 400, 200)
	for _, scale := range []float32{1, 1.25, 1.5, 2} {
		tester.SetScale(scale)
		want := int(math.Round(float64(scale)))
		for phase := range 20 {
			e.scrollX, e.scrollY = 30+float32(phase)/20, 10.125+float32(phase)/20
			tester.Frame()
			img := tester.Image()
			x := int((rollKeysWidth + float32(e.doc.Song.BarTicks())*e.zoom - e.scrollX) * scale)
			barWidth := 0
			for px := x - 4; px <= x+4; px++ {
				pixel := img.RGBAAt(px, int(65*scale))
				if pixel.R == palette.Muted.R && pixel.G == palette.Muted.G && pixel.B == palette.Muted.B {
					barWidth++
				}
			}
			y := int((rollHeaderHeight + e.rowHeight - e.scrollY) * scale)
			rowWidth := 0
			for py := y - 4; py <= y+4; py++ {
				pixel := img.RGBAAt(int(200*scale), py)
				if pixel.R == palette.Grid.R && pixel.G == palette.Grid.G && pixel.B == palette.Grid.B {
					rowWidth++
				}
			}
			if barWidth != want || rowWidth != want {
				t.Fatalf("scale=%g phase=%d: bar/row widths=%d/%d, want %d physical pixels", scale, phase, barWidth, rowWidth, want)
			}
		}
	}
}

func TestRollMinimumHeightLabelsStayInPitchRows(t *testing.T) {
	e := newEditor()
	e.follow, e.focusRoll = false, false
	e.rowHeight, e.zoom = 10, 0.15
	e.scrollX, e.scrollY = -30.375, 425.25
	e.doc.Song.Tracks[0].Notes = []Note{
		{ID: 1, Tick: 100, Duration: 480, Key: 59, Velocity: 100},
		{ID: 2, Tick: 100, Duration: 480, Key: 60, Velocity: 100},
		{ID: 3, Tick: 100, Duration: 480, Key: 61, Velocity: 100},
	}
	tester := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Grow(1).Draw(func(p *ui.Painter, r ui.Rect) {
			e.drawRoll(p, r, 0, false)
		})
	}, 400, 180)
	for _, scale := range []float32{1, 1.25, 1.5, 2} {
		tester.SetScale(scale)
		// Only C4 is visible. Dark key-label ink must stay in its pitch row,
		// including where adjacent white-key shapes overlap.
		img := tester.Image()
		top := int(math.Round(float64((rollHeaderHeight + 48*e.rowHeight - e.scrollY) * scale)))
		bottom := int(math.Round(float64((rollHeaderHeight + 49*e.rowHeight - e.scrollY) * scale)))
		keyInk := 0
		for y := int(rollHeaderHeight*scale) + 2; y < img.Bounds().Dy()-2; y++ {
			for x := int(2 * scale); x < int(19*scale); x++ {
				pixel := img.RGBAAt(x, y)
				if pixel.R == palette.Text.R && pixel.G == palette.Text.G && pixel.B == palette.Text.B ||
					pixel.R == palette.Grid.R && pixel.G == palette.Grid.G && pixel.B == palette.Grid.B {
					continue
				}
				if y < top || y >= bottom {
					t.Fatalf("scale=%g: piano label crosses pitch row at %d,%d", scale, x, y)
				}
				keyInk++
			}
		}
		if keyInk == 0 {
			t.Fatalf("scale=%g: C4 piano label disappeared", scale)
		}
		for _, mode := range []string{"音名", "简谱"} {
			e.label = "隐藏"
			tester.Frame()
			withoutLabels := tester.Image()
			e.label = mode
			tester.Frame()
			img = tester.Image()
			for _, note := range e.doc.Song.Tracks[0].Notes {
				nr := e.noteRect(note)
				y0 := int(math.Round(float64(nr.Y * scale)))
				y1 := int(math.Round(float64((nr.Y + nr.H) * scale)))
				inkRows := 0
				first, last := y1, y0
				for y := y0; y < y1; y++ {
					hasInk := false
					for x := int((nr.X + 3) * scale); x < int((nr.X+nr.W-3)*scale); x++ {
						if img.RGBAAt(x, y) != withoutLabels.RGBAAt(x, y) {
							hasInk = true
						}
					}
					if hasInk {
						inkRows++
						first, last = min(first, y), max(last, y)
					}
				}
				if inkRows < int(3*scale) || math.Abs(float64(first+last+1-y0-y1)) > float64(4*scale) {
					t.Fatalf("scale=%g mode=%s key=%d: unreadable or off-center label, rows=%d bounds=%d..%d note=%d..%d", scale, mode, note.Key, inkRows, first, last, y0, y1)
				}
			}
		}
	}
}
