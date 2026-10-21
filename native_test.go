package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"github.com/ebitengine/oto/v3"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"gitlab.com/gomidi/midi/v2/smf"
)

func testSong() Song {
	s := emptySong()
	s.NextID = 3
	s.Tracks[0].Notes = []Note{{1, 0, 480, 60, 100, 0}, {2, 240, 480, 60, 80, 0}, {3, 960, 480, 64, 90, 0}}
	s.Tracks[0].Events = []MIDIEvent{{0, smf.MetaTempo(120)}, {480, smf.MetaTempo(60)}, {0, smf.Message{0xb0, 64, 127}}, {720, smf.Message{0xb0, 64, 0}}, {0, smf.Message{0xc0, 5}}, {0, smf.Message{0xe0, 0, 64}}}
	return s
}
func TestNativeMIDIRoundTrip(t *testing.T) {
	s := testSong()
	b, err := s.MIDI()
	if err != nil {
		t.Fatal(err)
	}
	got, err := readSong(b)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s.Tracks[0].Notes, got.Tracks[0].Notes) {
		t.Fatalf("notes changed: %#v", got.Tracks[0].Notes)
	}
	if len(got.Tracks[0].Events) != len(s.Tracks[0].Events) {
		t.Fatal("lost raw MIDI events")
	}
	points := got.Tempos()
	if v := secondsAt(points, got.PPQ, 960); math.Abs(v-1.5) > 1e-9 {
		t.Fatalf("tempo map: %f", v)
	}
	if tickAt(points, got.PPQ, 1.5) != 960 {
		t.Fatal("clock inverse")
	}
}
func TestNativeRejectsUnsupportedMIDI(t *testing.T) {
	b, _ := testSong().MIDI()
	binary.BigEndian.PutUint16(b[8:10], 2)
	if _, err := readSong(b); err == nil {
		t.Fatal("accepted SMF2")
	}
	binary.BigEndian.PutUint16(b[8:10], 1)
	binary.BigEndian.PutUint16(b[12:14], 0xe728)
	if _, err := readSong(b); err == nil {
		t.Fatal("accepted SMPTE")
	}
}
func writeWAV(t *testing.T, path string) {
	t.Helper()
	var b bytes.Buffer
	b.WriteString("RIFF")
	binary.Write(&b, binary.LittleEndian, uint32(36+4800*2))
	b.WriteString("WAVEfmt ")
	binary.Write(&b, binary.LittleEndian, uint32(16))
	binary.Write(&b, binary.LittleEndian, uint16(1))
	binary.Write(&b, binary.LittleEndian, uint16(1))
	binary.Write(&b, binary.LittleEndian, uint32(48000))
	binary.Write(&b, binary.LittleEndian, uint32(96000))
	binary.Write(&b, binary.LittleEndian, uint16(2))
	binary.Write(&b, binary.LittleEndian, uint16(16))
	b.WriteString("data")
	binary.Write(&b, binary.LittleEndian, uint32(9600))
	for i := range 4800 {
		binary.Write(&b, binary.LittleEndian, int16(math.Sin(float64(i)*0.1)*10000))
	}
	if err := os.WriteFile(path, b.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
}
func TestNativeProjectAssets(t *testing.T) {
	dir := t.TempDir()
	sample := filepath.Join(dir, "tone.wav")
	writeWAV(t, sample)
	manifest := filepath.Join(dir, "pack.json")
	os.WriteFile(manifest, []byte(`{"version":1,"name":"test","samples":[{"file":"tone.wav","rootKey":60}]}`), 0644)
	d := Document{Song: testSong(), Notation: "1=C", Generated: true, OutOfSync: true, Dirty: true}
	d.Song.Tracks[0].Settings.Instrument = Instrument{Kind: "pack", File: manifest}
	path := filepath.Join(dir, "sheet.json")
	if err := d.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := openDocument(path)
	if err != nil {
		t.Fatal(err)
	}
	in, _, err := loadInstrument(got.Song.Tracks[0].Settings.Instrument, maxDecodedBytes)
	if err != nil || len(in.Samples) != 1 {
		t.Fatalf("assets: %v", err)
	}
	if d.Dirty {
		t.Fatal("save did not clear dirty")
	}
	portable := filepath.Join(dir, "portable")
	if err = os.Mkdir(portable, 0755); err != nil {
		t.Fatal(err)
	}
	if err = os.CopyFS(filepath.Join(portable, "sheet.assets"), os.DirFS(filepath.Join(dir, "sheet.assets"))); err != nil {
		t.Fatal(err)
	}
	projectBytes, _ := os.ReadFile(path)
	portablePath := filepath.Join(portable, "sheet.json")
	if err = os.WriteFile(portablePath, projectBytes, 0644); err != nil {
		t.Fatal(err)
	}
	moved, err := openDocument(portablePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = loadInstrument(moved.Song.Tracks[0].Settings.Instrument, maxDecodedBytes); err != nil {
		t.Fatal("moved project: ", err)
	}
	d.Song.Tracks[0].Settings.Instrument.File = filepath.Join(dir, "missing.json")
	before, _ := os.ReadFile(path)
	if err = d.Save(path); err == nil {
		t.Fatal("saved missing instrument")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("failed save overwrote project")
	}
	if _, err = packPath(manifest, "../outside.wav"); err == nil {
		t.Fatal("path traversal accepted")
	}
}
func TestSamplePackSymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	writeWAV(t, filepath.Join(outside, "tone.wav"))
	pack := filepath.Join(dir, "pack.json")
	os.WriteFile(pack, []byte(`{"version":1,"samples":[{"file":"link/tone.wav","rootKey":60}]}`), 0644)
	if err := os.Symlink(outside, filepath.Join(dir, "link")); err != nil {
		t.Skip("symlink permission: ", err)
	}
	if _, err := readPack(pack); err == nil {
		t.Fatal("followed symlink out of sample pack")
	}
}
func TestNativeGeneratedTextAndV2(t *testing.T) {
	dir := t.TempDir()
	d := Document{Song: testSong(), Notation: "1=C 4/4 120\n1 2 3", Generated: true}
	path := filepath.Join(dir, "text.json")
	if err := d.Save(path); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	var p nativeProject
	json.Unmarshal(b, &p)
	if p.MIDI != "" {
		t.Fatal("unnecessary MIDI snapshot")
	}
	if _, err := openDocument(path); err != nil {
		t.Fatal(err)
	}
	p.Version = 2
	p.Tracks = nil
	b, _ = json.Marshal(p)
	os.WriteFile(path, b, 0644)
	if _, err := openDocument(path); err != nil {
		t.Fatal(err)
	}
}
func TestNativeSamplesAndClock(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "tone.wav")
	writeWAV(t, file)
	s := testSong()
	s.Tracks[0].Settings.Instrument = Instrument{Kind: "sample", File: file, RootKey: 60, Gated: true}
	a := newAudioEngine(20)
	if err := a.Configure(s); err != nil {
		t.Fatal(err)
	}
	a.Play(true)
	buf := make([]byte, 48000*8)
	if _, err := a.Read(buf); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(buf, make([]byte, len(buf))) {
		t.Fatal("silent synth")
	}
	tick, _ := a.State()
	if math.Abs(tick-720) > 1 {
		t.Fatalf("clock tick=%f", tick)
	}
	a.SeekTick(960)
	tick, _ = a.State()
	if math.Abs(tick-960) > 1 {
		t.Fatal("seek")
	}
	a.SetLoop(true, 0, 480)
	a.SeekTick(0)
	a.Play(true)
	a.Read(buf)
	tick, _ = a.State()
	if tick >= 480 {
		t.Fatal("loop overflow")
	}
	samples := []sampleData{{Root: 48}, {Root: 60}}
	if nearestSample(samples, 54).Root != 48 {
		t.Fatal("sample tie break")
	}
	if _, err := decodeSample(file, 60, 4); err == nil {
		t.Fatal("decoded budget ignored")
	}
}
func TestNativePracticeAndMapping(t *testing.T) {
	p := builtinProfile()
	b, _ := json.Marshal(p)
	if _, err := parseGameProfile(b); err != nil {
		t.Fatal(err)
	}
	if matchesFreshChord([]int{60}, map[int]bool{60: true}, map[int]bool{}) {
		t.Fatal("held note advanced")
	}
	if matchesFreshChord([]int{60}, map[int]bool{60: true, 64: true}, map[int]bool{60: true}) {
		t.Fatal("extra note accepted")
	}
	s := testSong()
	s.Tracks[0].Notes = []Note{{1, 0, 480, 60, 100, 0}, {2, 480, 480, 64, 100, 0}}
	actions, err := buildKeySequence(s, p, 0)
	if err != nil || len(actions) != 4 {
		t.Fatalf("timeline %v %#v", err, actions)
	}
	for i := 1; i < len(actions); i++ {
		if actions[i].AtMS < actions[i-1].AtMS {
			t.Fatal("unsorted timeline")
		}
	}
	p.States[0].Bindings["KeyA"] = 200
	b, _ = json.Marshal(p)
	if _, err := parseGameProfile(b); err == nil {
		t.Fatal("invalid profile accepted")
	}
}
func TestNativeEditorGestures(t *testing.T) {
	e := newEditor()
	e.doc.Song = testSong()
	e.audio.Configure(e.doc.Song)
	e.selected = map[int]bool{1: true}
	e.transpose(1)
	if e.doc.Song.Tracks[0].Notes[0].Key != 61 {
		t.Fatal("transpose")
	}
	e.copySelection(false)
	e.seek(1440)
	e.paste()
	if len(e.doc.Song.Tracks[0].Notes) != 4 {
		t.Fatal("paste")
	}
	before := len(e.undo)
	e.rollInput(ui.InputEvent{Kind: ui.InputPointerDown, X: 53, Y: 26 + float32(pianoHigh-61)*20 - e.scrollY + 5, Button: 0})
	e.rollInput(ui.InputEvent{Kind: ui.InputPointerMove, X: 173, Y: 26 + float32(pianoHigh-61)*20 - e.scrollY + 5, Button: 0})
	e.rollInput(ui.InputEvent{Kind: ui.InputPointerUp})
	if len(e.undo) != before+1 {
		t.Fatal("drag did not produce one undo")
	}
	e.mode = "播放"
	n := len(e.doc.Song.Tracks[0].Notes)
	e.deleteSelection()
	if len(e.doc.Song.Tracks[0].Notes) != n {
		t.Fatal("playback mode edited notes")
	}
}
func TestNativeView(t *testing.T) {
	e := newEditor()
	e.doc.Song = testSong()
	e.audio.Configure(e.doc.Song)
	tester := ui.NewTester(e.view, 1380, 900)
	if tester.Image().Bounds().Dx() != 1380 {
		t.Fatal("render")
	}
	if err := tester.Click("撤销"); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("DOMISO_RENDER_PREVIEW") != "" {
		os.MkdirAll("bin", 0755)
		midi, err := e.doc.Song.MIDI()
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile("bin/native-smoke.mid", midi, 0644); err != nil {
			t.Fatal(err)
		}
		f, err := os.Create("bin/native-preview.png")
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err = png.Encode(f, tester.Image()); err != nil {
			t.Fatal(err)
		}
	}
}
func TestPracticeWaitUsesAudioClock(t *testing.T) {
	e := newEditor()
	e.doc.Song = testSong()
	e.audio.Configure(e.doc.Song)
	e.practice, e.waitMode = true, true
	e.practiceFrame()
	e.audio.Play(true)
	buf := make([]byte, 4096)
	e.audio.Read(buf)
	if !e.audio.Waiting() {
		t.Fatal("target did not stop the audio clock")
	}
	if !bytes.Equal(buf, make([]byte, len(buf))) {
		t.Fatal("target sounded before judgement")
	}
	e.practiceFrame()
	e.noteInput("KeyA", true, false)
	_, on := e.audio.State()
	if !on || e.group != 1 {
		t.Fatal("fresh correct key did not resume wait")
	}
	e.audio.Play(false)
	e.group = 0
	clear(e.fresh)
	e.noteInput("KeyA", false, false)
	e.noteInput("KeyA", true, false)
	_, on = e.audio.State()
	if on {
		t.Fatal("mapped key resumed a manual pause")
	}
}
func TestNativeUIEditingAndInputFocus(t *testing.T) {
	e := newEditor()
	e.doc.Song = testSong()
	e.audio.Configure(e.doc.Song)
	tester := ui.NewTester(e.view, 1380, 900)
	roll, ok := tester.Find("钢琴卷帘；双击创建，拖动移动，右边缘改变时长，空白拖动框选")
	if !ok {
		t.Fatal("roll missing")
	}
	nr := e.noteRect(e.doc.Song.Tracks[0].Notes[0])
	tester.ClickAt(roll.X+nr.X+5, roll.Y+nr.Y+5)
	if e.doc.Dirty || len(e.undo) != 0 {
		t.Fatal("selection changed music")
	}
	tester.Key(ui.Ctrl, ui.KeyC)
	if len(e.clipboard) != 1 {
		t.Fatal("copy shortcut not routed to roll")
	}
	tester.Key(0, ui.KeyDelete)
	if len(e.doc.Song.Tracks[0].Notes) != 2 {
		t.Fatal("delete shortcut")
	}
	tester.KeyDown(ui.KeyA)
	if !e.pressed[60] {
		t.Fatal("PC note did not enter input stream")
	}
	tester.KeyUp(ui.KeyA)
	if len(e.pressed) != 0 {
		t.Fatal("PC key stayed held")
	}
	tester.KeyDown(ui.KeyA)
	if err := tester.Click("DoMiSo 乐谱源文"); err != nil {
		t.Fatal(err)
	}
	if len(e.pressed) != 0 {
		t.Fatal("focus change left live input held")
	}
	tester.KeyUp(ui.KeyA)
	tester.Type("1=C")
	tester.KeyDown(ui.KeyQ)
	if len(e.pressed) != 0 {
		t.Fatal("text input triggered a musical note")
	}
	tester.KeyUp(ui.KeyQ)
	if e.doc.Notation != "1=C" {
		t.Fatal("text editing")
	}
	tester.SetScale(2)
	tester.Frame()
	if tester.Image().Bounds().Dx() != 2760 {
		t.Fatal("HiDPI render")
	}
}
func TestLocalSoundFont(t *testing.T) {
	file := os.Getenv("DOMISO_TEST_SF2")
	if file == "" {
		t.Skip("optional local SF2 fixture")
	}
	s := testSong()
	s.Tracks[0].Settings.Instrument = Instrument{Kind: "soundfont", File: file}
	a := newAudioEngine(20)
	if err := a.Configure(s); err != nil {
		t.Fatal(err)
	}
	a.Play(true)
	buf := make([]byte, 48000*8)
	a.Read(buf)
	if bytes.Equal(buf, make([]byte, len(buf))) {
		t.Fatal("silent SoundFont")
	}
}
func TestNativeEndReleasesLastNote(t *testing.T) {
	s := emptySong()
	s.Tracks[0].Notes = []Note{{1, 0, 1920, 60, 100, 0}}
	a := newAudioEngine(20)
	a.Configure(s)
	a.Play(true)
	a.Read(make([]byte, 48000*2*8))
	for _, v := range a.phase {
		if !v.Released {
			t.Fatal("last note-off was skipped")
		}
	}
	a.Read(make([]byte, 48000*8))
	if len(a.phase) != 0 {
		t.Fatal("stuck voice at end")
	}
}
func TestNativeTenMinutePlayback(t *testing.T) {
	a := newAudioEngine(20)
	a.Configure(testSong())
	a.SetLoop(true, 0, 1920)
	a.Play(true)
	buf := make([]byte, 48000*8)
	for range 10 {
		a.Read(buf)
	}
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for range 600 {
		a.Read(buf)
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	if after.HeapAlloc > before.HeapAlloc+2<<20 {
		t.Fatalf("heap grew: %d -> %d", before.HeapAlloc, after.HeapAlloc)
	}
	tick, on := a.State()
	if !on || tick >= 1920 {
		t.Fatalf("long playback stopped: %f %v", tick, on)
	}
	t.Logf("600 seconds of PCM, retained heap %d -> %d", before.HeapAlloc, after.HeapAlloc)
}
func TestNativeScrubDefersRestore(t *testing.T) {
	e := newEditor()
	e.doc.Song = testSong()
	e.audio.Configure(e.doc.Song)
	e.audio.Play(true)
	e.drag = "seek"
	e.beginScrub()
	next := e.audio.next
	for _, tick := range []float64{100, 400, 800, 1200} {
		e.seek(tick)
	}
	if e.audio.next != next {
		t.Fatal("scrub rescanned MIDI before release")
	}
	e.finishDrag()
	_, playing := e.audio.State()
	if !playing || e.audio.next <= next {
		t.Fatal("scrub did not restore on release")
	}
}
func TestConcurrentNativeAudio(t *testing.T) {
	a := newAudioEngine(20)
	s := testSong()
	a.Configure(s)
	var wg sync.WaitGroup
	wg.Go(func() {
		buf := make([]byte, 4096)
		for range 100 {
			a.Read(buf)
		}
	})
	for range 100 {
		a.State()
		a.Play(true)
		a.Note(0, 60, true)
		a.Note(0, 60, false)
		a.Update(s)
		a.SeekTick(480)
	}
	wg.Wait()
}
func TestNativeMeterAndTonic(t *testing.T) {
	s := emptySong()
	s.SetMeter(3, 4)
	s.Tracks[0].Events = append(s.Tracks[0].Events, MIDIEvent{1440, smf.MetaMeter(4, 4)})
	var ticks, bars []int64
	barsInRange(s.Meters(), -2880, 3360, func(tick, bar int64) { ticks = append(ticks, tick); bars = append(bars, bar) })
	if !reflect.DeepEqual(ticks, []int64{-2880, -1440, 0, 1440, 3360}) || !reflect.DeepEqual(bars, []int64{-1, 0, 1, 2, 3}) {
		t.Fatalf("meter map %v %v", ticks, bars)
	}
	for text, want := range map[string]int{"1=F5#": 66, "1 = d4b": 49, "1=G\n=====\n1=C": 60, "1=B99": 60} {
		if got := notationTonic(text); got != want {
			t.Fatalf("%q: %d", text, got)
		}
	}
}
func TestContinuousNativeDisplayClock(t *testing.T) {
	a := newAudioEngine(20)
	a.Configure(testSong())
	a.Play(true)
	a.player = &oto.Player{}
	a.frame = 48000
	a.clockAt = time.Now().Add(-250 * time.Millisecond)
	tick, on := a.State()
	if !on || math.Abs(tick-240) > 5 {
		t.Fatalf("display followed producer frontier: %f", tick)
	}
	a.playing = false
	a.frame = a.end
	a.clockAt = time.Now().Add(-3400 * time.Millisecond)
	tick, on = a.State()
	if !on || tick >= 1920 {
		t.Fatal("display ended at lookahead frontier")
	}
	a.clockAt = time.Now().Add(-3600 * time.Millisecond)
	tick, on = a.State()
	if on || tick != 1920 {
		t.Fatal("display did not end on actual clock")
	}
}
func BenchmarkNativeFrame(b *testing.B) {
	e := newEditor()
	s := emptySong()
	for i := range 100000 {
		s.Tracks[0].Notes = append(s.Tracks[0].Notes, Note{ID: i, Tick: int64(i * 60), Duration: 120, Key: uint8(48 + i%36), Velocity: 100})
	}
	e.doc.Song = s
	tester := ui.NewTester(e.view, 1920, 1080)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.scrollX += 3
		tester.Frame()
	}
}
func BenchmarkNativeViewport(b *testing.B) {
	e := newEditor()
	s := emptySong()
	for i := range 100000 {
		s.Tracks[0].Notes = append(s.Tracks[0].Notes, Note{ID: i, Tick: int64(i * 60), Duration: 120, Key: uint8(48 + i%36), Velocity: 100})
	}
	e.doc.Song = s
	e.ensureRollIndex()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = e.visibleNotes(0, float64(i*60), float64(i*60+4000))
	}
}

func TestPianoRangeAndImportedTrackVisibility(t *testing.T) {
	e := newEditor()
	song := testSong()
	metadata := newTrack("conductor-only")
	song.Tracks = append([]Track{metadata}, song.Tracks...)
	e.installDocument(Document{Song: song})
	if e.track != 1 {
		t.Fatal("metadata track selected")
	}
	tester := ui.NewTester(e.view, 1120, 760)
	if _, ok := tester.Find("1  conductor-only  (0)"); ok {
		t.Fatal("empty imported track displayed")
	}
	e.doc.Song.Tracks[1].Notes = append(e.doc.Song.Tracks[1].Notes,
		Note{ID: 100, Key: 20, Duration: 480}, Note{ID: 101, Key: 109, Duration: 480})
	e.rollNotes = nil
	e.ensureRollIndex()
	for _, n := range e.rollNotes[1] {
		if n.Key < pianoLow || n.Key > pianoHigh {
			t.Fatal("out of piano range indexed")
		}
	}
	if e.keyAtY(-10000) != pianoHigh || e.keyAtY(10000) != pianoLow {
		t.Fatal("piano input bounds")
	}
	if len(e.doc.Song.Tracks) != 2 {
		t.Fatal("metadata track lost")
	}
}
