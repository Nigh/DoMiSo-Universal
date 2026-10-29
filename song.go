package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"sort"

	"gitlab.com/gomidi/midi/v2/smf"
)

type Note struct {
	ID                     int
	Tick, Duration         int64
	Key, Velocity, Channel uint8
}
type MIDIEvent struct {
	Tick int64
	Data smf.Message
}
type SoundFontPreset struct {
	Bank    int `json:"bank"`
	Program int `json:"program"`
}
type Instrument struct {
	Kind    string           `json:"kind,omitempty"`
	File    string           `json:"file,omitempty"`
	RootKey int              `json:"rootKey,omitempty"`
	Gated   bool             `json:"gated,omitempty"`
	Preset  *SoundFontPreset `json:"preset,omitempty"`
}
type TrackSettings struct {
	Instrument Instrument `json:"instrument"`
	Volume     float64    `json:"volume"`
	Mute       bool       `json:"mute,omitempty"`
	Solo       bool       `json:"solo,omitempty"`
}

func (s *TrackSettings) UnmarshalJSON(data []byte) error {
	type settings TrackSettings
	value := settings{Volume: 1}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*s = TrackSettings(value)
	return nil
}

type Track struct {
	Name     string
	Notes    []Note
	Events   []MIDIEvent
	Settings TrackSettings
}
type Song struct {
	PPQ    int64
	Tracks []Track
	NextID int
}
type TempoPoint struct {
	Tick         int64
	Seconds, BPM float64
}
type MeterPoint struct{ Tick, Bar, Size int64 }

func (s Song) Meters() []MeterPoint {
	out := []MeterPoint{{0, 1, s.PPQ * 4}}
	for _, t := range s.Tracks {
		for _, e := range t.Events {
			var n, d uint8
			if e.Data.GetMetaMeter(&n, &d) && n > 0 && d > 0 {
				out = append(out, MeterPoint{Tick: e.Tick, Size: max(1, s.PPQ*4*int64(n)/int64(d))})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Tick < out[j].Tick })
	clean := out[:0]
	for _, p := range out {
		if len(clean) > 0 && clean[len(clean)-1].Tick == p.Tick {
			clean[len(clean)-1] = p
		} else {
			clean = append(clean, p)
		}
	}
	out = clean
	out[0].Bar = 1
	for i := 1; i < len(out); i++ {
		p := out[i-1]
		out[i].Bar = p.Bar + (out[i].Tick-p.Tick+p.Size-1)/p.Size
	}
	return out
}
func barsInRange(meters []MeterPoint, left, right int64, fn func(tick, bar int64)) {
	i := max(0, sort.Search(len(meters), func(i int) bool { return meters[i].Tick > left })-1)
	for ; i < len(meters); i++ {
		m := meters[i]
		n := int64(math.Ceil(float64(left-m.Tick) / float64(m.Size)))
		if i > 0 {
			n = max(0, n)
		}
		end := right
		if i+1 < len(meters) {
			end = min(end, meters[i+1].Tick-1)
		}
		for tick := m.Tick + n*m.Size; tick <= end; tick += m.Size {
			fn(tick, m.Bar+n)
			n++
		}
		if end >= right {
			return
		}
		left = end + 1
	}
}

func newTrack(name string) Track { return Track{Name: name, Settings: TrackSettings{Volume: 1}} }
func emptySong() Song            { return Song{PPQ: 480, Tracks: []Track{newTrack("Track 1")}} }
func readSong(data []byte) (result Song, err error) {
	if len(data) < 14 || string(data[:4]) != "MThd" || binary.BigEndian.Uint16(data[8:10]) > 1 || binary.BigEndian.Uint16(data[12:14])&0x8000 != 0 {
		return Song{}, fmt.Errorf("仅支持 PPQ MIDI 格式 0/1")
	}
	defer func() {
		if p := recover(); p != nil {
			result = Song{}
			err = fmt.Errorf("无效 MIDI: %v", p)
		}
	}()
	f, err := smf.ReadFrom(bytes.NewReader(data))
	if err != nil {
		return Song{}, err
	}
	ppq, ok := f.TimeFormat.(smf.MetricTicks)
	if !ok || ppq == 0 || f.Format() > 1 {
		return Song{}, fmt.Errorf("仅支持 PPQ MIDI 格式 0/1")
	}
	s := Song{PPQ: int64(ppq)}
	for i, raw := range f.Tracks {
		t := newTrack(fmt.Sprintf("Track %d", i+1))
		tick := int64(0)
		pending := map[[2]uint8][]int{}
		for _, ev := range raw {
			tick += int64(ev.Delta)
			m := ev.Message
			var ch, key, vel uint8
			if m.GetMetaTrackName(&t.Name) {
				continue
			}
			if len(m) >= 2 && m[0] == 0xff && m[1] == 0x2f {
				continue
			}
			if m.GetNoteStart(&ch, &key, &vel) {
				s.NextID++
				t.Notes = append(t.Notes, Note{s.NextID, tick, 1, key, vel, ch})
				k := [2]uint8{ch, key}
				pending[k] = append(pending[k], len(t.Notes)-1)
			} else if m.GetNoteEnd(&ch, &key) {
				k := [2]uint8{ch, key}
				q := pending[k]
				if len(q) > 0 {
					n := &t.Notes[q[0]]
					n.Duration = max(1, tick-n.Tick)
					pending[k] = q[1:]
				} else {
					t.Events = append(t.Events, MIDIEvent{tick, append(smf.Message(nil), m...)})
				}
			} else {
				t.Events = append(t.Events, MIDIEvent{tick, append(smf.Message(nil), m...)})
			}
		}
		for _, q := range pending {
			for _, idx := range q {
				t.Notes[idx].Duration = max(1, tick-t.Notes[idx].Tick)
			}
		}
		s.Tracks = append(s.Tracks, t)
	}
	if len(s.Tracks) == 0 {
		s.Tracks = emptySong().Tracks
	}
	return s, nil
}
func (s Song) MIDI() ([]byte, error) {
	f := smf.New()
	f.TimeFormat = smf.MetricTicks(s.PPQ)
	for _, t := range s.Tracks {
		events := append([]MIDIEvent(nil), t.Events...)
		events = append(events, MIDIEvent{0, smf.MetaTrackSequenceName(t.Name)})
		for _, n := range t.Notes {
			events = append(events, MIDIEvent{n.Tick, smf.Message{0x90 | n.Channel, n.Key, n.Velocity}}, MIDIEvent{n.Tick + n.Duration, smf.Message{0x80 | n.Channel, n.Key, 0}})
		}
		sort.SliceStable(events, func(i, j int) bool {
			if events[i].Tick != events[j].Tick {
				return events[i].Tick < events[j].Tick
			}
			return eventOrder(events[i].Data) < eventOrder(events[j].Data)
		})
		var tr smf.Track
		var prev int64
		for _, e := range events {
			if e.Tick < 0 {
				return nil, fmt.Errorf("negative MIDI event")
			}
			tr.Add(uint32(e.Tick-prev), e.Data)
			prev = e.Tick
		}
		tr.Close(0)
		f.Add(tr)
	}
	var b bytes.Buffer
	_, err := f.WriteTo(&b)
	return b.Bytes(), err
}
func eventOrder(m smf.Message) int {
	if len(m) == 0 {
		return 0
	}
	if m[0]&0xf0 == 0x80 || (m[0]&0xf0 == 0x90 && len(m) > 2 && m[2] == 0) {
		return 1
	}
	if m[0]&0xf0 == 0x90 {
		return 2
	}
	return 0
}
func (s Song) End() int64 {
	var end int64
	for _, t := range s.Tracks {
		for _, n := range t.Notes {
			end = max(end, n.Tick+n.Duration)
		}
		for _, e := range t.Events {
			end = max(end, e.Tick)
		}
	}
	return max(end, s.PPQ*4)
}
func (s Song) Tempos() []TempoPoint {
	points := []TempoPoint{{0, 0, 120}}
	for _, t := range s.Tracks {
		for _, e := range t.Events {
			var bpm float64
			if e.Data.GetMetaTempo(&bpm) && bpm > 0 {
				points = append(points, TempoPoint{Tick: e.Tick, BPM: bpm})
			}
		}
	}
	sort.SliceStable(points, func(i, j int) bool { return points[i].Tick < points[j].Tick })
	for i := 1; i < len(points); i++ {
		p := points[i-1]
		points[i].Seconds = p.Seconds + float64(points[i].Tick-p.Tick)*60/(p.BPM*float64(s.PPQ))
	}
	return points
}
func secondsAt(points []TempoPoint, ppq int64, tick float64) float64 {
	i := sort.Search(len(points), func(i int) bool { return float64(points[i].Tick) > tick }) - 1
	i = max(i, 0)
	p := points[i]
	return p.Seconds + (tick-float64(p.Tick))*60/(p.BPM*float64(ppq))
}
func tickAt(points []TempoPoint, ppq int64, seconds float64) float64 {
	i := sort.Search(len(points), func(i int) bool { return points[i].Seconds > seconds }) - 1
	i = max(i, 0)
	p := points[i]
	return float64(p.Tick) + (seconds-p.Seconds)*p.BPM*float64(ppq)/60
}
func (s Song) BarTicks() int64 {
	num, den := uint8(4), uint8(4)
	for _, t := range s.Tracks {
		for _, e := range t.Events {
			if e.Tick == 0 {
				e.Data.GetMetaMeter(&num, &den)
			}
		}
	}
	return s.PPQ * 4 * int64(num) / int64(max(den, 1))
}
func (s *Song) SetTempo(bpm float64) {
	for i := range s.Tracks {
		e := s.Tracks[i].Events
		out := e[:0]
		for _, v := range e {
			var b float64
			if v.Tick == 0 && v.Data.GetMetaTempo(&b) {
				continue
			}
			out = append(out, v)
		}
		s.Tracks[i].Events = out
	}
	s.Tracks[0].Events = append(s.Tracks[0].Events, MIDIEvent{0, smf.MetaTempo(bpm)})
}
func (s *Song) SetMeter(num, den uint8) {
	for i := range s.Tracks {
		e := s.Tracks[i].Events
		out := e[:0]
		for _, v := range e {
			var n, d uint8
			if v.Tick == 0 && v.Data.GetMetaMeter(&n, &d) {
				continue
			}
			out = append(out, v)
		}
		s.Tracks[i].Events = out
	}
	s.Tracks[0].Events = append(s.Tracks[0].Events, MIDIEvent{0, smf.MetaMeter(num, den)})
}
func (s Song) Audible(i int) bool {
	if s.Tracks[i].Settings.Mute {
		return false
	}
	solo := false
	for _, t := range s.Tracks {
		solo = solo || t.Settings.Solo
	}
	return !solo || s.Tracks[i].Settings.Solo
}
func cloneSong(s Song) Song { b, _ := json.Marshal(s); var c Song; _ = json.Unmarshal(b, &c); return c }
func snapTick(t float64, grid int64) int64 {
	if grid <= 0 {
		return int64(math.Round(t))
	}
	return int64(math.Round(t/float64(grid))) * grid
}
