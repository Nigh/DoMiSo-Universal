package main

import (
	"encoding/json"
	"fmt"
	"sort"
)

type MappingState struct {
	ID       string         `json:"id"`
	Label    string         `json:"label"`
	Bindings map[string]int `json:"bindings"`
}
type StateTransition struct {
	From     string `json:"from"`
	To       string `json:"to"`
	KeyCode  string `json:"keyCode"`
	ScanCode uint16 `json:"scanCode"`
}
type GameProfile struct {
	Version      int               `json:"version"`
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	ProcessNames []string          `json:"processNames"`
	InitialState string            `json:"initialState"`
	States       []MappingState    `json:"states"`
	Transitions  []StateTransition `json:"transitions"`
	ScanCodes    map[string]uint16 `json:"scanCodes"`
	Timing       struct {
		KeyDownMS          int64 `json:"keyDownMs"`
		ChordGapMS         int64 `json:"chordGapMs"`
		StateSwitchDelayMS int64 `json:"stateSwitchDelayMs"`
	} `json:"timing"`
}

func builtinProfile() GameProfile {
	p := GameProfile{Version: 1, ID: "genshin-windsong-lyre", Name: "原神风物之诗琴", ProcessNames: []string{"YuanShen.exe", "GenshinImpact.exe"}, InitialState: "default", ScanCodes: map[string]uint16{}}
	p.States = []MappingState{{ID: "default", Label: "默认", Bindings: map[string]int{}}}
	rows := []string{"ZXCVBNM", "ASDFGHJ", "QWERTYU"}
	scans := [][]uint16{{44, 45, 46, 47, 48, 49, 50}, {30, 31, 32, 33, 34, 35, 36}, {16, 17, 18, 19, 20, 21, 22}}
	white := []int{0, 2, 4, 5, 7, 9, 11}
	for row, keys := range rows {
		for i, k := range keys {
			code := "Key" + string(k)
			p.States[0].Bindings[code] = 48 + row*12 + white[i]
			p.ScanCodes[code] = scans[row][i]
		}
	}
	p.Timing.KeyDownMS = 35
	p.Timing.ChordGapMS = 8
	p.Timing.StateSwitchDelayMS = 80
	return p
}
func parseGameProfile(b []byte) (GameProfile, error) {
	var p GameProfile
	if len(b) > maxSheetBytes {
		return p, fmt.Errorf("映射文件过大")
	}
	if e := json.Unmarshal(b, &p); e != nil {
		return p, e
	}
	if p.Version != 1 || p.ID == "" || p.Name == "" || len(p.States) == 0 || len(p.ProcessNames) == 0 || p.Timing.KeyDownMS < 1 || p.Timing.KeyDownMS > 1000 || p.Timing.StateSwitchDelayMS < 0 || p.Timing.StateSwitchDelayMS > 5000 || p.Timing.ChordGapMS < 0 || p.Timing.ChordGapMS > 1000 {
		return p, fmt.Errorf("无效映射配置")
	}
	states := map[string]bool{}
	for _, s := range p.States {
		if s.ID == "" || states[s.ID] {
			return p, fmt.Errorf("映射状态重复或为空")
		}
		states[s.ID] = true
		for k, v := range s.Bindings {
			if v < 0 || v > 127 || p.ScanCodes[k] == 0 {
				return p, fmt.Errorf("无效音高或 scanCode")
			}
		}
	}
	if !states[p.InitialState] {
		return p, fmt.Errorf("初始状态不存在")
	}
	for _, t := range p.Transitions {
		if !states[t.From] || !states[t.To] || t.ScanCode == 0 || t.KeyCode == "" {
			return p, fmt.Errorf("无效状态迁移")
		}
	}
	return p, nil
}
func (p GameProfile) Binding(state, key string) (int, string, bool) {
	for _, t := range p.Transitions {
		if t.From == state && t.KeyCode == key {
			return 0, t.To, false
		}
	}
	for _, s := range p.States {
		if s.ID == state {
			n, ok := s.Bindings[key]
			return n, state, ok
		}
	}
	return 0, state, false
}
func (p GameProfile) KeyFor(state string, note int) string {
	for _, s := range p.States {
		if s.ID == state {
			keys := []string{}
			for k, n := range s.Bindings {
				if n == note {
					keys = append(keys, k)
				}
			}
			sort.Strings(keys)
			if len(keys) > 0 {
				return keys[0]
			}
		}
	}
	return ""
}
func (p GameProfile) Path(from, to string) ([]StateTransition, bool) {
	type step struct {
		State string
		Path  []StateTransition
	}
	queue := []step{{State: from}}
	seen := map[string]bool{from: true}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur.State == to {
			return cur.Path, true
		}
		for _, t := range p.Transitions {
			if t.From == cur.State && !seen[t.To] {
				seen[t.To] = true
				path := append(append([]StateTransition(nil), cur.Path...), t)
				queue = append(queue, step{t.To, path})
			}
		}
	}
	return nil, false
}

type Chord struct {
	Tick  int64
	Notes []int
}

func songChords(s Song, start int64) []Chord {
	groups := map[int64]map[int]bool{}
	for i, t := range s.Tracks {
		if !s.Audible(i) {
			continue
		}
		for _, n := range t.Notes {
			if n.Tick < start {
				continue
			}
			if groups[n.Tick] == nil {
				groups[n.Tick] = map[int]bool{}
			}
			groups[n.Tick][int(n.Key)] = true
		}
	}
	out := []Chord{}
	for tick, notes := range groups {
		g := Chord{Tick: tick}
		for n := range notes {
			g.Notes = append(g.Notes, n)
		}
		sort.Ints(g.Notes)
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Tick < out[j].Tick })
	return out
}
func matchesFreshChord(target []int, pressed, fresh map[int]bool) bool {
	if len(target) != len(pressed) {
		return false
	}
	hasFresh := false
	for _, n := range target {
		if !pressed[n] {
			return false
		}
		hasFresh = hasFresh || fresh[n]
	}
	return hasFresh
}
func buildKeySequence(s Song, p GameProfile, start int64) ([]KeyAction, error) {
	tempos := s.Tempos()
	origin := secondsAt(tempos, s.PPQ, float64(start))
	state := p.InitialState
	var actions []KeyAction
	ends := map[[2]int64]int64{}
	for i, t := range s.Tracks {
		if !s.Audible(i) {
			continue
		}
		for _, n := range t.Notes {
			k := [2]int64{n.Tick, int64(n.Key)}
			ends[k] = max(ends[k], n.Tick+n.Duration)
		}
	}
	for _, g := range songChords(s, start) {
		target := ""
		ordered := append([]MappingState(nil), p.States...)
		sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].ID == state && ordered[j].ID != state })
		for _, candidate := range ordered {
			ok := true
			for _, n := range g.Notes {
				ok = ok && p.KeyFor(candidate.ID, n) != ""
			}
			if ok {
				target = candidate.ID
				break
			}
		}
		if target == "" {
			return nil, fmt.Errorf("tick %d 的和弦无法映射", g.Tick)
		}
		path, ok := p.Path(state, target)
		if !ok {
			return nil, fmt.Errorf("无法从 %s 切换到 %s", state, target)
		}
		at := int64((secondsAt(tempos, s.PPQ, float64(g.Tick)) - origin) * 1000)
		for _, t := range path {
			actions = append(actions, KeyAction{at, t.ScanCode, true}, KeyAction{at + p.Timing.KeyDownMS, t.ScanCode, false})
			at += p.Timing.StateSwitchDelayMS
		}
		state = target
		for _, n := range g.Notes {
			end := max(at+p.Timing.KeyDownMS, int64((secondsAt(tempos, s.PPQ, float64(ends[[2]int64{g.Tick, int64(n)}]))-origin)*1000))
			scan := p.ScanCodes[p.KeyFor(state, n)]
			actions = append(actions, KeyAction{at, scan, true}, KeyAction{end, scan, false})
		}
	}
	sort.SliceStable(actions, func(i, j int) bool {
		if actions[i].AtMS != actions[j].AtMS {
			return actions[i].AtMS < actions[j].AtMS
		}
		return !actions[i].Down && actions[j].Down
	})
	return actions, nil
}
