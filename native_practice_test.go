package main

import (
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
