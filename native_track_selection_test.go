package main

import (
	"bytes"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"
	"gitlab.com/gomidi/midi/v2/smf"
)

func TestTrackSelectionPreservesPlayingPCMAndTransport(t *testing.T) {
	for _, kind := range []string{"builtin", "sample-gated", "sample-decay"} {
		t.Run(kind, func(t *testing.T) {
			e := newEditor()
			e.mode = "播放"
			song := emptySong()
			song.Tracks = append(song.Tracks, newTrack("Second"), newTrack("Third"))
			instrument := Instrument{Kind: "builtin"}
			if kind != "builtin" {
				file := filepath.Join(t.TempDir(), "tone.wav")
				writeWAV(t, file)
				instrument = Instrument{Kind: "sample", File: file, RootKey: 60, Gated: kind == "sample-gated"}
			}
			for i := range song.Tracks {
				song.Tracks[i].Settings.Instrument = instrument
				song.Tracks[i].Notes = []Note{{ID: i + 1, Duration: 1920, Key: uint8(60 + i*4), Velocity: 100}}
				song.Tracks[i].Events = []MIDIEvent{{Tick: 0, Data: smf.Message{0xb0, 64, 127}}}
			}
			e.doc.Song = song
			before := cloneSong(song)
			e.selected[1] = true
			reference := newAudioEngine(20)
			for _, audio := range []*AudioEngine{e.audio, reference} {
				if err := audio.Configure(song); err != nil {
					t.Fatal(err)
				}
				audio.SetLoop(true, 0, 1920)
				audio.WaitAt(1440)
				audio.Play(true)
				audio.Read(make([]byte, 512*8))
			}
			tester := ui.NewTester(e.sidebarTracks, 280, 400)
			frame, next, clockFrame, clockAt := e.audio.frame, e.audio.next, e.audio.clockFrame, e.audio.clockAt
			tick, playing := e.audio.State()
			for _, label := range []string{"2  Second  (1)", "▸ 2  Second  (1)", "1  Track 1  (1)"} {
				if err := tester.Click(label); err != nil {
					t.Fatal(err)
				}
				gotTick, gotPlaying := e.audio.State()
				if gotTick != tick || gotPlaying != playing || !gotPlaying || e.audio.frame != frame || e.audio.next != next || e.audio.clockFrame != clockFrame || e.audio.clockAt != clockAt {
					t.Fatal("track selection changed playback clock or event cursor")
				}
			}
			// A release arriving after selection must not stop a mapped pitch on the new track.
			e.noteInput("KeyA", false, false)
			if e.track != 0 || !e.focusRoll || len(e.selected) != 0 || e.doc.Dirty || len(e.undo) != 0 || !reflect.DeepEqual(e.doc.Song, before) {
				t.Fatal("track selection changed document instead of display/edit selection")
			}
			if !reflect.DeepEqual(e.audio.hold, reference.hold) || e.audio.waitFrame != reference.waitFrame || e.audio.waiting != reference.waiting || e.audio.loop != reference.loop || e.audio.loopBegin != reference.loopBegin || e.audio.loopEnd != reference.loopEnd {
				t.Fatal("track selection changed sustain, wait, or loop state")
			}
			got, want := make([]byte, 1024*8), make([]byte, 1024*8)
			e.audio.Read(got)
			reference.Read(want)
			if bytes.Equal(want, make([]byte, len(want))) || !bytes.Equal(got, want) {
				t.Fatal("track selection interrupted or changed rendered playback PCM")
			}
		})
	}
}

func TestTrackSelectionReleasesOnlyOwnedLiveInput(t *testing.T) {
	e := newEditor()
	profile := builtinProfile()
	e.profiles, e.profileName, e.state = []GameProfile{profile}, profile.Name, profile.InitialState
	e.mode = "播放"
	liveKey, _, _ := profile.Binding(e.state, "KeyA")
	gameKey, _, _ := profile.Binding(e.state, "KeyS")
	song := emptySong()
	song.Tracks = append(song.Tracks, newTrack("Second"), newTrack("Third"))
	for i, key := range []int{gameKey, liveKey, gameKey} {
		song.Tracks[i].Notes = []Note{{ID: i + 1, Duration: 1920, Key: uint8(key), Velocity: 100}}
	}
	e.doc.Song = song
	reference := newAudioEngine(20)
	for _, audio := range []*AudioEngine{e.audio, reference} {
		if err := audio.Configure(song); err != nil {
			t.Fatal(err)
		}
		audio.Play(true)
	}
	e.noteInput("KeyA", true, false)
	e.noteInput("KeyS", true, true)
	reference.Note(0, liveKey, true)
	for _, audio := range []*AudioEngine{e.audio, reference} {
		audio.Read(make([]byte, 128*8))
	}
	tester := ui.NewTester(e.sidebarTracks, 280, 400)
	if err := tester.Click("▸ 1  Track 1  (1)"); err != nil {
		t.Fatal(err)
	}
	if len(e.keyDown) != 2 {
		t.Fatal("clicking selected track released held input")
	}
	frame, next, clockFrame, clockAt := e.audio.frame, e.audio.next, e.audio.clockFrame, e.audio.clockAt
	if err := tester.Click("2  Second  (1)"); err != nil {
		t.Fatal(err)
	}
	reference.Note(0, liveKey, false)
	if e.track != 1 || len(e.keyDown) != 0 || len(e.pressed) != 0 || len(e.fresh) != 0 {
		t.Fatal("track switch retained old input state")
	}
	if !e.audio.playing || e.audio.frame != frame || e.audio.next != next || e.audio.clockFrame != clockFrame || e.audio.clockAt != clockAt {
		t.Fatal("releasing old track input changed playback transport")
	}
	if !reflect.DeepEqual(e.audio.phase, reference.phase) {
		t.Fatal("track switch released a playback note or retained the old live note")
	}
	// Delayed releases and repeated release-all must be inert after ownership is cleared.
	e.noteInput("KeyA", false, false)
	e.noteInput("KeyS", false, true)
	e.releaseKeys()
	got, want := make([]byte, 1024*8), make([]byte, 1024*8)
	e.audio.Read(got)
	reference.Read(want)
	if bytes.Equal(want, make([]byte, len(want))) || !bytes.Equal(got, want) {
		t.Fatal("live-input release changed other tracks' PCM")
	}
}

func TestLiveInputReleaseUsesOriginalTrackAndSource(t *testing.T) {
	for _, method := range []string{"key-up", "release-all"} {
		t.Run(method, func(t *testing.T) {
			e := newEditor()
			profile := builtinProfile()
			e.profiles, e.profileName, e.state = []GameProfile{profile}, profile.Name, profile.InitialState
			key, _, _ := profile.Binding(e.state, "KeyA")
			e.doc.Song = emptySong()
			e.doc.Song.Tracks = append(e.doc.Song.Tracks, newTrack("Second"))
			if err := e.audio.Configure(e.doc.Song); err != nil {
				t.Fatal(err)
			}
			e.noteInput("KeyA", true, false)
			e.track = 1
			e.audio.Note(1, key, true)
			if method == "release-all" {
				e.releaseKeys()
			} else {
				// The stored source, not a later caller's source, owns the note-off.
				e.noteInput("KeyA", false, true)
			}
			if len(e.audio.phase) != 2 || !e.audio.phase[0].Released || e.audio.phase[1].Released || len(e.keyDown) != 0 {
				t.Fatal("input release targeted the current track or lost original source ownership")
			}
		})
	}
}
