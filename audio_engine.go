package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
	"github.com/egoist/mygo/audio"
	_ "github.com/egoist/mygo/audio/mp3"
	_ "github.com/egoist/mygo/audio/vorbis"
	"github.com/sinshu/go-meltysynth/meltysynth"
	"gitlab.com/gomidi/midi/v2/smf"
)

const sampleRate = 48000

type sampleData struct {
	Root           int
	Rate, Channels int
	PCM            []float32
}
type loadedInstrument struct {
	Config      Instrument
	Samples     []sampleData
	Synth       *meltysynth.Synthesizer
	PresetNames []string
}
type sampleVoice struct {
	Track, Key      int
	Pos, Step, Gain float64
	Data            *sampleData
	Released        bool
}
type scheduledEvent struct {
	Frame int64
	Track int
	Data  smf.Message
}
type AudioEngine struct {
	mu                 sync.Mutex
	player             *oto.Player
	instruments        []*loadedInstrument
	volumes            []float64
	events             []scheduledEvent
	next               int
	tempos             []TempoPoint
	ppq                int64
	frame, end         int64
	playing            bool
	displayOn          bool
	clockFrame         int64
	clockAt            time.Time
	loop               bool
	loopBegin, loopEnd int64
	voices             []sampleVoice
	phase              []sampleVoice
	hold               map[[2]int]bool
	left, right        []float32
	bufferMS           int
	err                error
	waitFrame          int64
	waiting            bool
}

func newAudioEngine(ms int) *AudioEngine {
	return &AudioEngine{waitFrame: -1, bufferMS: ms, ppq: 480, tempos: []TempoPoint{{0, 0, 120}}, hold: map[[2]int]bool{}, left: make([]float32, 1024), right: make([]float32, 1024)}
}
func (a *AudioEngine) start() error {
	a.mu.Lock()
	existing := a.player
	a.mu.Unlock()
	if existing != nil {
		return a.err
	}
	ctx, ready, e := oto.NewContext(&oto.NewContextOptions{SampleRate: sampleRate, ChannelCount: 2, Format: oto.FormatFloat32LE, BufferSize: time.Duration(a.bufferMS) * time.Millisecond, ApplicationName: "DoMiSo Universal"})
	if e != nil {
		return e
	}
	<-ready
	if e = ctx.Err(); e != nil {
		return e
	}
	player := ctx.NewPlayer(a)
	player.SetBufferSize(sampleRate * 8 * a.bufferMS / 1000)
	a.mu.Lock()
	a.player = player
	a.mu.Unlock()
	player.Play()
	return nil
}
func loadInstrument(cfg Instrument, remaining int64) (*loadedInstrument, int64, error) {
	in := &loadedInstrument{Config: cfg}
	if cfg.Kind == "" || cfg.Kind == "builtin" {
		return in, 0, nil
	}
	if cfg.Kind == "soundfont" {
		b, e := boundedRead(cfg.File, maxAssetBytes)
		if e != nil {
			return nil, 0, e
		}
		sf, e := meltysynth.NewSoundFont(bytes.NewReader(b))
		if e != nil {
			return nil, 0, e
		}
		if cfg.Preset != nil && !soundFontHasPreset(sf, *cfg.Preset) {
			return nil, 0, fmt.Errorf("SoundFont 没有 bank %d / program %d", cfg.Preset.Bank, cfg.Preset.Program)
		}
		in.PresetNames = make([]string, len(sf.Presets)+1)
		in.PresetNames[0] = "跟随 MIDI 音色"
		for i, p := range sf.Presets {
			in.PresetNames[i+1] = fmt.Sprintf("Bank %d / Program %d · %s", p.BankNumber, p.PatchNumber, p.Name)
		}
		s, e := meltysynth.NewSynthesizer(sf, meltysynth.NewSynthesizerSettings(sampleRate))
		in.Synth = s
		return in, int64(len(b)), e
	}
	specs := []SampleSpec{{File: cfg.File, RootKey: cfg.RootKey}}
	if cfg.Kind == "pack" {
		p, e := readPack(cfg.File)
		if e != nil {
			return nil, 0, e
		}
		specs = p.Samples
		for i := range specs {
			f, e := packPath(cfg.File, specs[i].File)
			if e != nil {
				return nil, 0, e
			}
			specs[i].File = f
		}
	}
	var used int64
	for _, s := range specs {
		pcm, e := decodeSample(s.File, s.RootKey, remaining-used)
		if e != nil {
			return nil, 0, e
		}
		used += int64(len(pcm.PCM)) * 4
		in.Samples = append(in.Samples, pcm)
	}
	sort.Slice(in.Samples, func(i, j int) bool { return in.Samples[i].Root < in.Samples[j].Root })
	return in, used, nil
}
func decodeSample(file string, root int, remaining int64) (sampleData, error) {
	var out sampleData
	f, e := os.Open(file)
	if e != nil {
		return out, e
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		return out, e
	}
	if st.Size() > maxAssetBytes {
		return out, fmt.Errorf("采样文件超过 256 MiB")
	}
	d, _, e := audio.NewDecoder(f)
	if e != nil {
		return out, e
	}
	out.Root, out.Rate, out.Channels = root, d.SampleRate(), d.Channels()
	if out.Rate <= 0 || out.Channels < 1 || out.Channels > 2 {
		return out, fmt.Errorf("采样仅支持单声道或立体声")
	}
	buf := make([]float32, 4096)
	for {
		n, e := d.Read(buf)
		if int64(len(out.PCM)+n)*4 > remaining {
			return out, fmt.Errorf("已解码采样超过 512 MiB")
		}
		out.PCM = append(out.PCM, buf[:n]...)
		if e == io.EOF {
			break
		}
		if e != nil {
			return out, e
		}
		if n == 0 {
			return out, io.ErrNoProgress
		}
	}
	if len(out.PCM) == 0 {
		return out, fmt.Errorf("空采样")
	}
	return out, nil
}
func nearestSample(samples []sampleData, key int) *sampleData {
	var chosen *sampleData
	for i := range samples {
		if chosen == nil || absInt(samples[i].Root-key) < absInt(chosen.Root-key) {
			chosen = &samples[i]
		}
	}
	return chosen
}
func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
func (a *AudioEngine) Configure(s Song) error { return a.configure(s, false) }
func (a *AudioEngine) configure(s Song, allowMissing bool) error {
	// Decode outside the audio lock; a failed replacement leaves the working engine intact.
	loaded := make([]*loadedInstrument, len(s.Tracks))
	var used int64
	var warning error
	for i, t := range s.Tracks {
		var e error
		var n int64
		loaded[i], n, e = loadInstrument(t.Settings.Instrument, maxDecodedBytes-used)
		if e != nil {
			if !allowMissing {
				return fmt.Errorf("%s: %w", t.Name, e)
			}
			warning = fmt.Errorf("%s: %w", t.Name, e)
			loaded[i] = &loadedInstrument{}
			n = 0
		}
		used += n
		if used > maxDecodedBytes {
			return fmt.Errorf("音源总量超过 512 MiB")
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.instruments = loaded
	a.rebuild(s)
	return warning
}
func (a *AudioEngine) rebuild(s Song) {
	frame := a.frame
	if a.player != nil && a.displayOn {
		frame = a.displayFrameLocked(time.Now())
	}
	tick := tickAt(a.tempos, a.ppq, float64(frame)/sampleRate)
	a.ppq = s.PPQ
	a.tempos = s.Tempos()
	a.events = nil
	a.volumes = make([]float64, len(s.Tracks))
	a.voices = nil
	a.phase = nil
	for i, t := range s.Tracks {
		if s.Audible(i) {
			a.volumes[i] = t.Settings.Volume
		}
		if i < len(a.instruments) && a.instruments[i].Config.Kind == t.Settings.Instrument.Kind && a.instruments[i].Config.File == t.Settings.Instrument.File {
			a.instruments[i].Config.Preset = t.Settings.Instrument.Preset
		}
		for _, e := range t.Events {
			a.events = append(a.events, scheduledEvent{int64(math.Round(secondsAt(a.tempos, s.PPQ, float64(e.Tick)) * sampleRate)), i, e.Data})
		}
		for _, n := range t.Notes {
			a.events = append(a.events, scheduledEvent{int64(math.Round(secondsAt(a.tempos, s.PPQ, float64(n.Tick)) * sampleRate)), i, smf.Message{0x90 | n.Channel, n.Key, n.Velocity}}, scheduledEvent{int64(math.Round(secondsAt(a.tempos, s.PPQ, float64(n.Tick+n.Duration)) * sampleRate)), i, smf.Message{0x80 | n.Channel, n.Key, 0}})
		}
	}
	sort.SliceStable(a.events, func(i, j int) bool {
		if a.events[i].Frame != a.events[j].Frame {
			return a.events[i].Frame < a.events[j].Frame
		}
		return eventOrder(a.events[i].Data) < eventOrder(a.events[j].Data)
	})
	a.end = int64(secondsAt(a.tempos, s.PPQ, float64(s.End())) * sampleRate)
	a.seekLocked(tick)
	a.clockFrame = a.frame
	a.clockAt = time.Now()
}
func (a *AudioEngine) Update(s Song) { a.mu.Lock(); defer a.mu.Unlock(); a.rebuild(s) }
func (a *AudioEngine) seekLocked(tick float64) {
	a.frame = int64(secondsAt(a.tempos, a.ppq, tick) * sampleRate)
	a.next = sort.Search(len(a.events), func(i int) bool { return a.events[i].Frame >= a.frame })
	a.voices = nil
	a.phase = nil
	clear(a.hold)
	for _, in := range a.instruments {
		if in.Synth != nil {
			in.Synth.Reset()
			applySoundFontPreset(in)
		}
	}
	for i := 0; i < a.next; i++ {
		e := a.events[i]
		if len(e.Data) > 0 && e.Data[0] < 0xf0 && e.Data[0]&0xe0 != 0x80 {
			a.sendLocked(e.Track, e.Data)
		}
	}
}
func (a *AudioEngine) SeekTick(tick float64) {
	a.mu.Lock()
	player := a.player
	a.mu.Unlock()
	if player != nil {
		player.PauseAndStopReading()
	}
	a.mu.Lock()
	a.seekLocked(tick)
	a.clockFrame = a.frame
	a.clockAt = time.Now()
	a.mu.Unlock()
	if player != nil {
		_, _ = player.Seek(0, io.SeekStart)
		player.Play()
	}
}
func (a *AudioEngine) PreviewTick(tick float64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.frame = int64(secondsAt(a.tempos, a.ppq, tick) * sampleRate)
	a.clockFrame = a.frame
	a.clockAt = time.Now()
}

// Seek clears Oto's queued PCM without changing the transport selected by SeekTick.
func (a *AudioEngine) Seek(_ int64, _ int) (int64, error) { return 0, nil }
func (a *AudioEngine) State() (float64, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	f, on := a.frame, a.playing
	if a.player != nil && a.displayOn {
		f = a.displayFrameLocked(time.Now())
		on = f < a.end
		if a.waiting && f >= a.waitFrame {
			on = false
		}
	}
	return tickAt(a.tempos, a.ppq, float64(f)/sampleRate), on
}
func (a *AudioEngine) displayFrameLocked(now time.Time) int64 {
	f := a.frame
	if a.displayOn {
		f = a.clockFrame + int64(now.Sub(a.clockAt).Seconds()*sampleRate)
		if a.loop && f >= a.loopEnd {
			f = a.loopBegin + (f-a.loopBegin)%(a.loopEnd-a.loopBegin)
		}
		if a.waiting {
			f = min(f, a.waitFrame)
		}
		f = min(f, a.end)
	}
	return f
}
func (a *AudioEngine) Play(on bool) {
	a.mu.Lock()
	a.playing = on
	a.displayOn = on
	a.clockFrame = a.frame
	a.clockAt = time.Now()
	a.waiting = false
	if !on {
		a.waiting = false
		a.voices = nil
		a.phase = nil
		for _, in := range a.instruments {
			if in.Synth != nil {
				in.Synth.NoteOffAll(true)
			}
		}
	}
	a.mu.Unlock()
}
func (a *AudioEngine) WaitAt(tick float64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if tick < 0 {
		a.waitFrame = -1
	} else {
		a.waitFrame = int64(secondsAt(a.tempos, a.ppq, tick) * sampleRate)
	}
}
func (a *AudioEngine) Waiting() bool { a.mu.Lock(); defer a.mu.Unlock(); return a.waiting }
func (a *AudioEngine) SetLoop(on bool, begin, end float64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.player != nil {
		now := time.Now()
		a.clockFrame = a.displayFrameLocked(now)
		a.clockAt = now
	}
	a.loop = on && end > begin
	a.loopBegin = int64(secondsAt(a.tempos, a.ppq, begin) * sampleRate)
	a.loopEnd = int64(secondsAt(a.tempos, a.ppq, end) * sampleRate)
}
func (a *AudioEngine) Note(track, key int, down bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	status := byte(0x80)
	if down {
		status = 0x90
	}
	a.sendLocked(track, smf.Message{status, byte(key), 100})
}
func (a *AudioEngine) sendLocked(track int, m smf.Message) {
	if track < 0 || track >= len(a.instruments) || len(m) < 2 || m[0] >= 0xf0 {
		return
	}
	in := a.instruments[track]
	if in.Synth != nil {
		// Track selection overrides MIDI patch messages without changing stored events.
		if in.Config.Preset != nil && (m[0]&0xf0 == 0xc0 || m[0]&0xf0 == 0xb0 && (m[1] == 0 || m[1] == 32)) {
			return
		}
		d2 := int32(0)
		if len(m) > 2 {
			d2 = int32(m[2])
		}
		in.Synth.ProcessMidiMessage(int32(m[0]&15), int32(m[0]&0xf0), int32(m[1]), d2)
		return
	}
	key := int(m[1])
	kind := m[0] & 0xf0
	if kind == 0xb0 && len(m) > 2 {
		if m[1] == 64 {
			a.hold[[2]int{track, int(m[0] & 15)}] = m[2] >= 64
		}
		return
	}
	if kind == 0x80 || (kind == 0x90 && len(m) > 2 && m[2] == 0) {
		if in.Config.Gated || len(in.Samples) == 0 {
			for i := range a.voices {
				if a.voices[i].Track == track && a.voices[i].Key == key {
					a.voices[i].Released = true
				}
			}
			for i := range a.phase {
				if a.phase[i].Track == track && a.phase[i].Key == key {
					a.phase[i].Released = true
				}
			}
		}
		return
	}
	if kind != 0x90 || len(m) < 3 || a.volumes[track] == 0 {
		return
	}
	gain := float64(m[2]) / 127
	v := sampleVoice{Track: track, Key: key, Gain: gain, Step: 440 * math.Pow(2, float64(key-69)/12) / sampleRate}
	if len(in.Samples) > 0 {
		v.Data = nearestSample(in.Samples, key)
		v.Step = float64(v.Data.Rate) / sampleRate * math.Pow(2, float64(key-v.Data.Root)/12)
		a.voices = append(a.voices, v)
	} else {
		v.Gain *= 0.15
		a.phase = append(a.phase, v)
	}
	if len(a.voices) > 512 {
		a.voices = a.voices[len(a.voices)-512:]
	}
	if len(a.phase) > 128 {
		a.phase = a.phase[len(a.phase)-128:]
	}
}
func (a *AudioEngine) Read(buf []byte) (int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	frames := len(buf) / 8
	for pos := 0; pos < frames; {
		if a.playing && a.waitFrame >= 0 && a.frame >= a.waitFrame {
			a.frame = a.waitFrame
			a.playing = false
			a.waiting = true
		}
		if a.playing && a.loop && a.frame >= a.loopEnd {
			a.seekLocked(tickAt(a.tempos, a.ppq, float64(a.loopBegin)/sampleRate))
		}
		for a.playing && a.next < len(a.events) && a.events[a.next].Frame <= a.frame {
			e := a.events[a.next]
			a.sendLocked(e.Track, e.Data)
			a.next++
		}
		n := min(1024, frames-pos)
		if a.playing && a.waitFrame >= 0 {
			n = min(n, max(1, int(a.waitFrame-a.frame)))
		}
		if a.playing && a.next < len(a.events) {
			n = min(n, max(1, int(a.events[a.next].Frame-a.frame)))
		}
		if a.playing && a.loop {
			n = min(n, max(1, int(a.loopEnd-a.frame)))
		}
		l, r := a.left[:n], a.right[:n]
		clear(l)
		clear(r)
		for i, in := range a.instruments {
			if in.Synth != nil {
				var tl, tr [1024]float32
				in.Synth.Render(tl[:n], tr[:n])
				for j := range n {
					l[j] += tl[j] * float32(a.volumes[i])
					r[j] += tr[j] * float32(a.volumes[i])
				}
			}
		}
		a.voices = mixVoices(a.voices, l, r, a.volumes, false)
		a.phase = mixVoices(a.phase, l, r, a.volumes, true)
		for j := range n {
			binary.LittleEndian.PutUint32(buf[(pos+j)*8:], math.Float32bits(float32(math.Tanh(float64(l[j])))))
			binary.LittleEndian.PutUint32(buf[(pos+j)*8+4:], math.Float32bits(float32(math.Tanh(float64(r[j])))))
		}
		if a.playing {
			a.frame += int64(n)
			if a.loop && a.frame >= a.loopEnd {
				a.seekLocked(tickAt(a.tempos, a.ppq, float64(a.loopBegin)/sampleRate))
			}
			if !a.loop && a.frame >= a.end {
				a.frame = a.end
				for a.next < len(a.events) && a.events[a.next].Frame <= a.end {
					ev := a.events[a.next]
					a.sendLocked(ev.Track, ev.Data)
					a.next++
				}
				a.playing = false
			}
		}
		pos += n
	}
	return frames * 8, nil
}
func mixVoices(voices []sampleVoice, l, r []float32, volumes []float64, builtin bool) []sampleVoice {
	out := voices[:0]
	for _, v := range voices {
		alive := true
		for i := range l {
			if v.Released {
				v.Gain *= 0.995
			}
			if v.Gain < 0.0001 {
				alive = false
				break
			}
			gain := float32(v.Gain * volumes[v.Track])
			if builtin {
				sample := float32(math.Sin(v.Pos*2*math.Pi)) * gain
				l[i] += sample
				r[i] += sample
				v.Pos += v.Step
				v.Pos -= math.Floor(v.Pos)
			} else {
				d := v.Data
				idx := int(v.Pos)
				count := len(d.PCM) / d.Channels
				if idx >= count {
					alive = false
					break
				}
				next := min(idx+1, count-1)
				frac := float32(v.Pos - float64(idx))
				x := d.PCM[idx*d.Channels]*(1-frac) + d.PCM[next*d.Channels]*frac
				y := x
				if d.Channels == 2 {
					y = d.PCM[idx*2+1]*(1-frac) + d.PCM[next*2+1]*frac
				}
				l[i] += x * gain
				r[i] += y * gain
				v.Pos += v.Step
			}
		}
		if alive {
			out = append(out, v)
		}
	}
	return out
}
