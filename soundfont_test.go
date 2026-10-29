package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/sinshu/go-meltysynth/meltysynth"
	"gitlab.com/gomidi/midi/v2/smf"
)

func TestSoundFontMissingPresetReference(t *testing.T) {
	s := testSong()
	cfg := Instrument{Kind: "soundfont", File: "missing.sf2", Preset: &SoundFontPreset{Bank: 128, Program: 8}}
	s.Tracks[0].Settings.Instrument = cfg
	midi, err := s.MIDI()
	if err != nil {
		t.Fatal(err)
	}
	project := nativeProject{SheetJSON: SheetJSON{Version: 3, MIDI: base64.StdEncoding.EncodeToString(midi)}, Tracks: []TrackSettings{s.Tracks[0].Settings}}
	data, err := json.Marshal(project)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "missing.json")
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	d, err := openDocument(path)
	if err != nil {
		t.Fatal(err)
	}
	want := cfg
	want.File = filepath.Join(filepath.Dir(path), cfg.File)
	if !reflect.DeepEqual(d.Song.Tracks[0].Settings.Instrument, want) {
		t.Fatal("loading lost missing SF2 reference or bank/program")
	}
	a := newAudioEngine(20)
	if err = a.configure(d.Song, true); err == nil {
		t.Fatal("missing SoundFont did not report fallback")
	}
	a.Note(0, 60, true)
	pcm := make([]byte, 4096)
	a.Read(pcm)
	if bytes.Equal(pcm, make([]byte, len(pcm))) {
		t.Fatal("missing SoundFont fallback is silent")
	}
	if !reflect.DeepEqual(d.Song.Tracks[0].Settings.Instrument, want) {
		t.Fatal("fallback discarded original reference")
	}
	for _, p := range []SoundFontPreset{{Bank: -1}, {Bank: 65536}, {Program: -1}, {Program: 128}} {
		st := s.Tracks[0].Settings
		st.Instrument.Preset = &p
		if validateSettings(st) == nil {
			t.Fatalf("accepted invalid preset: %+v", p)
		}
	}
}

func TestLocalSoundFontPresetPlaybackAndSave(t *testing.T) {
	file := os.Getenv("DOMISO_TEST_SF2")
	if file == "" {
		file = filepath.Join("soundfonts", "GeneralUser-GS.sf2")
		if _, err := os.Stat(file); os.IsNotExist(err) {
			t.Skip("optional local SF2 fixture")
		}
	}
	s := emptySong()
	s.Tracks[0].Settings.Instrument = Instrument{Kind: "soundfont", File: file}
	a := newAudioEngine(20)
	if err := a.Configure(s); err != nil {
		t.Fatal(err)
	}
	sf, names := a.soundFontPresets(0)
	if len(sf.Presets) < 2 || len(names) != len(sf.Presets)+1 {
		t.Fatal("SoundFont choices do not expose actual presets")
	}
	e := newEditor()
	e.doc.Song = s
	e.audio = a
	tester := ui.NewTester(e.trackBar, 1000, 220)
	control, ok := tester.Find("SoundFont 音色，显示实际 Bank 和 Program，数值从 0 开始")
	if !ok || control.W > 280 || control.W < 180 {
		t.Fatalf("SoundFont selector must remain compact: found=%v, bounds=%+v", ok, control)
	}
	reference, err := meltysynth.NewSynthesizer(sf, meltysynth.NewSynthesizerSettings(sampleRate))
	if err != nil {
		t.Fatal(err)
	}
	choices := []*SoundFontPreset{nil}
	for _, p := range sf.Presets {
		if p.PatchNumber != 0 || p.BankNumber != 0 {
			choices = append(choices, &SoundFontPreset{Bank: int(p.BankNumber), Program: int(p.PatchNumber)})
			break
		}
	}
	for _, p := range sf.Presets {
		if p.BankNumber >= 128 {
			choices = append(choices, &SoundFontPreset{Bank: int(p.BankNumber), Program: int(p.PatchNumber)})
			break
		}
	}
	for _, choice := range choices {
		s.Tracks[0].Settings.Instrument.Preset = choice
		a.Update(s)
		for _, channel := range []byte{0, 9} {
			a.SeekTick(0)
			a.sendLocked(0, smf.Message{0xb0 | channel, 0, 0})
			a.sendLocked(0, smf.Message{0xc0 | channel, 5})
			a.sendLocked(0, smf.Message{0x90 | channel, 60, 100})
			reference.Reset()
			bank, program := int32(0), int32(5)
			if channel == 9 {
				bank = 128
			}
			if choice != nil {
				bank, program = int32(choice.Bank), int32(choice.Program)
			}
			reference.ProcessMidiMessage(0, 0xb0, 0, bank)
			reference.ProcessMidiMessage(0, 0xc0, program, 0)
			reference.NoteOn(0, 60, 100)
			var left, right, wantLeft, wantRight [2048]float32
			a.instruments[0].Synth.Render(left[:], right[:])
			reference.Render(wantLeft[:], wantRight[:])
			if left != wantLeft || right != wantRight {
				t.Fatalf("preset playback/seek differs on MIDI channel %d, preset %+v", channel, choice)
			}
		}
	}
	s.Tracks[0].Notes = []Note{{ID: 1, Duration: 480, Key: 60, Velocity: 100}}
	s.Tracks[0].Events = []MIDIEvent{{Data: smf.Message{0xc0, 5}}}
	d := Document{Song: s}
	path := filepath.Join(t.TempDir(), "preset.json")
	if err = d.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := openDocument(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.Song.Tracks[0].Settings.Instrument.Preset, s.Tracks[0].Settings.Instrument.Preset) {
		t.Fatal("save/load lost selected bank/program")
	}
	if !reflect.DeepEqual(loaded.Song.Tracks[0].Events, s.Tracks[0].Events) {
		t.Fatal("preset selection replaced original MIDI program events")
	}
}
