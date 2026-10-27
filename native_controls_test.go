package main

import (
	"github.com/egoist/mygo/ui"
	"testing"
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
