package main

import (
	"bytes"
	"math"
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestPracticeNotePresenceAcrossAllTracks(t *testing.T) {
	song := emptySong()
	song.Tracks[0].Notes = []Note{
		{Tick: 300, Duration: 100, Key: 60},
		{Tick: 0, Duration: 100, Key: 20},
		{Tick: 0, Duration: 50, Key: 108},
	}
	other := newTrack("muted")
	other.Settings.Mute = true
	other.Notes = []Note{{Tick: 50, Duration: 150, Key: 109}, {Tick: 200, Duration: 50, Key: 127}}
	song.Tracks = append(song.Tracks, other, newTrack("empty"))
	before := cloneSong(song)
	want := []practiceSpan{{0, 250}, {300, 400}}
	if got := practiceNoteSpans(song); !reflect.DeepEqual(got, want) {
		t.Fatalf("note presence = %v, want %v", got, want)
	}
	if !reflect.DeepEqual(song, before) {
		t.Fatal("flattening changed MIDI")
	}
	if got := practiceNoteSpans(emptySong()); got == nil || len(got) != 0 {
		t.Fatal("empty song must produce a cacheable empty timeline")
	}
}

func TestPracticeTimelineSeekBoundaries(t *testing.T) {
	e := newEditor()
	e.doc.Song = testSong()
	e.audio.Configure(e.doc.Song)
	e.mode = "练习"
	e.follow = true
	e.scrollX = -float32(e.doc.Song.BarTicks()*2) * e.zoom
	e.audio.Play(true)
	before := snapshot(e.doc)
	next := e.audio.next
	e.practiceTimelineInput(ui.InputEvent{Kind: ui.InputPointerDown, X: -100, Y: 50})
	if tick, _ := e.audio.State(); tick != -float64(e.doc.Song.BarTicks()*2) {
		t.Fatalf("negative pre-roll boundary = %g", tick)
	}
	e.practiceTimelineInput(ui.InputEvent{Kind: ui.InputPointerMove, X: 100000, Y: 50})
	if tick, _ := e.audio.State(); tick != float64(e.doc.Song.End()) {
		t.Fatalf("song-end boundary = %g", tick)
	}
	if e.audio.next != next {
		t.Fatal("drag restored MIDI before release")
	}
	e.practiceTimelineInput(ui.InputEvent{Kind: ui.InputPointerUp})
	if _, playing := e.audio.State(); !playing || e.drag != "" || !e.follow {
		t.Fatal("scrub lost playback or follow state")
	}
	if !reflect.DeepEqual(e.doc, before) || len(e.undo) != 0 {
		t.Fatal("timeline seek edited document")
	}
	e.audio.Play(false)
	e.practiceTimelineInput(ui.InputEvent{Kind: ui.InputPointerDown, X: 0, Y: 10, Mods: ui.Ctrl, Clicks: 2})
	e.practiceTimelineInput(ui.InputEvent{Kind: ui.InputPointerUp})
	if _, playing := e.audio.State(); playing || e.loopStart != 0 {
		t.Fatal("modified click resumed pause or edited loop")
	}
}

func TestPracticeTimelineFollowAndPage(t *testing.T) {
	for _, follow := range []bool{false, true} {
		e := newEditor()
		e.follow = follow
		var tick float64
		tester := ui.NewTester(func(c *ui.Context) {
			ui.Box(c).Grow(1).Draw(func(p *ui.Painter, r ui.Rect) {
				e.drawPracticeTimeline(p, r, tick, true)
			})
		}, 400, 76)
		for _, next := range []float64{3000, -3500, 0} {
			tick = next
			tester.Frame()
			head := float32(tick)*e.zoom - e.scrollX
			if head < 0 || head >= 400 {
				t.Fatalf("follow=%v tick=%g playhead=%g", follow, tick, head)
			}
			if e.scrollX < -float32(e.doc.Song.BarTicks()*2)*e.zoom || e.follow != follow {
				t.Fatal("paging lost pre-roll boundary or changed follow")
			}
		}
	}
}

func TestPracticeTimelineClipsSpansAndShowsBoundaryHead(t *testing.T) {
	e := newEditor()
	e.doc.Song.Tracks[0].Notes = []Note{{Tick: 0, Duration: 100000, Key: 60}}
	e.scrollX = 100
	tick := float64(0)
	tester := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Grow(1).Draw(func(p *ui.Painter, r ui.Rect) {
			p.Fill(r, palette.Surface, 0)
			e.drawPracticeTimeline(p, ui.Rect{X: 20, Y: 20, W: 160, H: 76}, tick, false)
		})
	}, 200, 116)
	check := func(x, y int, color ui.Color) {
		t.Helper()
		got := tester.Image().RGBAAt(x, y)
		if got.R != color.R || got.G != color.G || got.B != color.B || got.A != color.A {
			t.Fatalf("pixel (%d, %d) = %v, want %v", x, y, got, color)
		}
	}
	for _, zoom := range []float32{0.02, 0.25, 4} {
		e.zoom = zoom
		tester.Frame()
		check(22, 65, palette.Accent)
		check(178, 65, palette.Accent)
		check(18, 65, palette.Surface)
		check(182, 65, palette.Surface)
		check(50, 22, palette.Surface)
	}
	e.zoom = 1
	e.practiceSpans = []practiceSpan{{0, 100}, {260, 300}}
	tester.Frame()
	check(22, 65, palette.Background)
	check(178, 65, palette.Background)
	for _, edge := range []struct {
		tick float64
		x    int
	}{{100, 21}, {260, 178}} {
		tick = edge.tick
		tester.Frame()
		check(edge.x, 40, palette.Selected)
	}
}

func TestPracticeTimelineFocusRoutesLiveKeys(t *testing.T) {
	e := newEditor()
	e.doc.Song = testSong()
	if err := e.audio.Configure(e.doc.Song); err != nil {
		t.Fatal(err)
	}
	e.mode, e.practice = "练习", true
	e.practiceFrame()
	before := snapshot(e.doc)
	tester := ui.NewTester(func(c *ui.Context) {
		e.buildFocus = false
		ui.Column(c).Fill().Gap(4).Children(func() { e.practiceTimeline(c, 0, false) })
	}, 640, 200)
	tester.Key(0, ui.KeyTab)
	if !e.buildFocus {
		t.Fatal("timeline did not receive keyboard focus")
	}
	tester.KeyDown(ui.KeyA)
	if !e.pressed[60] {
		t.Fatal("focused timeline did not route live note")
	}
	tester.KeyUp(ui.KeyA)
	tester.Key(0, ui.KeySpace)
	if _, playing := e.audio.State(); !playing {
		t.Fatal("focused timeline did not route playback shortcut")
	}
	e.audio.Play(false)
	tester.SetScale(2)
	timeline, ok := tester.Find(practiceTimelineLabel)
	if !ok || timeline.X < 0 || timeline.Y < 0 || timeline.X+timeline.W > 640 || timeline.Y+timeline.H > 200 {
		t.Fatalf("scaled timeline exceeds viewport: %v", timeline)
	}
	if !reflect.DeepEqual(e.doc, before) || len(e.undo) != 0 || len(e.pressed) != 0 {
		t.Fatal("focused timeline edited or recorded document")
	}
}

func TestPracticeKeyboardFitsAndCenters(t *testing.T) {
	for _, size := range [][2]float32{{1200, 500}, {640, 180}, {1200, 90}, {1, 1}} {
		r := ui.Rect{X: 10, Y: 20, W: size[0], H: size[1]}
		bounds, unit, rowHeight := keyboardGeometry(r, true)
		if bounds.X < r.X || bounds.Y < r.Y || bounds.X+bounds.W > r.X+r.W+0.001 || bounds.Y+bounds.H > r.Y+r.H+0.001 {
			t.Fatalf("keyboard exceeds viewport: %v in %v", bounds, r)
		}
		if math.Abs(float64(bounds.X+bounds.W/2-r.X-r.W/2)) > 0.001 || math.Abs(float64(bounds.Y+bounds.H/2-r.Y-r.H/2)) > 0.001 {
			t.Fatalf("keyboard not centered: %v in %v", bounds, r)
		}
		if unit < 0 || rowHeight < 0 || bounds.W != 15*unit || bounds.H != 5*rowHeight {
			t.Fatal("invalid keyboard geometry")
		}
		if size[0] == 1200 && size[1] == 500 && unit <= 52 {
			t.Fatal("practice keyboard did not enlarge")
		}
	}
}

func TestPracticeKeyboardNeverRecordsOrEdits(t *testing.T) {
	e := newEditor()
	e.doc.Song = testSong()
	e.audio.Configure(e.doc.Song)
	e.mode, e.practice, e.recording = "练习", true, true
	e.selected[1] = true
	e.clipboard = []Note{e.doc.Song.Tracks[0].Notes[0]}
	e.undo = []Document{{Song: emptySong()}}
	before := snapshot(e.doc)
	e.audio.SeekTick(120)
	e.keyInput(ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyA})
	if !e.pressed[60] {
		t.Fatal("practice key did not reach shared note input")
	}
	e.audio.SeekTick(240)
	e.keyInput(ui.InputEvent{Kind: ui.InputKeyUp, Key: ui.KeyA})
	e.keyInput(ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyDelete})
	e.keyInput(ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyV, Mods: ui.Ctrl})
	e.keyInput(ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyZ, Mods: ui.Ctrl})
	e.addRecorded(60, 120, 240)
	if !reflect.DeepEqual(e.doc, before) || len(e.recordStart) != 0 || len(e.undo) != 1 || len(e.pressed) != 0 {
		t.Fatal("practice keyboard recorded or edited MIDI")
	}
}

func TestWaitPracticeKeyboardOutlinesAdvanceOnChordPress(t *testing.T) {
	e := newEditor()
	e.doc.Song = emptySong()
	e.doc.Song.Tracks[0].Notes = []Note{
		{ID: 1, Tick: 0, Duration: 480, Key: 60, Velocity: 100},
		{ID: 2, Tick: 0, Duration: 480, Key: 64, Velocity: 100},
		{ID: 3, Tick: 480, Duration: 480, Key: 62, Velocity: 100},
		{ID: 4, Tick: 960, Duration: 480, Key: 62, Velocity: 100},
	}
	if err := e.audio.Configure(e.doc.Song); err != nil {
		t.Fatal(err)
	}
	e.mode, e.practice, e.waitMode = "练习", true, true
	e.practiceFrame()
	tester := ui.NewTester(func(c *ui.Context) { e.keyboard(c, 0) }, 780, 220)
	tester.Key(0, ui.KeyTab)
	checkOutline := func(column float32, want bool) {
		t.Helper()
		bounds, width, height := keyboardGeometry(ui.Rect{W: 780, H: 220}, true)
		scale := height / 30
		x := int(bounds.X + (column+0.5)*width)
		y := int(bounds.Y + 3*scale + 2*height)
		found := false
		for dy := -1; dy <= 2; dy++ {
			pixel := tester.Image().RGBAAt(x, y+dy)
			if pixel.R == palette.Accent.R && pixel.G == palette.Accent.G && pixel.B == palette.Accent.B {
				found = true
			}
		}
		if found != want {
			t.Fatalf("key column %.2f outline=%v, want %v; target=%d", column, found, want, e.group)
		}
	}
	checkOutline(1.75, true)  // A
	checkOutline(2.75, false) // S
	checkOutline(3.75, true)  // D
	tester.KeyDown(ui.KeyA)
	if e.group != 0 {
		t.Fatal("partial chord advanced target")
	}
	checkOutline(1.75, true)
	checkOutline(3.75, true)
	tester.KeyDown(ui.KeyD)
	if e.group != 1 || !e.pressed[60] || !e.pressed[64] {
		t.Fatal("complete chord did not advance while keys remain held")
	}
	checkOutline(1.75, false)
	checkOutline(3.75, false)
	checkOutline(2.75, true)
	tester.KeyUp(ui.KeyA)
	tester.KeyUp(ui.KeyD)
	tester.KeyDown(ui.KeyS)
	if e.group != 2 {
		t.Fatal("second chord did not advance to repeated chord")
	}
	checkOutline(2.75, true)
	tester.KeyDown(ui.KeyS)
	if e.group != 2 {
		t.Fatal("held key satisfied repeated chord")
	}
	tester.KeyUp(ui.KeyS)
	tester.KeyDown(ui.KeyS)
	if e.group != 3 {
		t.Fatal("fresh press did not complete repeated chord")
	}
	checkOutline(1.75, false)
	checkOutline(2.75, false)
	checkOutline(3.75, false)
}

func TestPracticeTimelineMarksChordStartsWithoutMovingTarget(t *testing.T) {
	e := newEditor()
	e.doc.Song = emptySong()
	e.doc.Song.Tracks[0].Notes = []Note{
		{Tick: 0, Duration: 5000, Key: 60},
		{Tick: 240, Duration: 480, Key: 62},
		{Tick: 480, Duration: 480, Key: 64},
	}
	muted := newTrack("muted")
	muted.Settings.Mute = true
	muted.Notes = []Note{{Tick: 720, Duration: 480, Key: 65}}
	e.doc.Song.Tracks = append(e.doc.Song.Tracks, muted)
	e.groups = songChords(e.doc.Song, 0)
	e.zoom = 0.25
	tester := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Fill().Draw(func(p *ui.Painter, r ui.Rect) {
			e.drawPracticeTimeline(p, r, -480, false)
		})
	}, 400, 76)
	for _, x := range []int{60, 120} {
		pixel := tester.Image().RGBAAt(x, 40)
		if pixel.G < 190 || pixel.B < 190 {
			t.Fatalf("missing chord-start line inside sustained span at x=%d: %v", x, pixel)
		}
	}
	if pixel := tester.Image().RGBAAt(180, 40); pixel.G != palette.Accent.G {
		t.Fatalf("muted track introduced a required-key marker: %v", pixel)
	}
	before := append([]byte(nil), tester.Image().Pix...)
	e.group = 1
	tester.Frame()
	if !bytes.Equal(before, tester.Image().Pix) {
		t.Fatal("timeline still moves a target indicator when chord advances")
	}
	e.doc.Song.Tracks[0].Settings.Mute = true
	e.changed(false)
	tester.Frame()
	if pixel := tester.Image().RGBAAt(60, 40); pixel.G != palette.Accent.G {
		t.Fatalf("muting retained a stale required-key marker: %v", pixel)
	}
}

func TestPracticeKeysFlashOnlyFromInput(t *testing.T) {
	e := newEditor()
	e.doc.Song = emptySong()
	e.doc.Song.Tracks[0].Notes = []Note{
		{ID: 1, Tick: 0, Duration: 480, Key: 60, Velocity: 100},
		{ID: 2, Tick: 480, Duration: 480, Key: 62, Velocity: 100},
	}
	if err := e.audio.Configure(e.doc.Song); err != nil {
		t.Fatal(err)
	}
	e.mode, e.practice, e.waitMode = "练习", true, true
	e.practiceFrame()
	e.audio.Play(true)
	tick := 0.0
	tester := ui.NewTester(func(c *ui.Context) { e.keyboard(c, tick) }, 780, 220)
	checkFill := func(column float32, want bool) {
		t.Helper()
		bounds, width, height := keyboardGeometry(ui.Rect{W: 780, H: 220}, true)
		scale := height / 30
		x := int(bounds.X + (column+0.5)*width)
		y := int(bounds.Y + (3+26-5)*scale + 2*height)
		pixel := tester.Image().RGBAAt(x, y)
		colored := pixel.R != palette.Surface.R || pixel.G != palette.Surface.G || pixel.B != palette.Surface.B
		if colored != want {
			t.Fatalf("key column %.2f fill=%v, want input feedback=%v at tick %g", column, pixel, want, tick)
		}
	}
	// Before the first audio read, the display already reaches the gated note.
	checkFill(1.75, false)
	pcm := make([]byte, 4096)
	e.audio.Read(pcm)
	if !e.audio.Waiting() || !bytes.Equal(pcm, make([]byte, len(pcm))) {
		t.Fatal("first practice target sounded before input")
	}
	tester.Frame()
	checkFill(1.75, false)
	tester.Key(0, ui.KeyTab)
	tester.KeyDown(ui.KeyA)
	checkFill(1.75, true)
	tester.KeyUp(ui.KeyA)
	// Stop this read exactly at the next target, before its gate is consumed.
	e.audio.Read(make([]byte, sampleRate/2*8))
	tick, _ = e.audio.State()
	if tick != 480 || e.group != 1 {
		t.Fatalf("did not reach next unjudged chord: tick=%g, group=%d", tick, e.group)
	}
	tester.Frame()
	checkFill(2.75, false)
	e.audio.Read(pcm)
	tester.Frame()
	checkFill(2.75, false)
	// Free practice also uses input-only flashes; playback keeps note previews.
	e.waitMode = false
	e.audio.Play(true)
	tester.Frame()
	checkFill(2.75, false)
	e.mode = "播放"
	e.changeMode("练习")
	e.audio.Play(false)
	tester.Frame()
	e.audio.Play(true)
	tester.Frame()
	checkFill(2.75, true)
	e.mode = "练习"
	e.changeMode("播放")
	e.practiceFrame()
	tester.Frame()
	checkFill(2.75, false)
}
