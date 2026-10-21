package main

import (
	"encoding/json"
	"fmt"
	"github.com/egoist/mygo"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Editor struct {
	inputFocused, buildFocus               bool
	benchmark                              bool
	dragChanged                            bool
	dragPlaying                            bool
	dragIndices                            []int
	boxBase                                map[int]bool
	autoRunning                            bool
	autoStarted                            time.Time
	autoBase                               float64
	autoTempos                             []TempoPoint
	labelKey                               string
	labelCache                             [128]string
	displayTick                            float64
	displayPlaying                         bool
	rollNotes                              [][]Note
	rollEnds                               [][]int64
	meters                                 []MeterPoint
	win                                    *mygo.Window
	doc                                    Document
	audio                                  *AudioEngine
	track                                  int
	selected                               map[int]bool
	undo, redo                             []Document
	clipboard                              []Note
	message                                string
	busy                                   bool
	zoom, rowHeight, scrollX, scrollY      float32
	rollWidth, rollHeight                  float32
	grid, label, mode                      string
	tonic                                  float64
	loop, follow, recording                bool
	loopStart, loopEnd                     float64
	drag                                   string
	dragX, dragY, dragScrollX, dragScrollY float32
	dragTick                               float64
	dragBefore                             *Document
	dragNotes                              []Note
	boxX, boxY                             float32
	pressed                                map[int]bool
	keyDown                                map[string]int
	recordStart                            map[int]int64
	flashes                                map[int]time.Time
	profiles                               []GameProfile
	profileName, state                     string
	practice, waitMode, gamePractice       bool
	practiceWait                           bool
	groups                                 []Chord
	group                                  int
	fresh                                  map[int]bool
	sampleRoot, tempo, meterNum            float64
	meterDen, buffer, sheet                string
	sheets                                 []SheetInfo
}
type appSettings struct {
	Buffer   string        `json:"buffer"`
	Profiles []GameProfile `json:"profiles"`
}

func newEditor() *Editor {
	p := builtinProfile()
	e := &Editor{doc: Document{Song: emptySong()}, selected: map[int]bool{}, zoom: 0.15, rowHeight: 20, scrollY: 48 * 20, grid: "1/4", label: "简谱", mode: "编辑", tonic: 60, follow: true, loopEnd: 1920, pressed: map[int]bool{}, keyDown: map[string]int{}, recordStart: map[int]int64{}, flashes: map[int]time.Time{}, fresh: map[int]bool{}, profiles: []GameProfile{p}, profileName: p.Name, state: p.InitialState, sampleRoot: 60, tempo: 120, meterNum: 4, meterDen: "4", buffer: "20", message: "打开曲谱，或输入 DoMiSo 后生成"}
	e.readSettings()
	applyDefaultInstrument(&e.doc.Song)
	ms := 20
	fmt.Sscan(e.buffer, &ms)
	e.audio = newAudioEngine(ms)
	e.sheets, _ = (&AppService{}).ListFreeSheets()
	return e
}
func settingsPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "domiso-universal", "settings.json")
}
func (e *Editor) readSettings() {
	b, err := boundedRead(settingsPath(), maxSheetBytes)
	if err != nil {
		return
	}
	var s appSettings
	if json.Unmarshal(b, &s) != nil {
		return
	}
	if s.Buffer == "20" || s.Buffer == "40" || s.Buffer == "80" {
		e.buffer = s.Buffer
	}
	for _, p := range s.Profiles {
		b, _ := json.Marshal(p)
		valid, err := parseGameProfile(b)
		if err == nil && p.ID != e.profiles[0].ID {
			e.profiles = append(e.profiles, valid)
		}
	}
}
func (e *Editor) saveSettings() {
	path := settingsPath()
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		e.message = err.Error()
		return
	}
	b, _ := json.MarshalIndent(appSettings{e.buffer, e.profiles[1:]}, "", "  ")
	if err := atomicWrite(path, b); err != nil {
		e.message = err.Error()
	}
}
func (e *Editor) profile() GameProfile {
	for _, p := range e.profiles {
		if p.Name == e.profileName {
			return p
		}
	}
	return e.profiles[0]
}
func snapshot(d Document) Document { d.Song = cloneSong(d.Song); return d }
func (e *Editor) checkpoint() {
	e.undo = append(e.undo, snapshot(e.doc))
	if len(e.undo) > 100 {
		e.undo = e.undo[1:]
	}
	e.redo = nil
}
func (e *Editor) changed(music bool) {
	e.rollNotes = nil
	e.doc.Dirty = true
	if music {
		performance.stop()
		performance.mu.Lock()
		performance.actions = nil
		performance.mu.Unlock()
		e.autoRunning = false
		e.doc.OutOfSync = e.doc.Notation != ""
		e.audio.Update(e.doc.Song)
	}
	e.groups = nil
}
func (e *Editor) edit(fn func()) {
	if e.busy || e.mode != "编辑" {
		return
	}
	e.checkpoint()
	fn()
	e.changed(true)
}
func (e *Editor) history(redo bool) {
	if e.busy {
		return
	}
	src, dst := &e.undo, &e.redo
	if redo {
		src, dst = &e.redo, &e.undo
	}
	if len(*src) == 0 {
		return
	}
	e.stop()
	*dst = append(*dst, snapshot(e.doc))
	path := e.doc.Path
	e.doc = (*src)[len(*src)-1]
	e.doc.Path = path
	*src = (*src)[:len(*src)-1]
	e.doc.Dirty = true
	e.track = min(e.track, len(e.doc.Song.Tracks)-1)
	e.selected = map[int]bool{}
	e.configureAudio()
}
func (e *Editor) confirmUnsaved() bool {
	if !e.doc.Dirty {
		return true
	}
	r, err := mygo.Dialog.Message(mygo.MessageOptions{Parent: e.win, Type: mygo.MessageQuestion, Message: "保存当前项目的修改？", Buttons: []string{"保存", "丢弃", "取消"}, CancelButton: 2})
	if err != nil || r.Button == 2 {
		return false
	}
	if r.Button == 0 {
		return e.save(false)
	}
	return true
}
func (e *Editor) save(as bool) bool {
	path := e.doc.Path
	if as || !strings.EqualFold(filepath.Ext(path), ".json") {
		var err error
		path, err = mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: e.win, Title: "保存 DoMiSo 项目", DefaultPath: "sheet.json", Filters: []mygo.FileFilter{{Name: "DoMiSo 项目", Extensions: []string{"json"}}}})
		if err != nil {
			e.message = err.Error()
			return false
		}
		if path == "" {
			return false
		}
		if filepath.Ext(path) == "" {
			path += ".json"
		}
	}
	originals := make([]string, len(e.doc.Song.Tracks))
	for i, t := range e.doc.Song.Tracks {
		originals[i] = t.Settings.Instrument.File
	}
	if err := e.doc.Save(path); err != nil {
		e.message = err.Error()
		return false
	}
	rebased := map[string]string{}
	for i, old := range originals {
		if old != "" {
			rebased[old] = e.doc.Song.Tracks[i].Settings.Instrument.File
		}
	}
	for _, history := range [][]Document{e.undo, e.redo} {
		for i := range history {
			for j := range history[i].Song.Tracks {
				in := &history[i].Song.Tracks[j].Settings.Instrument
				if file, ok := rebased[in.File]; ok {
					in.File = file
				}
			}
		}
	}
	e.message = "已保存 " + path
	return true
}
func (e *Editor) pick(title string, extensions ...string) string {
	files, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: e.win, Title: title, Filters: []mygo.FileFilter{{Name: title, Extensions: extensions}}})
	if err != nil {
		e.message = err.Error()
	}
	if len(files) > 0 {
		return files[0]
	}
	return ""
}
func (e *Editor) open() {
	if !e.confirmUnsaved() {
		return
	}
	if path := e.pick("打开曲谱", "txt", "dms", "json", "mid", "midi"); path != "" {
		e.load(path)
	}
}
func (e *Editor) load(path string) {
	if e.busy {
		return
	}
	e.stop()
	d, err := openDocument(path)
	if err != nil {
		e.message = err.Error()
		return
	}
	e.installDocument(d)
}
func (e *Editor) installDocument(d Document) {
	e.rollNotes = nil
	applyDefaultInstrument(&d.Song)
	e.doc = d
	e.track = 0
	for i, t := range d.Song.Tracks {
		if len(t.Notes) > 0 {
			e.track = i
			break
		}
	}
	e.undo = nil
	e.redo = nil
	e.selected = map[int]bool{}
	e.scrollX = -float32(d.Song.BarTicks()*2) * e.zoom
	e.tempo = d.Song.Tempos()[0].BPM
	for _, p := range d.Song.Tempos() {
		if p.Tick == 0 {
			e.tempo = p.BPM
		}
	}
	e.loopEnd = float64(d.Song.End())
	e.tonic = float64(notationTonic(d.Notation))
	for _, t := range d.Song.Tracks {
		for _, v := range t.Events {
			var num, den uint8
			if v.Tick == 0 && v.Data.GetMetaMeter(&num, &den) {
				e.meterNum = float64(num)
				e.meterDen = fmt.Sprint(den)
			}
		}
	}
	e.configureAudio()
}
func applyDefaultInstrument(s *Song) {
	candidates := []string{filepath.Join("soundfonts", "GeneralUser-GS.sf2")}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "soundfonts", "GeneralUser-GS.sf2"), filepath.Join(filepath.Dir(exe), "..", "soundfonts", "GeneralUser-GS.sf2"))
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			abs, _ := filepath.Abs(path)
			for i := range s.Tracks {
				if s.Tracks[i].Settings.Instrument.Kind == "" {
					s.Tracks[i].Settings.Instrument = Instrument{Kind: "soundfont", File: abs}
				}
			}
			return
		}
	}
}
func (e *Editor) configureAudio() {
	if e.busy {
		return
	}
	e.busy = true
	s := cloneSong(e.doc.Song)
	engine := e.audio
	go func() {
		warningErr := engine.configure(s, true)
		var err error
		warning := ""
		if warningErr != nil {
			warning = warningErr.Error()
		}
		if err == nil {
			err = engine.start()
		}
		e.win.Update(func() {
			e.busy = false
			if err != nil {
				e.message = "音频不可用: " + err.Error()
			} else if warning != "" {
				e.message = "音源缺失，使用内置音试听；保存将保留原设置: " + warning
			} else {
				e.message = "已就绪"
			}
			e.audio.SeekTick(0)
			if e.benchmark {
				e.audio.Play(true)
			}
		})
	}()
}
func (e *Editor) regenerate() {
	if e.busy || strings.TrimSpace(e.doc.Notation) == "" {
		return
	}
	if e.doc.OutOfSync || e.doc.Dirty {
		r, err := mygo.Dialog.Message(mygo.MessageOptions{Parent: e.win, Type: mygo.MessageWarning, Message: "用文本重新生成 MIDI？当前音乐编辑将被替换。", Buttons: []string{"重新生成", "取消"}, CancelButton: 1})
		if err != nil || r.Button != 0 {
			return
		}
	}
	b, err := (&AppService{}).ParseTextToMIDI(e.doc.Notation)
	if err != nil {
		e.message = err.Error()
		return
	}
	s, err := readSong(b)
	if err != nil {
		e.message = err.Error()
		return
	}
	e.stop()
	e.checkpoint()
	e.doc.Song = s
	e.doc.Generated = true
	e.doc.OutOfSync = false
	e.doc.Dirty = true
	e.track = 0
	e.selected = map[int]bool{}
	e.tonic = float64(notationTonic(e.doc.Notation))
	e.configureAudio()
}
func (e *Editor) export(midi bool) {
	var b []byte
	var err error
	ext := "txt"
	if midi {
		ext = "mid"
		b, err = e.doc.Song.MIDI()
	} else {
		if e.doc.OutOfSync || !e.doc.Generated {
			e.message = "MIDI 已修改，请保存 JSON 项目，不能导出为文本"
			return
		}
		b = []byte(e.doc.Notation)
	}
	if err != nil {
		e.message = err.Error()
		return
	}
	path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: e.win, Title: "导出 " + ext, DefaultPath: "sheet." + ext})
	if err != nil {
		e.message = err.Error()
		return
	}
	if path == "" {
		return
	}
	if err = atomicWrite(path, b); err != nil {
		e.message = err.Error()
	} else {
		e.message = "已导出 " + path
	}
}
func (e *Editor) togglePlay() {
	if e.busy {
		return
	}
	tick, on := e.audio.State()
	if on {
		e.audio.Play(false)
		e.audio.SeekTick(tick)
		e.practiceWait = false
	} else {
		if tick >= float64(e.doc.Song.End()) {
			e.audio.SeekTick(0)
		}
		e.audio.Play(true)
	}
}
func (e *Editor) stop() {
	if e.audio == nil {
		return
	}
	e.releaseKeys()
	e.audio.Play(false)
	e.audio.WaitAt(-1)
	e.audio.SeekTick(0)
	performance.stop()
	e.autoRunning = false
	e.recording = false
	e.practiceWait = false
	e.groups = nil
}
func (e *Editor) seek(tick float64) {
	if e.autoRunning {
		performance.stop()
		e.autoRunning = false
	}
	e.audio.WaitAt(-1)
	tick = max(-float64(e.doc.Song.BarTicks()*2), min(tick, float64(e.doc.Song.End())))
	if e.drag == "seek" || e.drag == "pan" && e.mode == "播放" {
		e.audio.PreviewTick(tick)
	} else {
		e.audio.SeekTick(tick)
	}
	e.groups = nil
	e.practiceWait = false
}
func (e *Editor) releaseKeys() {
	for code, n := range e.keyDown {
		e.audio.Note(e.track, n, false)
		delete(e.keyDown, code)
	}
	e.pressed = map[int]bool{}
	clear(e.fresh)
	e.finishRecording()
	e.audio.Silence()
}
func (e *Editor) finishRecording() {
	if len(e.recordStart) == 0 {
		return
	}
	tick, _ := e.audio.State()
	for n, start := range e.recordStart {
		if tick > float64(start) {
			e.addRecorded(n, start, int64(tick))
		}
	}
	clear(e.recordStart)
}
func (e *Editor) addRecorded(key int, start, end int64) {
	s := &e.doc.Song
	s.NextID++
	t := &s.Tracks[e.track]
	t.Notes = append(t.Notes, Note{s.NextID, start, max(1, end-start), uint8(key), 100, uint8(e.track % 16)})
	e.changed(true)
}
func (e *Editor) noteInput(code string, down bool, game bool) {
	p := e.profile()
	n, state, ok := p.Binding(e.state, code)
	if !down {
		if held, exists := e.keyDown[code]; exists {
			n = held
			ok = true
			delete(e.keyDown, code)
		}
	}
	if down && state != e.state {
		e.state = state
		return
	}
	if !ok {
		return
	}
	if down {
		if _, held := e.keyDown[code]; held {
			return
		}
		e.keyDown[code] = n
		e.pressed[n] = true
		e.fresh[n] = true
		e.flashes[n] = time.Now()
	} else {
		delete(e.pressed, n)
	}
	if !game {
		e.audio.Note(e.track, n, down)
		if e.recording {
			tick, _ := e.audio.State()
			if down && tick >= 0 {
				e.recordStart[n] = int64(tick)
			} else if start, exists := e.recordStart[n]; exists {
				delete(e.recordStart, n)
				e.addRecorded(n, start, int64(tick))
			}
		}
	}
	e.judge()
}
func (e *Editor) judge() {
	if !e.practice || e.group >= len(e.groups) {
		return
	}
	if matchesFreshChord(e.groups[e.group].Notes, e.pressed, e.fresh) {
		resume := e.audio.Waiting()
		e.group++
		clear(e.fresh)
		if e.waitMode {
			if e.group < len(e.groups) {
				e.audio.WaitAt(float64(e.groups[e.group].Tick))
			} else {
				e.audio.WaitAt(-1)
			}
		}
		if resume {
			e.practiceWait = false
			e.audio.Play(true)
		}
	}
}
func (e *Editor) practiceFrame() {
	if !e.practice {
		e.audio.WaitAt(-1)
		return
	}
	tick, _ := e.audio.State()
	if e.groups == nil {
		e.groups = songChords(e.doc.Song, int64(tick))
		e.group = 0
		clear(e.fresh)
	}
	if e.group >= len(e.groups) {
		e.audio.WaitAt(-1)
		return
	}
	if e.waitMode {
		e.audio.WaitAt(float64(e.groups[e.group].Tick))
		e.practiceWait = e.audio.Waiting()
	} else {
		e.audio.WaitAt(-1)
		for e.group < len(e.groups)-1 && tick >= float64(e.groups[e.group+1].Tick) {
			e.group++
			clear(e.fresh)
		}
	}
}
func (e *Editor) performanceEvent(ev performanceEvent) {
	switch ev.Name {
	case "performance:started":
		e.autoRunning = true
		e.autoStarted, _ = ev.Data.(time.Time)
		e.audio.Play(false)
	case "performance:focus-lost":
		e.releaseKeys()
	case "performance:key":
		if e.gamePractice {
			m, ok := ev.Data.(map[string]any)
			if ok {
				scan, _ := m["scanCode"].(uint32)
				down, _ := m["down"].(bool)
				for code, s := range e.profile().ScanCodes {
					if s == uint16(scan) {
						e.noteInput(code, down, true)
						break
					}
				}
			}
		}
	case "performance:error", "performance:stopped":
		if e.autoRunning {
			tick, _ := e.transportState()
			e.autoRunning = false
			e.audio.SeekTick(tick)
		}
		e.message = fmt.Sprint(ev.Data)
		if ev.Name == "performance:stopped" {
			e.audio.Play(false)
		}
	}
}
func (e *Editor) prepareAuto() {
	if runtime.GOOS != "windows" {
		e.message = "游戏自动演奏仅支持 Windows"
		return
	}
	tick, _ := e.audio.State()
	p := e.profile()
	actions, err := buildKeySequence(e.doc.Song, p, int64(tick))
	if err == nil {
		err = (&AppService{}).PrepareKeySequence(actions, p.ProcessNames)
	}
	if err != nil {
		e.message = err.Error()
		return
	}
	e.autoBase = tick
	e.autoTempos = e.doc.Song.Tempos()
	e.message = "时间线已准备；切换到游戏，Ctrl+Alt+F8 开始，F9 紧急停止"
}
func (e *Editor) transportState() (float64, bool) {
	if !e.autoRunning {
		return e.audio.State()
	}
	points := e.autoTempos
	sec := secondsAt(points, e.doc.Song.PPQ, e.autoBase) + time.Since(e.autoStarted).Seconds()
	return min(float64(e.doc.Song.End()), tickAt(points, e.doc.Song.PPQ, sec)), true
}
func (e *Editor) importProfile() {
	path := e.pick("导入按键映射", "json")
	if path == "" {
		return
	}
	b, err := boundedRead(path, maxSheetBytes)
	if err == nil {
		var p GameProfile
		p, err = parseGameProfile(b)
		if err == nil {
			if p.ID == e.profiles[0].ID {
				err = fmt.Errorf("不能覆盖内置映射")
			} else {
				for i := 1; i < len(e.profiles); i++ {
					if e.profiles[i].ID == p.ID {
						e.profiles = append(e.profiles[:i], e.profiles[i+1:]...)
						break
					}
				}
				e.profiles = append(e.profiles, p)
				e.profileName = p.Name
				e.state = p.InitialState
				e.saveSettings()
			}
		}
	}
	if err != nil {
		e.message = err.Error()
	}
}
func (e *Editor) exportProfile() {
	b, _ := json.MarshalIndent(e.profile(), "", "  ")
	path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: e.win, Title: "导出按键映射", DefaultPath: "profile.json"})
	if err == nil && path != "" {
		err = atomicWrite(path, b)
	}
	if err != nil {
		e.message = err.Error()
	}
}
func (e *Editor) setInstrument() {
	if e.busy {
		return
	}
	e.releaseKeys()
	path := e.pick("选择音源", "sf2", "wav", "ogg", "mp3", "json")
	if path == "" {
		return
	}
	kind := "sample"
	switch strings.ToLower(filepath.Ext(path)) {
	case ".sf2":
		kind = "soundfont"
	case ".json":
		kind = "pack"
	}
	cfg := Instrument{Kind: kind, File: path, RootKey: int(e.sampleRoot)}
	e.busy = true
	s := cloneSong(e.doc.Song)
	s.Tracks[e.track].Settings.Instrument = cfg
	go func() {
		err := e.audio.Configure(s)
		e.win.Update(func() {
			e.busy = false
			if err != nil {
				e.message = err.Error()
				return
			}
			e.checkpoint()
			e.doc.Song = s
			e.changed(false)
			e.message = "已加载音源"
		})
	}()
}
func (e *Editor) gridTicks() int64 {
	switch e.grid {
	case "自由":
		return 0
	case "1/16":
		return max(1, e.doc.Song.PPQ/4)
	case "1/8":
		return max(1, e.doc.Song.PPQ/2)
	case "1/2":
		return e.doc.Song.PPQ * 2
	}
	return e.doc.Song.PPQ
}
func (e *Editor) deleteSelection() {
	e.edit(func() {
		t := &e.doc.Song.Tracks[e.track]
		out := t.Notes[:0]
		for _, n := range t.Notes {
			if !e.selected[n.ID] {
				out = append(out, n)
			}
		}
		t.Notes = out
		clear(e.selected)
	})
}
func (e *Editor) copySelection(cut bool) {
	e.clipboard = nil
	for _, n := range e.doc.Song.Tracks[e.track].Notes {
		if e.selected[n.ID] {
			e.clipboard = append(e.clipboard, n)
		}
	}
	if len(e.clipboard) > 0 {
		sort.Slice(e.clipboard, func(i, j int) bool { return e.clipboard[i].Tick < e.clipboard[j].Tick })
		if cut {
			e.deleteSelection()
		}
	}
}
func (e *Editor) paste() {
	tick, _ := e.audio.State()
	if tick < 0 || len(e.clipboard) == 0 {
		return
	}
	e.edit(func() {
		clear(e.selected)
		origin := e.clipboard[0].Tick
		for _, n := range e.clipboard {
			e.doc.Song.NextID++
			n.ID = e.doc.Song.NextID
			n.Tick = snapTick(tick, e.gridTicks()) + n.Tick - origin
			e.doc.Song.Tracks[e.track].Notes = append(e.doc.Song.Tracks[e.track].Notes, n)
			e.selected[n.ID] = true
		}
	})
}
func (e *Editor) transpose(delta int) {
	e.edit(func() {
		for i := range e.doc.Song.Tracks[e.track].Notes {
			n := &e.doc.Song.Tracks[e.track].Notes[i]
			if e.selected[n.ID] {
				n.Key = uint8(max(0, min(127, int(n.Key)+delta)))
			}
		}
	})
}

var tonicPattern = regexp.MustCompile(`(?i)\b1\s*=\s*([a-g])(\d+)?([#b])?`)
var notationSeparator = regexp.MustCompile(`(?m)^\s*={5,}\s*$`)

func notationTonic(text string) int {
	if sep := notationSeparator.FindStringIndex(text); sep != nil {
		text = text[sep[1]:]
	}
	m := tonicPattern.FindStringSubmatch(text)
	if m == nil {
		return 60
	}
	octave := 5
	if m[2] != "" {
		n, err := strconv.Atoi(m[2])
		if err != nil {
			return 60
		}
		octave = n
	}
	names := map[string]int{"C": 0, "D": 2, "E": 4, "F": 5, "G": 7, "A": 9, "B": 11}
	n := octave*12 + names[strings.ToUpper(m[1])]
	if m[3] == "#" {
		n++
	} else if strings.EqualFold(m[3], "b") {
		n--
	}
	if n < 0 || n > 127 {
		return 60
	}
	return n
}
func noteLabel(key int, mode string, tonic int, p GameProfile, state string) string {
	switch mode {
	case "隐藏":
		return ""
	case "按键":
		return strings.TrimPrefix(p.KeyFor(state, key), "Key")
	case "简谱":
		offset := key - tonic
		degree := []string{"1", "♯1", "2", "♯2", "3", "4", "♯4", "5", "♯5", "6", "♯6", "7"}[((offset%12)+12)%12]
		oct := int(math.Floor(float64(offset) / 12))
		if oct > 0 {
			return degree + strings.Repeat("˙", min(oct, 4))
		}
		if oct < 0 {
			return degree + strings.Repeat("̣", min(-oct, 4))
		}
		return degree
	default:
		return fmt.Sprintf("%s%d", []string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}[key%12], key/12-1)
	}
}
