package main

import (
	"fmt"
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
	"gitlab.com/gomidi/midi/v2/smf"
)

func TestModeSwitchReleasesInputAndPracticeWait(t *testing.T) {
	e := newEditor()
	e.doc.Song = testSong()
	if err := e.audio.Configure(e.doc.Song); err != nil {
		t.Fatal(err)
	}
	e.noteInput("KeyA", true, false)
	e.mode = "练习"
	e.changeMode("编辑")
	if len(e.keyDown) != 0 || len(e.pressed) != 0 || !e.practice {
		t.Fatal("mode switch retained input or failed to enable practice")
	}
	e.waitMode = true
	e.practiceFrame()
	e.audio.Play(true)
	e.audio.Read(make([]byte, 4096))
	if !e.audio.Waiting() {
		t.Fatal("practice did not gate audio")
	}
	e.mode = "播放"
	e.changeMode("练习")
	if e.practice || e.audio.Waiting() {
		t.Fatal("leaving practice retained wait")
	}
}

func TestTransportNavigationAndReadOnlySongInfo(t *testing.T) {
	e := newEditor()
	e.doc.Song = testSong()
	if err := e.audio.Configure(e.doc.Song); err != nil {
		t.Fatal(err)
	}
	tester := ui.NewTester(e.view, 1380, 900)
	for _, step := range []struct {
		label string
		want  float64
	}{
		{"移动到结尾", float64(e.doc.Song.End())},
		{"移动到开头", 0},
		{"往后一小节", float64(e.doc.Song.End())},
		{"往前一小节", float64(e.doc.Song.End() - e.doc.Song.BarTicks())},
	} {
		if err := tester.Click(step.label); err != nil {
			t.Fatal(err)
		}
		tick, _ := e.audio.State()
		if tick != step.want {
			t.Fatalf("%s: tick %v, want %v", step.label, tick, step.want)
		}
	}
	e.mode = "播放"
	e.changeMode("编辑")
	tester.Frame()
	meter, ok := tester.Find("拍号分子")
	if !ok {
		t.Fatal("meter control missing")
	}
	tester.ClickAt(meter.X+5, meter.Y+5)
	tester.Type("7")
	if e.meterNum != 4 || e.doc.Dirty {
		t.Fatal("play mode changed song metadata")
	}
}

func TestTransportNavigationRevealsPausedPlayhead(t *testing.T) {
	for _, mode := range []string{"编辑", "播放", "练习"} {
		for _, follow := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/follow=%v", mode, follow), func(t *testing.T) {
				e := newEditor()
				e.mode, e.follow = mode, follow
				e.doc.Song = emptySong()
				e.doc.Song.Tracks[0].Notes = []Note{{ID: 1, Duration: 40000, Key: 60, Velocity: 100}}
				if err := e.audio.Configure(e.doc.Song); err != nil {
					t.Fatal(err)
				}
				tester := ui.NewTester(func(c *ui.Context) {
					tick, playing := e.transportState()
					ui.Column(c).Children(func() {
						ui.Box(c).Height(200).Draw(func(p *ui.Painter, r ui.Rect) {
							e.rollWidth = r.W
							if mode == "练习" {
								e.drawPracticeTimeline(p, r, tick, playing)
							} else {
								e.drawRoll(p, r, tick, playing)
							}
						})
						e.transportBar(c, tick, playing)
					})
				}, 1200, 400)
				for _, step := range []struct {
					label  string
					want   float64
					scroll float32
				}{
					{"移动到结尾", 40000, 0},
					{"移动到开头", 0, 5000},
					{"往后一小节", 1920, 5000},
					{"往前一小节", 0, 5000},
				} {
					e.scrollX = step.scroll
					if err := tester.Click(step.label); err != nil {
						t.Fatal(err)
					}
					tester.Frame()
					tick, playing := e.transportState()
					width := e.rollWidth
					if mode != "练习" {
						width -= rollKeysWidth
					}
					head := float32(tick)*e.zoom - e.scrollX
					if tick != step.want || playing || head < 0 || head >= width || e.revealPlayhead || e.follow != follow {
						t.Fatalf("%s: tick=%g playing=%v head=%g width=%g pending=%v follow=%v", step.label, tick, playing, head, width, e.revealPlayhead, e.follow)
					}
				}
				e.scrollX = 5000
				tester.Frame()
				if e.scrollX != 5000 {
					t.Fatal("paused frame undid manual scrolling after navigation")
				}
			})
		}
	}
}

func TestPlaybackKeyboardReleasesAtRoundedAudioEnd(t *testing.T) {
	e := newEditor()
	e.mode = "播放"
	e.doc.Song = emptySong()
	e.doc.Song.SetTempo(123)
	e.doc.Song.Tracks[0].Notes = []Note{{ID: 1, Duration: 1920, Key: 60, Velocity: 100}}
	if err := e.audio.Configure(e.doc.Song); err != nil {
		t.Fatal(err)
	}
	e.audio.Play(true)
	tester := ui.NewTester(func(c *ui.Context) {
		tick, _ := e.transportState()
		e.keyboard(c, tick)
	}, 780, 156)
	checkOutline := func(want bool) {
		t.Helper()
		bounds, width, height := keyboardGeometry(ui.Rect{W: 780, H: 156}, false)
		x, y := int(bounds.X+2.25*width), int(bounds.Y+3+2*height)
		found := false
		for dy := -1; dy <= 2; dy++ {
			pixel := tester.Image().RGBAAt(x, y+dy)
			if pixel.R == palette.Accent.R && pixel.G == palette.Accent.G && pixel.B == palette.Accent.B {
				found = true
			}
		}
		if found != want {
			t.Fatalf("playback key outline=%v, want %v", found, want)
		}
	}
	checkOutline(true)
	e.audio.Read(make([]byte, int(e.audio.end+1024)*8))
	tick, playing := e.transportState()
	if playing || tick >= 1920 || tick < 1919 {
		t.Fatalf("expected stopped clock just before note end after sample rounding: tick=%g playing=%v", tick, playing)
	}
	tester.Frame()
	checkOutline(false)
	e.noteInput("KeyA", true, false)
	tester.Frame()
	checkOutline(true)
	e.noteInput("KeyA", false, false)
	tester.Frame()
	checkOutline(false)
}

func TestTransportPositionUsesTempoAndMeterMaps(t *testing.T) {
	e := newEditor()
	e.doc.Song = testSong()
	e.doc.Song.Tracks[0].Notes[2].Duration = 4040
	e.doc.Song.SetMeter(3, 4)
	e.doc.Song.Tracks[0].Events = append(e.doc.Song.Tracks[0].Events,
		MIDIEvent{1440, smf.MetaMeter(6, 8)},
		MIDIEvent{2500, smf.MetaMeter(2, 4)},
	)
	if err := e.audio.Configure(e.doc.Song); err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct {
		tick, seconds float64
		bar           int64
	}{
		{-1441, -1441.0 / 960, -1},
		{-480, -0.5, 0},
		{0, 0, 1},
		{480, 0.5, 1},
		{960, 1.5, 1},
		{1440, 2.5, 2},
		{2499, 0.5 + 2019.0/480, 2},
		{2500, 0.5 + 2020.0/480, 3},
		{3460, 0.5 + 2980.0/480, 4},
	} {
		seconds, total, bar := e.transportPosition(step.tick)
		if math.Abs(seconds-step.seconds) > 1e-9 || bar != step.bar {
			t.Fatalf("tick %g: time %g, bar %d; want %g, %d", step.tick, seconds, bar, step.seconds, step.bar)
		}
		if want := 0.5 + 4520.0/480; math.Abs(total-want) > 1.0/sampleRate {
			t.Fatalf("duration %g, want %g", total, want)
		}
	}
	e.doc.Song.SetMeter(4, 4)
	e.changed(true)
	if _, _, bar := e.transportPosition(-1441); bar != 0 {
		t.Fatalf("meter edit retained stale position: bar %d", bar)
	}
}

func TestPracticeWaitControlResumesOnlyGatedPlayback(t *testing.T) {
	for _, waiting := range []bool{false, true} {
		t.Run(fmt.Sprint(waiting), func(t *testing.T) {
			e := newEditor()
			e.doc.Song = testSong()
			if err := e.audio.Configure(e.doc.Song); err != nil {
				t.Fatal(err)
			}
			e.practice, e.waitMode = true, true
			e.practiceFrame()
			if waiting {
				e.audio.Play(true)
				e.audio.Read(make([]byte, 4096))
				if !e.audio.Waiting() {
					t.Fatal("target did not gate playback")
				}
			}
			tester := ui.NewTester(e.practiceControls, 1200, 180)
			if err := tester.Click("等音"); err != nil {
				t.Fatal(err)
			}
			_, playing := e.audio.State()
			if e.waitMode || e.audio.Waiting() || playing != waiting {
				t.Fatalf("disable wait: mode=%v, waiting=%v, playing=%v; want playing=%v", e.waitMode, e.audio.Waiting(), playing, waiting)
			}
		})
	}
}
