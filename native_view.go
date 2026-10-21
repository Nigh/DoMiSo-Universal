package main

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"math"
	"runtime"
	"sort"
	"strings"
	"time"
)

var palette = struct{ Background, Surface, Grid, Text, Muted, Accent, Selected ui.Color }{ui.Hex("#141419"), ui.Hex("#202028"), ui.Hex("#33333f"), ui.Hex("#eeeaf5"), ui.Hex("#aaa4b5"), ui.Hex("#b5a2ee"), ui.Hex("#f2d49b")}

func button(c *ui.Context, label string, fn func()) {
	if ui.Button(c, label).Clicked() {
		fn()
	}
}
func (e *Editor) view(c *ui.Context) {
	e.buildFocus = false
	theme := ui.DarkTheme()
	theme.Background = palette.Background
	theme.Surface = palette.Surface
	theme.Accent = palette.Accent
	theme.Text = palette.Text
	theme.Radius = 5
	c.SetTheme(theme)
	e.practiceFrame()
	tick, playing := e.transportState()
	if playing || e.recording {
		c.After(time.Second / 60)
	}
	ui.Column(c).Fill().Gap(6).Padding(8).Children(func() {
		ui.Row(c).Gap(6).Children(func() {
			ui.Text(c, "DoMiSo").FontSize(22).Bold()
			button(c, "打开", e.open)
			button(c, "保存", func() { e.save(false) })
			button(c, "另存为", func() { e.save(true) })
			button(c, "导出 MIDI", func() { e.export(true) })
			button(c, "导出原文", func() { e.export(false) })
			ui.Spacer(c)
			play := "播放"
			if playing {
				play = "暂停"
			}
			button(c, play, e.togglePlay)
			button(c, "停止", e.stop)
			if ui.Checkbox(c, &e.recording, "录音").Changed() {
				if e.recording {
					e.checkpoint()
					e.audio.Play(true)
				} else {
					e.finishRecording()
				}
			}
			ui.Textf(c, "tick %.0f", tick).Width(95)
		})
		ui.Row(c).Gap(6).Children(func() {
			button(c, "撤销", func() { e.history(false) })
			button(c, "重做", func() { e.history(true) })
			ui.Select(c, &e.mode, []string{"编辑", "播放"}).Width(80)
			ui.Text(c, "吸附")
			ui.Select(c, &e.grid, []string{"自由", "1/16", "1/8", "1/4", "1/2"}).Width(75)
			ui.Text(c, "标签")
			ui.Select(c, &e.label, []string{"隐藏", "音名", "简谱", "按键"}).Width(80)
			ui.NumberInput(c, &e.tonic, 0, 127, 1).Width(65).Label("简谱主音 MIDI")
			button(c, "−", func() { e.zoom = max(0.02, e.zoom/1.3) })
			button(c, "+", func() { e.zoom = min(4, e.zoom*1.3) })
			ui.Checkbox(c, &e.follow, "跟随")
			if ui.Checkbox(c, &e.loop, "循环").Changed() {
				e.audio.SetLoop(e.loop, e.loopStart, e.loopEnd)
			}
			if ui.NumberInput(c, &e.loopStart, 0, float64(e.doc.Song.End()), float64(e.doc.Song.PPQ)).Width(85).Label("循环起点").Changed() {
				e.audio.SetLoop(e.loop, e.loopStart, e.loopEnd)
			}
			if ui.NumberInput(c, &e.loopEnd, 1, float64(e.doc.Song.End()+e.doc.Song.PPQ*16), float64(e.doc.Song.PPQ)).Width(85).Label("循环终点").Changed() {
				e.audio.SetLoop(e.loop, e.loopStart, e.loopEnd)
			}
		})
		ui.Row(c).Grow(1).MinHeight(200).Gap(8).AlignItems(ui.Stretch).Children(func() {
			ui.Column(c).Width(270).Gap(6).Disabled(e.busy).Children(func() { e.sidebar(c) })
			ui.Column(c).Grow(1).Gap(5).Children(func() {
				e.trackBar(c)
				roll := ui.Box(c).Grow(1).MinHeight(140).Clip().Focusable().Label("钢琴卷帘；双击创建，拖动移动，右边缘改变时长，空白拖动框选").HandleInput(e.rollInput).Draw(func(p *ui.Painter, r ui.Rect) { e.rollWidth, e.rollHeight = r.W, r.H; e.drawRoll(p, r, tick, playing) })
				e.buildFocus = e.buildFocus || roll.Focused()
				e.keyboard(c, tick)
			})
		})
		e.performanceBar(c)
		state := ""
		if e.doc.Dirty {
			state = "● 未保存  "
		}
		if e.doc.OutOfSync {
			state += "文本仅供参考  "
		}
		if e.busy {
			state += "正在加载音源…  "
		}
		ui.Text(c, state+e.message).FontSize(12).TextColor(palette.Muted).SingleLine()
	})
	if e.inputFocused && !e.buildFocus && len(e.keyDown) > 0 {
		e.releaseKeys()
	}
	e.inputFocused = e.buildFocus
}
func (e *Editor) sidebar(c *ui.Context) {
	ui.Text(c, "曲谱").Bold()
	names := []string{"选择内置曲谱…"}
	for _, s := range e.sheets {
		names = append(names, s.Name)
	}
	if ui.Select(c, &e.sheet, names).Changed() {
		for _, s := range e.sheets {
			if s.Name == e.sheet && e.confirmUnsaved() {
				text, err := (&AppService{}).LoadSheet(s.Path)
				if err != nil {
					e.message = err.Error()
					break
				}
				b, err := (&AppService{}).ParseTextToMIDI(text)
				if err != nil {
					e.message = err.Error()
					break
				}
				song, err := readSong(b)
				if err != nil {
					e.message = err.Error()
					break
				}
				e.stop()
				e.installDocument(Document{Song: song, Notation: text, Generated: true})
				break
			}
		}
	}
	if ui.TextInput(c, &e.doc.Meta.Title).Label("标题").Placeholder("标题").Changed() {
		e.changed(false)
	}
	if ui.TextInput(c, &e.doc.Meta.Composer).Label("记谱 / 作曲").Placeholder("记谱 / 作曲").Changed() {
		e.changed(false)
	}
	if ui.TextInput(c, &e.doc.Meta.Arranger).Label("编曲").Placeholder("编曲").Changed() {
		e.changed(false)
	}
	if ui.TextInput(c, &e.doc.Meta.Description).Label("说明").Placeholder("说明").Changed() {
		e.changed(false)
	}
	ui.Row(c).Gap(4).Children(func() {
		ui.Text(c, "BPM")
		if ui.NumberInput(c, &e.tempo, 20, 400, 1).Width(75).Changed() {
			e.checkpoint()
			e.doc.Song.SetTempo(e.tempo)
			e.changed(true)
		}
		if ui.NumberInput(c, &e.meterNum, 1, 32, 1).Width(55).Label("拍号分子").Changed() {
			e.setMeter()
		}
		if ui.Select(c, &e.meterDen, []string{"2", "4", "8", "16"}).Width(55).Changed() {
			e.setMeter()
		}
	})
	ui.Text(c, "DoMiSo 文本").Bold()
	if ui.TextArea(c, &e.doc.Notation).Grow(1).MinHeight(90).Font("monospace").Label("DoMiSo 乐谱源文").Changed() {
		e.changed(false)
		e.doc.OutOfSync = e.doc.Notation != ""
	}
	button(c, "从文本重新生成 MIDI", e.regenerate)
	ui.Text(c, "轨道").Bold()
	ui.Scroll(c).Height(135).Children(func() {
		for i := range e.doc.Song.Tracks {
			func() {
				c.Key(i)
				t := &e.doc.Song.Tracks[i]
				label := fmt.Sprintf("%d  %s  (%d)", i+1, t.Name, len(t.Notes))
				if i == e.track {
					label = "▸ " + label
				}
				button(c, label, func() { e.releaseKeys(); e.track = i; clear(e.selected) })
			}()
		}
	})
	ui.Row(c).Gap(4).Children(func() {
		button(c, "新增", func() {
			e.checkpoint()
			e.stop()
			e.doc.Song.Tracks = append(e.doc.Song.Tracks, newTrack(fmt.Sprintf("Track %d", len(e.doc.Song.Tracks)+1)))
			e.track = len(e.doc.Song.Tracks) - 1
			e.changed(true)
			e.configureAudio()
		})
		button(c, "删除", func() {
			if len(e.doc.Song.Tracks) > 1 {
				e.checkpoint()
				e.stop()
				e.doc.Song.Tracks = append(e.doc.Song.Tracks[:e.track], e.doc.Song.Tracks[e.track+1:]...)
				e.track = min(e.track, len(e.doc.Song.Tracks)-1)
				clear(e.selected)
				e.changed(true)
				e.configureAudio()
			}
		})
		button(c, "↑", func() { e.moveTrack(-1) })
		button(c, "↓", func() { e.moveTrack(1) })
	})
	ui.Row(c).Gap(4).Children(func() {
		ui.Text(c, "音频缓冲 ms")
		if ui.Select(c, &e.buffer, []string{"20", "40", "80"}).Width(65).Changed() {
			e.saveSettings()
			e.message = "缓冲设置重启后生效"
		}
	})
}
func (e *Editor) setMeter() {
	var den int
	fmt.Sscan(e.meterDen, &den)
	e.checkpoint()
	e.doc.Song.SetMeter(uint8(e.meterNum), uint8(den))
	e.changed(true)
}
func (e *Editor) moveTrack(delta int) {
	to := e.track + delta
	if to < 0 || to >= len(e.doc.Song.Tracks) || e.busy {
		return
	}
	e.checkpoint()
	e.stop()
	e.doc.Song.Tracks[e.track], e.doc.Song.Tracks[to] = e.doc.Song.Tracks[to], e.doc.Song.Tracks[e.track]
	e.track = to
	e.changed(true)
	e.configureAudio()
}
func (e *Editor) trackBar(c *ui.Context) {
	t := &e.doc.Song.Tracks[e.track]
	ui.Row(c).Gap(5).Disabled(e.busy).Children(func() {
		name := t.Name
		if ui.TextInput(c, &name).Width(130).Label("轨道名称").Changed() {
			e.checkpoint()
			t.Name = name
			e.changed(true)
		}
		mute := t.Settings.Mute
		if ui.Checkbox(c, &mute, "静音").Changed() {
			e.checkpoint()
			t.Settings.Mute = mute
			e.changed(true)
		}
		solo := t.Settings.Solo
		if ui.Checkbox(c, &solo, "独奏").Changed() {
			e.checkpoint()
			t.Settings.Solo = solo
			e.changed(true)
		}
		if ui.Slider(c, &t.Settings.Volume, 0, 1).Width(90).Label("轨道音量").Changed() {
			e.changed(false)
			e.audio.Update(e.doc.Song)
		}
		button(c, "选择音源…", e.setInstrument)
		button(c, "内置音", func() {
			e.checkpoint()
			t.Settings.Instrument = Instrument{Kind: "builtin"}
			e.changed(false)
			e.configureAudio()
		})
		ui.Text(c, "原始音高")
		if ui.NumberInput(c, &e.sampleRoot, 0, 127, 1).Width(65).Changed() {
			if t.Settings.Instrument.Kind == "sample" {
				e.checkpoint()
				t.Settings.Instrument.RootKey = int(e.sampleRoot)
				e.changed(false)
				e.configureAudio()
			}
		}
		gated := t.Settings.Instrument.Gated
		if ui.Checkbox(c, &gated, "随音符结束").Changed() {
			e.checkpoint()
			t.Settings.Instrument.Gated = gated
			e.changed(false)
			e.configureAudio()
		}
	})
	ui.Row(c).Gap(6).Children(func() {
		ui.Text(c, "音源: "+instrumentLabel(t.Settings.Instrument)).FontSize(12).TextColor(palette.Muted)
		ui.Spacer(c)
		if len(e.selected) > 0 {
			var vel float64 = 100
			for _, n := range t.Notes {
				if e.selected[n.ID] {
					vel = float64(n.Velocity)
					break
				}
			}
			ui.Textf(c, "已选 %d  力度", len(e.selected))
			if ui.NumberInput(c, &vel, 1, 127, 1).Width(65).Changed() {
				e.edit(func() {
					for i := range t.Notes {
						if e.selected[t.Notes[i].ID] {
							t.Notes[i].Velocity = uint8(vel)
						}
					}
				})
			}
			button(c, "删除音符", e.deleteSelection)
		}
	})
}
func instrumentLabel(in Instrument) string {
	if in.Kind == "" || in.Kind == "builtin" {
		return "内置合成音"
	}
	return in.File
}
func (e *Editor) performanceBar(c *ui.Context) {
	ui.Row(c).Gap(6).Disabled(e.busy).Children(func() {
		names := []string{}
		for _, p := range e.profiles {
			names = append(names, p.Name)
		}
		if ui.Select(c, &e.profileName, names).Width(160).Changed() {
			e.releaseKeys()
			performance.stop()
			performance.mu.Lock()
			performance.actions = nil
			performance.mu.Unlock()
			e.autoRunning = false
			e.state = e.profile().InitialState
			e.groups = nil
			if e.gamePractice {
				if err := (&AppService{}).SetGamePracticeMonitoring(true, e.profile().ProcessNames); err != nil {
					e.message = err.Error()
				}
			}
		}
		states := []string{}
		for _, s := range e.profile().States {
			states = append(states, s.ID)
		}
		if len(states) > 1 {
			if ui.Select(c, &e.state, states).Width(90).Changed() {
				e.releaseKeys()
				e.groups = nil
			}
		}
		button(c, "导入映射", e.importProfile)
		button(c, "导出映射", e.exportProfile)
		if ui.Checkbox(c, &e.practice, "练习").Changed() {
			if !e.practice && e.audio.Waiting() {
				e.audio.WaitAt(-1)
				e.audio.Play(true)
			}
			e.groups = nil
			e.practiceWait = false
		}
		if ui.Checkbox(c, &e.waitMode, "等音").Changed() {
			if !e.waitMode && e.audio.Waiting() {
				e.audio.WaitAt(-1)
				e.audio.Play(true)
			}
		}
		if ui.Checkbox(c, &e.gamePractice, "游戏监听").Disabled(runtime.GOOS != "windows").Changed() {
			e.releaseKeys()
			if err := (&AppService{}).SetGamePracticeMonitoring(e.gamePractice, e.profile().ProcessNames); err != nil {
				e.gamePractice = false
				e.message = err.Error()
			}
		}
		if runtime.GOOS == "windows" {
			button(c, "准备自动演奏", e.prepareAuto)
			button(c, "紧急停止", func() { performance.stop() })
		}
		if e.practice {
			for i := e.group; i < min(e.group+4, len(e.groups)); i++ {
				g := e.groups[i]
				labels := []string{}
				for _, n := range g.Notes {
					v := e.profile().KeyFor(e.state, n)
					if v == "" {
						v = fmt.Sprint(n)
					}
					labels = append(labels, strings.TrimPrefix(v, "Key"))
				}
				ui.Text(c, strings.Join(labels, "+")).TextColor(palette.Accent)
			}
		}
	})
}
func (e *Editor) drawRoll(p *ui.Painter, r ui.Rect, tick float64, playing bool) {
	e.ensureRollIndex()
	e.ensureLabelCache()
	e.scrollX = max(-float32(e.doc.Song.BarTicks()*2)*e.zoom, e.scrollX)
	if e.follow && playing && e.drag == "" {
		e.scrollX = float32(tick)*e.zoom - (r.W-52)/2
	}
	p.Fill(r, palette.Background, 0)
	p.Clip(r, 0, func() {
		for key := 0; key < 128; key++ {
			y := r.Y + 26 + float32(127-key)*e.rowHeight - e.scrollY
			if y+e.rowHeight < r.Y+26 || y > r.Y+r.H {
				continue
			}
			shade := palette.Surface
			if isBlack(key) {
				shade = palette.Background
			}
			p.Fill(ui.Rect{X: r.X, Y: y, W: r.W, H: e.rowHeight}, shade, 0)
			p.Line(r.X, y, r.X+r.W, y, 1, palette.Grid)
			p.Text(r.X+3, y+3, fmt.Sprint(key), 10, palette.Muted)
		}
		// Grid density follows zoom; drawing is bounded by the visible viewport.
		step := e.doc.Song.PPQ
		for float32(step)*e.zoom < 35 {
			step *= 2
		}
		left := int64(math.Floor(float64(e.scrollX/e.zoom)/float64(step))) * step
		for t := left; float32(t)*e.zoom-e.scrollX < r.W; t += step {
			x := r.X + 52 + float32(t)*e.zoom - e.scrollX
			if x < r.X+52 {
				continue
			}
			p.Line(x, r.Y+26, x, r.Y+r.H, 1, palette.Grid)
		}
		barsInRange(e.meters, int64(e.scrollX/e.zoom), int64((e.scrollX+r.W)/e.zoom), func(tick, bar int64) {
			x := r.X + 52 + float32(tick)*e.zoom - e.scrollX
			if x >= r.X+52 {
				p.Line(x, r.Y+26, x, r.Y+r.H, 1, palette.Muted)
				p.Text(x+3, r.Y+4, fmt.Sprint(bar), 11, palette.Muted)
			}
		})
		drawNote := func(n Note, ti int, color ui.Color) {
			nr := e.noteRect(n)
			nr.X += r.X
			nr.Y += r.Y
			if nr.X+nr.W < r.X+52 || nr.X > r.X+r.W || nr.Y+nr.H < r.Y+26 || nr.Y > r.Y+r.H {
				return
			}
			if ti == e.track && e.selected[n.ID] {
				color = palette.Selected
			}
			p.Fill(nr, color, 3)
			if ti == e.track && nr.W > 18 && e.label != "隐藏" {
				text := e.labelCache[n.Key]
				p.Clip(nr, 0, func() { p.Text(max(nr.X+4, r.X+54), nr.Y+2, text, 11, palette.Background) })
			}
		}
		for ti := range e.doc.Song.Tracks {
			color := palette.Accent
			if ti != e.track {
				color = ui.RGBA(145, 130, 180, 0.25)
			}
			for _, n := range e.visibleNotes(ti, float64(e.scrollX/e.zoom), float64((e.scrollX+r.W)/e.zoom)) {
				if ti == e.track && e.dragBefore != nil && e.selected[n.ID] {
					continue
				}
				drawNote(n, ti, color)
			}
		}
		if e.dragBefore != nil {
			for _, i := range e.dragIndices {
				drawNote(e.doc.Song.Tracks[e.track].Notes[i], e.track, palette.Selected)
			}
		}
		for key, start := range e.recordStart {
			drawNote(Note{Tick: start, Duration: max(1, int64(tick)-start), Key: uint8(key)}, e.track, palette.Selected)
		}
		if e.drag == "box" {
			x, y := min(e.dragX, e.boxX), min(e.dragY, e.boxY)
			p.Stroke(ui.Rect{X: r.X + x, Y: r.Y + y, W: absf(e.boxX - e.dragX), H: absf(e.boxY - e.dragY)}, palette.Selected, 0, 1)
		}
		x := r.X + 52 + float32(tick)*e.zoom - e.scrollX
		if x >= r.X+52 {
			p.Line(x, r.Y, x, r.Y+r.H, 2, palette.Selected)
		}
		p.Fill(ui.Rect{X: r.X, Y: r.Y, W: 52, H: 26}, palette.Surface, 0)
	})
}
func isBlack(k int) bool {
	switch k % 12 {
	case 1, 3, 6, 8, 10:
		return true
	}
	return false
}
func absf(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
func (e *Editor) noteRect(n Note) ui.Rect {
	return ui.Rect{X: 52 + float32(n.Tick)*e.zoom - e.scrollX, Y: 26 + float32(127-int(n.Key))*e.rowHeight - e.scrollY + 1, W: max(3, float32(n.Duration)*e.zoom), H: e.rowHeight - 2}
}
func (e *Editor) tickAtX(x float32) float64 { return float64((x - 52 + e.scrollX) / e.zoom) }
func (e *Editor) keyAtY(y float32) int {
	return max(0, min(127, 127-int((y-26+e.scrollY)/e.rowHeight)))
}
func (e *Editor) rollInput(ev ui.InputEvent) bool {
	if e.busy {
		return true
	}
	if ev.Kind == ui.InputKeyDown || ev.Kind == ui.InputKeyUp {
		return e.keyInput(ev)
	}
	switch ev.Kind {
	case ui.InputScroll:
		if ev.Mods&ui.Ctrl != 0 {
			e.zoom = max(0.02, min(4, e.zoom*float32(math.Exp(-float64(ev.DY)/250))))
		} else if ev.Mods&ui.Shift != 0 {
			e.scrollX += ev.DY
		} else {
			e.scrollY = max(0, min(128*e.rowHeight-e.rollHeight+26, e.scrollY+ev.DY))
		}
		return true
	case ui.InputPointerDown:
		e.dragX, e.dragY, e.dragScrollX, e.dragScrollY = ev.X, ev.Y, e.scrollX, e.scrollY
		e.dragTick = e.tickAtX(ev.X)
		if ev.Y < 26 {
			e.drag = "seek"
			if ev.Mods&ui.Ctrl != 0 {
				e.drag = "loopBegin"
				e.loopStart = max(0, e.dragTick)
			} else if ev.Mods&ui.Alt != 0 {
				e.drag = "loopEnd"
				e.loopEnd = max(1, e.dragTick)
			} else {
				e.beginScrub()
				e.seek(e.dragTick)
			}
			e.audio.SetLoop(e.loop, e.loopStart, e.loopEnd)
			return true
		}
		if e.mode == "播放" || ev.Button == 2 {
			e.drag = "pan"
			if e.mode == "播放" {
				e.beginScrub()
			}
			e.follow = false
			return true
		}
		notes := e.doc.Song.Tracks[e.track].Notes
		hit := -1
		for i := len(notes) - 1; i >= 0; i-- {
			r := e.noteRect(notes[i])
			if ev.X >= r.X && ev.X <= r.X+r.W && ev.Y >= r.Y && ev.Y <= r.Y+r.H {
				hit = i
				break
			}
		}
		if hit >= 0 {
			n := notes[hit]
			if ev.Button == 1 {
				e.selected = map[int]bool{n.ID: true}
				e.deleteSelection()
				return true
			}
			if ev.Mods&ui.Shift != 0 {
				e.selected[n.ID] = !e.selected[n.ID]
			} else if !e.selected[n.ID] {
				e.selected = map[int]bool{n.ID: true}
			}
			d := snapshot(e.doc)
			e.dragBefore = &d
			e.dragChanged = false
			e.dragNotes = append([]Note(nil), notes...)
			e.dragIndices = nil
			for i, n := range notes {
				if e.selected[n.ID] {
					e.dragIndices = append(e.dragIndices, i)
				}
			}
			e.drag = "move"
			if ev.X >= e.noteRect(n).X+e.noteRect(n).W-6 {
				e.drag = "resize"
			}
			return true
		}
		if ev.Clicks >= 2 && e.dragTick >= 0 && ev.Button == 0 {
			e.edit(func() {
				s := &e.doc.Song
				s.NextID++
				duration := e.gridTicks()
				if duration == 0 {
					duration = s.PPQ
				}
				n := Note{s.NextID, max(0, snapTick(e.dragTick, e.gridTicks())), duration, uint8(e.keyAtY(ev.Y)), 100, uint8(e.track % 16)}
				s.Tracks[e.track].Notes = append(s.Tracks[e.track].Notes, n)
				e.selected = map[int]bool{n.ID: true}
			})
			return true
		}
		if ev.Mods&ui.Shift == 0 {
			clear(e.selected)
		}
		e.drag = "box"
		e.boxBase = map[int]bool{}
		for id, v := range e.selected {
			e.boxBase[id] = v
		}
		e.boxX, e.boxY = ev.X, ev.Y
		return true
	case ui.InputPointerMove:
		switch e.drag {
		case "loopBegin":
			e.loopStart = max(0, e.tickAtX(ev.X))
			e.audio.SetLoop(e.loop, e.loopStart, e.loopEnd)
		case "loopEnd":
			e.loopEnd = max(1, e.tickAtX(ev.X))
			e.audio.SetLoop(e.loop, e.loopStart, e.loopEnd)
		case "seek":
			e.seek(e.tickAtX(ev.X))
		case "pan":
			e.scrollX = e.dragScrollX - (ev.X - e.dragX)
			e.scrollY = max(0, e.dragScrollY-(ev.Y-e.dragY))
			if e.mode == "播放" {
				e.seek(e.tickAtX(e.rollWidth/2 + 26))
			}
		case "box":
			e.boxX, e.boxY = ev.X, ev.Y
			clear(e.selected)
			for id, v := range e.boxBase {
				e.selected[id] = v
			}
			for _, n := range e.doc.Song.Tracks[e.track].Notes {
				r := e.noteRect(n)
				if r.X <= max(e.dragX, ev.X) && r.X+r.W >= min(e.dragX, ev.X) && r.Y <= max(e.dragY, ev.Y) && r.Y+r.H >= min(e.dragY, ev.Y) {
					e.selected[n.ID] = true
				}
			}
		case "move", "resize":
			e.dragChanged = false
			delta := snapTick(float64((ev.X-e.dragX)/e.zoom), e.gridTicks())
			dk := int(math.Round(float64((e.dragY - ev.Y) / e.rowHeight)))
			for _, i := range e.dragIndices {
				orig := e.dragNotes[i]
				n := orig
				if e.drag == "move" {
					n.Tick = max(0, orig.Tick+delta)
					n.Key = uint8(max(0, min(127, int(orig.Key)+dk)))
				} else {
					n.Duration = max(1, orig.Duration+delta)
				}
				e.doc.Song.Tracks[e.track].Notes[i] = n
				e.dragChanged = e.dragChanged || n != orig
			}
		}
		return e.drag != ""
	case ui.InputPointerUp:
		e.finishDrag()
		return true
	}
	return false
}
func (e *Editor) finishDrag() {
	if e.drag == "seek" || e.drag == "pan" && e.mode == "播放" {
		tick, _ := e.audio.State()
		e.audio.SeekTick(tick)
		e.audio.Play(e.dragPlaying)
	}
	e.rollNotes = nil
	if e.dragBefore != nil && e.dragChanged {
		e.undo = append(e.undo, *e.dragBefore)
		e.redo = nil
		e.changed(true)
	}
	e.dragBefore = nil
	e.dragNotes = nil
	e.dragIndices = nil
	e.drag = ""
}
func (e *Editor) beginScrub() {
	tick, on := e.audio.State()
	e.dragPlaying = on
	e.audio.Play(false)
	e.audio.PreviewTick(tick)
}
func (e *Editor) keyInput(ev ui.InputEvent) bool {
	if e.busy {
		return true
	}
	if ev.Kind == ui.InputKeyDown && !ev.Repeat {
		if ev.Key == ui.KeySpace {
			e.togglePlay()
			return true
		}
		if ev.Key == ui.KeyEscape {
			e.stop()
			return true
		}
		if ev.Mods&ui.Ctrl != 0 {
			switch ev.Key {
			case ui.KeyZ:
				e.history(ev.Mods&ui.Shift != 0)
			case ui.KeyY:
				e.history(true)
			case ui.KeyS:
				e.save(false)
			case ui.KeyC:
				e.copySelection(false)
			case ui.KeyX:
				if e.mode == "编辑" {
					e.copySelection(true)
				}
			case ui.KeyV:
				e.paste()
			case ui.KeyA:
				for _, n := range e.doc.Song.Tracks[e.track].Notes {
					e.selected[n.ID] = true
				}
			default:
				return false
			}
			return true
		}
		if e.mode == "编辑" {
			switch ev.Key {
			case ui.KeyDelete, ui.KeyBackspace:
				e.deleteSelection()
				return true
			case ui.KeyUp:
				e.transpose(1)
				return true
			case ui.KeyDown:
				e.transpose(-1)
				return true
			}
		}
	}
	k := ev.PhysicalKey
	if k == ui.KeyUnknown {
		k = ev.Key
	}
	if k >= ui.KeyA && k <= ui.KeyZ && ev.Mods&(ui.Ctrl|ui.Alt|ui.Super) == 0 {
		code := "Key" + string(rune('A'+k-ui.KeyA))
		if !e.gamePractice {
			e.noteInput(code, ev.Kind == ui.InputKeyDown, false)
		}
		return true
	}
	return false
}
func (e *Editor) keyboard(c *ui.Context, tick float64) {
	e.ensureRollIndex()
	_, playing := e.transportState()
	if tick < e.displayTick || e.drag != "" {
		clear(e.flashes)
	}
	p := e.profile()
	active := map[int]bool{}
	for i := range e.doc.Song.Tracks {
		if !e.doc.Song.Audible(i) {
			continue
		}
		for _, n := range e.visibleNotes(i, tick, tick+1) {
			if float64(n.Tick) <= tick && float64(n.Tick+n.Duration) > tick {
				active[int(n.Key)] = true
			}
		}
		if playing && e.drag == "" && tick >= e.displayTick {
			for _, n := range e.visibleNotes(i, e.displayTick, tick) {
				if float64(n.Tick) <= tick && (float64(n.Tick) > e.displayTick || !e.displayPlaying && float64(n.Tick) == e.displayTick) {
					e.flashes[int(n.Key)] = time.Now()
				}
			}
		}
	}
	e.displayTick, e.displayPlaying = tick, playing
	keyboard := ui.Box(c).Height(116).Focusable().HandleInput(func(ev ui.InputEvent) bool { return e.keyInput(ev) }).Draw(func(painter *ui.Painter, r ui.Rect) {
		rows := []string{"QWERTYU", "ASDFGHJ", "ZXCVBNM"}
		width := min(float32(65), (r.W-40)/7)
		for row, keys := range rows {
			for col, k := range keys {
				code := "Key" + string(k)
				n, _, ok := p.Binding(e.state, code)
				if !ok {
					continue
				}
				rect := ui.Rect{X: r.X + float32(col)*width + float32(row)*10, Y: r.Y + float32(row)*37, W: width - 5, H: 33}
				color := palette.Surface
				if e.pressed[n] || active[n] {
					color = palette.Accent
				}
				if flash, ok := e.flashes[n]; ok && time.Since(flash) < 120*time.Millisecond {
					color = palette.Selected
					c.After(time.Second / 60)
				}
				painter.Fill(rect, color, 4)
				painter.Text(rect.X+8, rect.Y+7, fmt.Sprintf("%c  %s", k, noteLabel(n, "简谱", int(e.tonic), p, e.state)), 12, palette.Text)
			}
		}
	})
	e.buildFocus = e.buildFocus || keyboard.Focused()
}
func (e *Editor) ensureLabelCache() {
	key := fmt.Sprintf("%s/%s/%s/%d", e.label, e.profileName, e.state, int(e.tonic))
	if key == e.labelKey {
		return
	}
	e.labelKey = key
	p := e.profile()
	for n := range 128 {
		e.labelCache[n] = noteLabel(n, e.label, int(e.tonic), p, e.state)
	}
}

func (e *Editor) ensureRollIndex() {
	if e.rollNotes != nil {
		return
	}
	e.meters = e.doc.Song.Meters()
	e.rollNotes = make([][]Note, len(e.doc.Song.Tracks))
	e.rollEnds = make([][]int64, len(e.rollNotes))
	for i, t := range e.doc.Song.Tracks {
		notes := append([]Note(nil), t.Notes...)
		sort.Slice(notes, func(i, j int) bool { return notes[i].Tick < notes[j].Tick })
		ends := make([]int64, len(notes))
		var end int64
		for j, n := range notes {
			end = max(end, n.Tick+n.Duration)
			ends[j] = end
		}
		e.rollNotes[i], e.rollEnds[i] = notes, ends
	}
}
func (e *Editor) visibleNotes(track int, left, right float64) []Note {
	notes := e.rollNotes[track]
	lo := sort.Search(len(notes), func(i int) bool { return float64(e.rollEnds[track][i]) >= left })
	hi := sort.Search(len(notes), func(i int) bool { return float64(notes[i].Tick) > right })
	return notes[lo:max(lo, hi)]
}
