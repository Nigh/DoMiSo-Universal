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
