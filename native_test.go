package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ebitengine/oto/v3"

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
func TestNativeRollZoomControls(t *testing.T) {
	e := newEditor()
	e.rollWidth, e.rollHeight = 600, 300
	centerX := (e.rollWidth + rollKeysWidth) / 2
	beforeTick := e.tickAtX(centerX)
	horizontal := rollZoomButton(e.rollWidth, 1)
	e.rollInput(ui.InputEvent{Kind: ui.InputPointerDown, X: horizontal.X + horizontal.W/2, Y: horizontal.Y + horizontal.H/2})
	if e.zoom <= 0.15 || math.Abs(e.tickAtX(centerX)-beforeTick) > 0.01 || e.drag != "" {
		t.Fatal("horizontal zoom changed center tick or began timeline seek")
	}
	beforeHeight, beforeScroll := e.rowHeight, e.scrollY
	vertical := rollZoomButton(e.rollWidth, 3)
	e.rollInput(ui.InputEvent{Kind: ui.InputPointerDown, X: vertical.X + vertical.W/2, Y: vertical.Y + vertical.H/2})
	if e.rowHeight <= beforeHeight || e.scrollY <= beforeScroll || e.drag != "" {
		t.Fatal("vertical zoom failed or began timeline seek")
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
		tester.SetSize(1120, 760)
		small, err := os.Create("bin/native-preview-small.png")
		if err != nil {
			t.Fatal(err)
		}
		defer small.Close()
		if err := png.Encode(small, tester.Image()); err != nil {
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
	roll, ok := tester.Find("钢琴卷帘；顶部时间轴右侧可横向或纵向缩放，双击创建音符，拖动移动，右边缘改变时长，空白拖动框选")
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
func TestSpacePlaysOutsideTextEditors(t *testing.T) {
	e := newEditor()
	e.doc.Song = testSong()
	e.audio.Configure(e.doc.Song)
	tester := ui.NewTester(e.view, 1380, 900)
	for _, label := range []string{"静音", "跟随", "独奏"} {
		if err := tester.Click(label); err != nil {
			t.Fatal(err)
		}
		_, before := e.audio.State()
		tester.Key(0, ui.KeySpace)
		_, after := e.audio.State()
		if before == after {
			t.Fatalf("space did not toggle playback with %s focused", label)
		}
	}
	roll, ok := tester.Find("钢琴卷帘；顶部时间轴右侧可横向或纵向缩放，双击创建音符，拖动移动，右边缘改变时长，空白拖动框选")
	if !ok {
		t.Fatal("roll missing")
	}
	tester.ClickAt(roll.X+roll.W/2, roll.Y+roll.H/2)
	_, beforeRoll := e.audio.State()
	tester.Key(0, ui.KeySpace)
	_, afterRoll := e.audio.State()
	if beforeRoll == afterRoll {
		t.Fatal("space did not toggle playback with roll focused")
	}
	if err := tester.Click("DoMiSo 乐谱源文"); err != nil {
		t.Fatal(err)
	}
	_, before := e.audio.State()
	tester.Type("a")
	tester.TypeKey(0, ui.KeySpace, " ")
	tester.Type("b")
	_, after := e.audio.State()
	if before != after || e.doc.Notation != "a b" {
		t.Fatalf("text space changed playback or was not inserted: %q", e.doc.Notation)
	}
}

func TestPhysicalKeyboardRows(t *testing.T) {
	seen := map[string]bool{}
	for row, keys := range keyboardRows {
		var width float32
		for _, key := range keys {
			width += key.units
			if seen[key.code] {
				t.Fatalf("duplicate keyboard key %s", key.code)
			}
			seen[key.code] = true
		}
		if width != 15 {
			t.Fatalf("keyboard row %d width = %g, want 15", row, width)
		}
	}
	if keyboardRows[0][0].label != "`" || keyboardRows[4][len(keyboardRows[4])-1].label != "RCTRL" {
		t.Fatal("keyboard endpoints missing")
	}
	for ch := 'A'; ch <= 'Z'; ch++ {
		if !seen["Key"+string(ch)] {
			t.Fatalf("missing physical key %c", ch)
		}
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
func TestNativeToolbarTonicAndGrid(t *testing.T) {
	for text, want := range map[string]int{"1=C4": 60, "C#4": 61, "1=F5#": 78, "1=d4b": 61, "60": 60, "1=C-1": 0} {
		if got, ok := tonicKey(text); !ok || got != want {
			t.Fatalf("tonic %q: %d, %v; want %d", text, got, ok, want)
		}
	}
	for _, text := range []string{"1=H4", "1=C#4b", "128", "1=C99"} {
		if _, ok := tonicKey(text); ok {
			t.Fatalf("invalid tonic accepted: %q", text)
		}
	}
	if got := tonicName(60); got != "C4" {
		t.Fatalf("tonic label: %q", got)
	}
	e := &Editor{doc: Document{Song: emptySong()}}
	for grid, want := range map[string]int64{"1/3": 640, "1/5": 384, "1/6": 320, "1/7": 274} {
		e.grid = grid
		if got := e.gridTicks(); got != want {
			t.Fatalf("grid %s: %d; want %d", grid, got, want)
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

func TestNativeRollPagesWhenFollowIsOff(t *testing.T) {
	e := newEditor()
	e.follow = false
	var tick float64
	tester := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Grow(1).Draw(func(p *ui.Painter, r ui.Rect) {
			e.drawRoll(p, r, tick, true)
		})
	}, 400, 200)
	tick = 3000
	tester.Frame()
	if head := rollKeysWidth + float32(tick)*e.zoom - e.scrollX; head < rollKeysWidth || head >= 400 {
		t.Fatalf("playhead stayed outside right edge: %f", head)
	}
	tick = 100
	tester.Frame()
	if head := rollKeysWidth + float32(tick)*e.zoom - e.scrollX; head < rollKeysWidth || head >= 400 {
		t.Fatalf("playhead stayed outside left edge: %f", head)
	}
	if e.follow {
		t.Fatal("automatic page enabled follow")
	}
}

func TestNativeRollPanKeepsFollow(t *testing.T) {
	e := newEditor()
	e.mode = "播放"
	e.rollWidth, e.rollHeight = 400, 200
	e.rollInput(ui.InputEvent{Kind: ui.InputPointerDown, X: 100, Y: 100})
	if e.drag != "pan" || !e.follow {
		t.Fatal("pan disabled follow")
	}
	e.rollInput(ui.InputEvent{Kind: ui.InputPointerUp})
	if !e.follow {
		t.Fatal("finishing pan disabled follow")
	}
}

func TestNativeRollGridAndKeyShadingOnFractionalScroll(t *testing.T) {
	e := newEditor()
	e.follow = false
	e.scrollY = 0
	tester := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Grow(1).Draw(func(p *ui.Painter, r ui.Rect) {
			e.drawRoll(p, r, 0, false)
		})
	}, 400, 200)
	if pixel := tester.Image().RGBAAt(70, 46); pixel.R != palette.Grid.R || pixel.G != palette.Grid.G || pixel.B != palette.Grid.B {
		t.Fatalf("integer-scroll grid line missing: %v", pixel)
	}
	e.scrollY = 0.5
	tester.Frame()
	if pixel := tester.Image().RGBAAt(70, 45); pixel.R != palette.Grid.R || pixel.G != palette.Grid.G || pixel.B != palette.Grid.B {
		t.Fatalf("fractional-scroll grid line missing: %v", pixel)
	}
	white := tester.Image().RGBAAt(70, 35)
	black := tester.Image().RGBAAt(70, 75)
	if white.R != palette.Background.R || black.R != palette.Surface.R {
		t.Fatalf("wrong white/black lane shades: white=%v black=%v", white, black)
	}
}

func TestPlaybackProtectsTrackStructure(t *testing.T) {
	e := newEditor()
	e.doc.Song = testSong()
	e.doc.Song.Tracks = append(e.doc.Song.Tracks, newTrack("Second"))
	e.mode = "播放"
	before := cloneSong(e.doc.Song)
	tester := ui.NewTester(e.view, 1120, 760)
	for _, label := range []string{"新增", "删除", "上移轨道", "下移轨道"} {
		if err := tester.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	if err := tester.Click("轨道名称"); err != nil {
		t.Fatal(err)
	}
	tester.Type("changed")
	if !reflect.DeepEqual(e.doc.Song, before) || e.doc.Dirty {
		t.Fatal("playback allowed track structure or name edits")
	}
}

func TestTrackSelectionRevealsNotesWithoutOverridingScroll(t *testing.T) {
	e := newEditor()
	e.doc.Song = testSong()
	high := newTrack("High")
	high.Notes = []Note{{ID: 4, Key: 100, Duration: 480, Velocity: 100}}
	e.doc.Song.Tracks = append(e.doc.Song.Tracks, high)
	tester := ui.NewTester(e.view, 1120, 760)
	if err := tester.Click("2  High  (1)"); err != nil {
		t.Fatal(err)
	}
	y := rollHeaderHeight + float32(pianoHigh-100)*e.rowHeight - e.scrollY
	if e.track != 1 || y < rollHeaderHeight || y+e.rowHeight > e.rollHeight {
		t.Fatal("selected track notes remain outside viewport")
	}
	e.scrollY += 40
	scroll := e.scrollY
	tester.Frame()
	if e.scrollY != scroll {
		t.Fatal("track focus reset manual scrolling")
	}
}

func TestMetadataAndSampleControlsKeepKeyboardVisible(t *testing.T) {
	e := newEditor()
	e.doc.Song = testSong()
	e.doc.Meta.Title = strings.Repeat("Title ", 20)
	e.doc.Meta.Composer = "Composer"
	e.doc.Meta.Arranger = "Arranger"
	e.doc.Meta.Description = strings.Repeat("Description ", 40)
	e.doc.Song.Tracks[0].Settings.Instrument = Instrument{Kind: "sample", File: "sample.wav", RootKey: 48}
	e.selected[1] = true
	tester := ui.NewTester(e.view, 1120, 760)
	keyboard, ok := tester.Find("实体键盘；映射按键与音符提示")
	if !ok || keyboard.H < 150 {
		t.Fatalf("metadata or sample controls compress physical keyboard: %v", keyboard)
	}
	follow, ok := tester.Find("跟随")
	if !ok || follow.Y < keyboard.Y+keyboard.H || follow.Y+follow.H > 760 {
		t.Fatalf("transport overlaps keyboard or leaves viewport: %v", follow)
	}
}

func TestNotationAreaFillsRemainingSidebarHeight(t *testing.T) {
	for _, mode := range []string{"编辑", "播放"} {
		t.Run(mode, func(t *testing.T) {
			e := newEditor()
			e.mode = mode
			e.doc.Notation = "1=C4\n1 2 3"
			tester := ui.NewTester(e.view, 1120, 760)
			area, ok := tester.Find("DoMiSo 乐谱源文")
			if !ok {
				t.Fatal("notation area missing")
			}
			button, ok := tester.Find("从文本重新生成 MIDI")
			if !ok {
				t.Fatal("regenerate button missing")
			}
			tester.SetSize(1120, 900)
			taller, _ := tester.Find("DoMiSo 乐谱源文")
			lowerButton, _ := tester.Find("从文本重新生成 MIDI")
			if math.Abs(float64(taller.H-area.H-140)) > 1 || math.Abs(float64(lowerButton.Y-button.Y-140)) > 1 {
				t.Fatalf("notation did not absorb extra window height: before=%v after=%v", area, taller)
			}
			follow, ok := tester.Find("跟随")
			gap := follow.Y - lowerButton.Y - lowerButton.H
			if !ok || gap < 0 || lowerButton.Y < taller.Y+taller.H {
				t.Fatalf("notation, regenerate button and transport overlap: area=%v button=%v transport=%v", taller, lowerButton, follow)
			}
		})
	}
}
