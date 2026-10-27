package main

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"math"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Xianii dark tokens: https://github.com/Nigh/xianii-theme (OKLCH converted to sRGB).
var palette = struct{ Background, Surface, Grid, Text, Muted, Accent, Selected ui.Color }{ui.Hex("#242424"), ui.Hex("#161616"), ui.Hex("#404040"), ui.Hex("#f2f2f2"), ui.Hex("#b0afb0"), ui.Hex("#ffa1ad"), ui.Hex("#7fcfc4")}

var (
	playIcon         = ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24"><path d="M6 3v18l15-9z"/></svg>`))
	pauseIcon        = ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24"><path d="M5 3h5v18H5zm9 0h5v18h-5z"/></svg>`))
	tonicNamePattern = regexp.MustCompile(`(?i)^\s*(?:1\s*=\s*)?([a-g])([#b]?)(-?\d+)([#b]?)\s*$`)
)

func tonicName(key int) string {
	return fmt.Sprintf("%s%d", []string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}[key%12], key/12-1)
}

func tonicKey(text string) (int, bool) {
	text = strings.TrimSpace(text)
	if key, err := strconv.Atoi(text); err == nil {
		return key, key >= 0 && key <= 127
	}
	match := tonicNamePattern.FindStringSubmatch(text)
	if match == nil || match[2] != "" && match[4] != "" {
		return 0, false
	}
	octave, err := strconv.Atoi(match[3])
	if err != nil {
		return 0, false
	}
	note := strings.Index("C D EF G A B", strings.ToUpper(match[1]))
	if match[2] == "#" || match[4] == "#" {
		note++
	} else if strings.EqualFold(match[2], "b") || strings.EqualFold(match[4], "b") {
		note--
	}
	key := (octave+1)*12 + note
	return key, key >= 0 && key <= 127
}

func iconButton(c *ui.Context, icon *ui.SVG, label string, fn func()) {
	if ui.Button(c, "").Size(36, 32).Label(label).Tooltip(label).Children(func() {
		ui.Icon(c, icon).Size(18, 18)
	}).Clicked() {
		fn()
	}
}

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
	theme.TextMuted = palette.Muted
	theme.Border = palette.Grid
	theme.SurfaceHover = ui.Hex("#383737")
	theme.SurfacePressed = ui.Hex("#483638")
	theme.AccentText = palette.Surface
	theme.AccentHover = palette.Accent
	theme.AccentPressed = palette.Selected
	theme.Focus = palette.Accent
	theme.Selection = palette.Grid
	theme.Scrollbar = palette.Muted
	theme.Radius = 8
	c.SetTheme(theme)
	e.practiceFrame()
	tick, playing := e.transportState()
	if playing || e.recording {
		c.After(time.Second / 60)
	}
	root := ui.Column(c).Fill().Gap(6).Padding(8)
	root.Children(func() {
		ui.Row(c).Gap(6).Wrap().Children(func() {
			ui.Text(c, "DoMiSo").FontSize(18).Bold()
			button(c, "打开", e.open)
			button(c, "保存", func() { e.save(false) })
			button(c, "另存为", func() { e.save(true) })
			button(c, "导出 MIDI", func() { e.export(true) })
			button(c, "导出原文", func() { e.export(false) })
			ui.Divider(c)
			button(c, "撤销", func() { e.history(false) })
			button(c, "重做", func() { e.history(true) })
		})
		ui.Row(c).Gap(8).AlignItems(ui.Start).Children(func() {
			ui.Column(c).Grow(1).Gap(6).Children(func() {
				ui.Row(c).Children(func() {
					previous := e.mode
					if choice(c, &e.mode, "编辑", "播放", "练习") {
						e.changeMode(previous)
					}
				})
				e.performanceBar(c)
			})
			ui.Column(c).Width(380).Gap(4).Children(func() { e.songInfo(c) })
		})
		ui.Row(c).Gap(8).Wrap().Children(func() {
			ui.Text(c, "吸附")
			choice(c, &e.grid, "自由", "1/16", "1/8", "1/7", "1/6", "1/5", "1/4", "1/3", "1/2")
			ui.Divider(c)
			ui.Text(c, "标签")
			choice(c, &e.label, "隐藏", "音名", "简谱", "按键")
			ui.Divider(c)
			ui.Text(c, "音频缓冲 ms")
			if ui.Select(c, &e.buffer, []string{"20", "40", "80"}).Width(65).Changed() {
				e.saveSettings()
				e.message = "缓冲设置重启后生效"
			}
		})
		if e.mode == "练习" {
			ui.Column(c).Width(380).Disabled(e.busy).Children(func() { e.sidebarInfo(c) })
		}
		ui.Row(c).Grow(1).MinHeight(200).Gap(8).AlignItems(ui.Stretch).Children(func() {
			if e.mode != "练习" {
				ui.Column(c).Width(380).Gap(8).Disabled(e.busy).Children(func() { e.sidebar(c) })
			}
			ui.Column(c).Grow(1).Gap(5).Children(func() {
				if e.mode == "练习" {
					e.practiceTimeline(c, tick, playing)
					e.keyboard(c, tick)
					e.practiceControls(c)
				} else {
					e.trackBar(c)
					roll := ui.Box(c).Grow(1).MinHeight(140).Clip().Focusable().Label("钢琴卷帘；顶部时间轴右侧可横向或纵向缩放，双击创建音符，拖动移动，右边缘改变时长，空白拖动框选").HandleInput(e.rollInput).Draw(func(p *ui.Painter, r ui.Rect) { e.rollWidth, e.rollHeight = r.W, r.H; e.drawRoll(p, r, tick, playing) })
					e.buildFocus = e.buildFocus || roll.Focused()
					e.keyboard(c, tick)
				}
			})
		})
		e.transportBar(c, tick, playing)
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
	if root.Shortcut(0, ui.KeySpace) && !e.busy {
		e.togglePlay()
	}
	if e.inputFocused && !e.buildFocus && len(e.keyDown) > 0 {
		e.releaseKeys()
	}
	e.inputFocused = e.buildFocus
}
func (e *Editor) sidebarInfo(c *ui.Context) {
	names := []string{"选择内置曲谱…"}
	for _, s := range e.sheets {
		names = append(names, s.Name)
	}
	ui.Row(c).Gap(8).Children(func() {
		ui.Text(c, "曲谱").Bold()
		if ui.Select(c, &e.sheet, names).Grow(1).Changed() {
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
	})
}

func (e *Editor) songInfo(c *ui.Context) {
	ui.Text(c, "曲谱信息").Bold()
	if e.doc.Meta.Title != "" {
		ui.Text(c, e.doc.Meta.Title)
	}
	if e.doc.Meta.Composer != "" {
		ui.Text(c, "记谱 / 作曲："+e.doc.Meta.Composer)
	}
	if e.doc.Meta.Arranger != "" {
		ui.Text(c, "编曲："+e.doc.Meta.Arranger)
	}
	if e.doc.Meta.Description != "" {
		ui.Scroll(c).Height(48).Children(func() {
			ui.Text(c, e.doc.Meta.Description).FontSize(12).TextColor(palette.Muted)
		})
	}
	ui.Divider(c)
	ui.Column(c).Gap(4).Disabled(e.busy || e.mode != "编辑").Children(func() {
		ui.Row(c).Gap(8).Wrap().Children(func() {
			ui.Row(c).Gap(4).Children(func() {
				ui.Text(c, "BPM")
				if ui.NumberInput(c, &e.tempo, 20, 400, 1).Width(100).Changed() {
					e.checkpoint()
					e.doc.Song.SetTempo(e.tempo)
					e.changed(true)
				}
			})
			ui.Row(c).Gap(4).Children(func() {
				ui.Text(c, "拍号")
				if ui.NumberInput(c, &e.meterNum, 1, 32, 1).Width(80).Label("拍号分子").Changed() {
					e.setMeter()
				}
				if ui.Select(c, &e.meterDen, []string{"2", "4", "8", "16"}).Width(50).Changed() {
					e.setMeter()
				}
			})
		})
		ui.Row(c).Gap(4).Wrap().Children(func() {
			ui.Text(c, "主音")
			if e.tonicInput == "" && e.tonicInputKey == 0 || e.tonicInputKey != e.tonic {
				e.tonicInput = tonicName(int(e.tonic))
				e.tonicInputKey = e.tonic
			}
			if ui.TextInput(c, &e.tonicInput).Width(90).Label("简谱主音，音名或 MIDI 数值").Placeholder("C4").Changed() {
				if key, ok := tonicKey(e.tonicInput); ok {
					e.tonic = float64(key)
					e.tonicInputKey = e.tonic
				}
			}
		})
	})
}

func (e *Editor) sidebarTracks(c *ui.Context) {
	ui.Text(c, "轨道").Bold()
	ui.Scroll(c).Height(120).MinHeight(120).Shrink(0).Children(func() {
		for i := range e.doc.Song.Tracks {
			if len(e.doc.Song.Tracks[i].Notes) == 0 && i != e.track {
				continue
			}
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
	ui.Row(c).Gap(4).Wrap().Children(func() {
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
		if ui.Button(c, "↑").Width(27).Label("上移轨道").Clicked() {
			e.moveTrack(-1)
		}
		if ui.Button(c, "↓").Width(27).Label("下移轨道").Clicked() {
			e.moveTrack(1)
		}
	})
}

func (e *Editor) sidebar(c *ui.Context) {
	e.sidebarInfo(c)
	ui.Divider(c)
	ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Gap(8).Children(func() {
			ui.Column(c).Gap(6).Children(func() { e.sidebarTracks(c) })
			ui.Divider(c)
			ui.Text(c, "DoMiSo 文本").Bold()
			ui.Column(c).Grow(1).Gap(6).Disabled(e.mode != "编辑").Children(func() {
				if ui.TextArea(c, &e.doc.Notation).Height(104).MinHeight(100).MaxHeightPercent(50).Font("monospace").Label("DoMiSo 乐谱源文").Changed() {
					e.changed(false)
					e.doc.OutOfSync = e.doc.Notation != ""
				}
				if ui.Button(c, "从文本重新生成 MIDI").Width(166).Clicked() {
					e.regenerate()
				}
			})
		})
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
	ui.Row(c).Gap(5).Wrap().Disabled(e.busy).Children(func() {
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
	})
	ui.Divider(c)
	ui.Row(c).Gap(8).Wrap().Disabled(e.busy).Children(func() {
		button(c, "选择音源…", e.setInstrument)
		if t.Settings.Instrument.Kind == "soundfont" {
			sf, names := e.audio.soundFontPresets(e.track)
			if sf != nil {
				selected := names[0]
				if p := t.Settings.Instrument.Preset; p != nil {
					for i, choice := range sf.Presets {
						if int(choice.BankNumber) == p.Bank && int(choice.PatchNumber) == p.Program {
							selected = names[i+1]
							break
						}
					}
				}
				if ui.Select(c, &selected, names).Width(320).Label("SoundFont 音色，显示实际 Bank 和 Program，数值从 0 开始").Changed() {
					if selected == names[0] {
						e.setSoundFontPreset(nil)
					} else {
						for i, name := range names[1:] {
							if selected == name {
								p := sf.Presets[i]
								e.setSoundFontPreset(&SoundFontPreset{Bank: int(p.BankNumber), Program: int(p.PatchNumber)})
								break
							}
						}
					}
				}
			} else {
				ui.Text(c, "音源不可用，内置音回退试听").FontSize(12).TextColor(palette.Muted)
			}
		}
	})
	if t.Settings.Instrument.Kind == "sample" || t.Settings.Instrument.Kind == "pack" {
		ui.Row(c).Gap(8).Wrap().Disabled(e.busy).Children(func() {
			ui.Text(c, "采样原始音高")
			if ui.NumberInput(c, &e.sampleRoot, 0, 127, 1).Width(140).Changed() {
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
	}
	ui.Row(c).Gap(6).Wrap().Children(func() {
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
			if ui.NumberInput(c, &vel, 1, 127, 1).Width(140).Changed() {
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
		return "内置回退音（未加载音源）"
	}
	if in.Kind == "soundfont" && in.Preset != nil {
		return fmt.Sprintf("%s · Bank %d / Program %d", in.File, in.Preset.Bank, in.Preset.Program)
	}
	return in.File
}
func (e *Editor) performanceBar(c *ui.Context) {
	ui.Row(c).Gap(6).Wrap().Disabled(e.busy).Children(func() {
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
			if choice(c, &e.state, states...) {
				e.releaseKeys()
				e.groups = nil
			}
		}
		button(c, "导入映射", e.importProfile)
		button(c, "导出映射", e.exportProfile)
		if runtime.GOOS == "windows" {
			button(c, "准备自动演奏", e.prepareAuto)
			button(c, "紧急停止", func() { performance.stop() })
		}
	})
}

const (
	rollKeysWidth         float32 = 52
	rollHeaderHeight      float32 = 26
	rollZoomControlsWidth float32 = 120
)

func (e *Editor) drawRoll(p *ui.Painter, r ui.Rect, tick float64, playing bool) {
	e.ensureRollIndex()
	e.ensureLabelCache()
	e.scrollY = max(0, min(max(0, float32(pianoHigh-pianoLow+1)*e.rowHeight-r.H+rollHeaderHeight), e.scrollY))
	e.scrollX = max(-float32(e.doc.Song.BarTicks()*2)*e.zoom, e.scrollX)
	if playing && e.drag == "" {
		head := float32(tick)*e.zoom - e.scrollX
		if e.follow || head < 0 || head >= r.W-rollKeysWidth {
			e.scrollX = float32(tick)*e.zoom - (r.W-rollKeysWidth)/2
		}
	}
	p.Fill(r, palette.Background, 0)
	p.Clip(r, 0, func() {
		p.Fill(ui.Rect{X: r.X, Y: r.Y, W: r.W, H: rollHeaderHeight}, palette.Surface, 0)
		p.Clip(ui.Rect{X: r.X + rollKeysWidth, Y: r.Y + rollHeaderHeight, W: max(0, r.W-rollKeysWidth), H: max(0, r.H-rollHeaderHeight)}, 0, func() {
			// Draw from top to bottom: pixel-snapped fills must not cover the previous row's grid line.
			for key := pianoHigh; key >= pianoLow; key-- {
				y := r.Y + rollHeaderHeight + float32(pianoHigh-key)*e.rowHeight - e.scrollY
				if y+e.rowHeight < r.Y+rollHeaderHeight || y > r.Y+r.H {
					continue
				}
				shade := palette.Background
				if isBlack(key) {
					shade = palette.Surface
				}
				p.Fill(ui.Rect{X: r.X + rollKeysWidth, Y: y, W: r.W - rollKeysWidth, H: e.rowHeight}, shade, 0)
				p.Line(r.X+rollKeysWidth, y, r.X+r.W, y, 1, palette.Grid)
			}
			// Grid density follows zoom; drawing is bounded by the visible viewport.
			step := e.doc.Song.PPQ
			for float32(step)*e.zoom < 35 {
				step *= 2
			}
			left := int64(math.Floor(float64(e.scrollX/e.zoom)/float64(step))) * step
			for t := left; float32(t)*e.zoom-e.scrollX < r.W; t += step {
				x := r.X + rollKeysWidth + float32(t)*e.zoom - e.scrollX
				if x < r.X+rollKeysWidth {
					continue
				}
				p.Line(x, r.Y+rollHeaderHeight, x, r.Y+r.H, 1, palette.Grid)
			}
			barsInRange(e.meters, int64(e.scrollX/e.zoom), int64((e.scrollX+r.W)/e.zoom), func(tick, bar int64) {
				x := r.X + rollKeysWidth + float32(tick)*e.zoom - e.scrollX
				if x >= r.X+rollKeysWidth {
					p.Line(x, r.Y+rollHeaderHeight, x, r.Y+r.H, 1, palette.Muted)
				}
			})
			drawNote := func(n Note, ti int, color ui.Color) {
				if n.Key < pianoLow || n.Key > pianoHigh {
					return
				}
				nr := e.noteRect(n)
				nr.X += r.X
				nr.Y += r.Y
				if nr.X+nr.W < r.X+rollKeysWidth || nr.X > r.X+r.W || nr.Y+nr.H < r.Y+rollHeaderHeight || nr.Y > r.Y+r.H {
					return
				}
				if ti == e.track && e.selected[n.ID] {
					color = palette.Selected
				}
				p.Fill(nr, color, 3)
				if ti == e.track && nr.W > 18 && e.label != "隐藏" {
					text := e.labelCache[n.Key]
					p.Clip(nr, 0, func() { p.Text(max(nr.X+4, r.X+rollKeysWidth+2), nr.Y+2, text, 11, palette.Background) })
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
			x := r.X + rollKeysWidth + float32(tick)*e.zoom - e.scrollX
			if x >= r.X+rollKeysWidth {
				p.Line(x, r.Y+rollHeaderHeight, x, r.Y+r.H, 2, palette.Selected)
			}
		})
		p.Clip(ui.Rect{X: r.X, Y: r.Y + rollHeaderHeight, W: rollKeysWidth, H: max(0, r.H-rollHeaderHeight)}, 0, func() {
			p.Fill(ui.Rect{X: r.X, Y: r.Y + rollHeaderHeight, W: rollKeysWidth, H: r.H - rollHeaderHeight}, palette.Surface, 0)
			for key := pianoLow; key <= pianoHigh; key++ {
				if isBlack(key) {
					continue
				}
				y := r.Y + rollHeaderHeight + float32(pianoHigh-key)*e.rowHeight - e.scrollY
				top, bottom := y, y+e.rowHeight
				if key < pianoHigh && isBlack(key+1) {
					top -= e.rowHeight / 2
				}
				if key > pianoLow && isBlack(key-1) {
					bottom += e.rowHeight / 2
				}
				rect := ui.Rect{X: r.X, Y: top, W: rollKeysWidth, H: bottom - top}
				p.Fill(rect, palette.Text, 0)
				p.Stroke(rect, palette.Grid, 0, 1)
				if key%12 == 0 || key == pianoLow {
					p.Text(r.X+3, y+3, fmt.Sprintf("%s%d", []string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}[key%12], key/12-1), 10, palette.Surface)
				}
			}
			for key := pianoLow; key <= pianoHigh; key++ {
				if !isBlack(key) {
					continue
				}
				y := r.Y + rollHeaderHeight + float32(pianoHigh-key)*e.rowHeight - e.scrollY
				p.Fill(ui.Rect{X: r.X + 20, Y: y + 1, W: 32, H: e.rowHeight - 2}, palette.Background, 1)
			}
		})
		controlsVisible := r.W >= rollKeysWidth+rollZoomControlsWidth
		labelWidth := max(0, r.W-rollKeysWidth)
		if controlsVisible {
			labelWidth -= rollZoomControlsWidth
		}
		p.Clip(ui.Rect{X: r.X + rollKeysWidth, Y: r.Y, W: labelWidth, H: rollHeaderHeight}, 0, func() {
			barsInRange(e.meters, int64(e.scrollX/e.zoom), int64((e.scrollX+r.W)/e.zoom), func(tick, bar int64) {
				x := r.X + rollKeysWidth + float32(tick)*e.zoom - e.scrollX
				if x >= r.X+rollKeysWidth {
					p.Text(x+3, r.Y+4, fmt.Sprint(bar), 11, palette.Muted)
				}
			})
		})
		x := r.X + rollKeysWidth + float32(tick)*e.zoom - e.scrollX
		if x >= r.X+rollKeysWidth {
			p.Line(x, r.Y, x, r.Y+rollHeaderHeight, 2, palette.Selected)
		}
		p.Fill(ui.Rect{X: r.X, Y: r.Y, W: rollKeysWidth, H: rollHeaderHeight}, palette.Surface, 0)
		p.Line(r.X, r.Y+rollHeaderHeight, r.X+r.W, r.Y+rollHeaderHeight, 1, palette.Muted)
		p.Line(r.X+rollKeysWidth, r.Y, r.X+rollKeysWidth, r.Y+r.H, 1, palette.Muted)
		if controlsVisible {
			for i, label := range [...]string{"横−", "横+", "纵−", "纵+"} {
				button := rollZoomButton(r.W, i)
				button.X += r.X
				button.Y += r.Y
				p.Fill(button, palette.Background, 2)
				p.Stroke(button, palette.Grid, 2, 1)
				p.Text(button.X+2, button.Y+3, label, 11, palette.Text)
			}
		}
		p.Stroke(r, palette.Grid, 0, 1)
	})
}
func rollZoomButton(width float32, index int) ui.Rect {
	return ui.Rect{X: width - rollZoomControlsWidth + float32(index)*29, Y: 2, W: 27, H: 22}
}

func (e *Editor) changeRollZoom(index int) {
	factor := float32(1.3)
	if index%2 == 0 {
		factor = 1 / factor
	}
	if index < 2 {
		next := max(float32(0.02), min(float32(4), e.zoom*factor))
		center := (e.rollWidth - rollKeysWidth) / 2
		e.scrollX = (e.scrollX+center)*next/e.zoom - center
		e.zoom = next
		return
	}
	next := max(float32(10), min(float32(40), e.rowHeight*factor))
	center := (e.rollHeight - rollHeaderHeight) / 2
	e.scrollY = max(0, min(max(0, float32(pianoHigh-pianoLow+1)*next-e.rollHeight+rollHeaderHeight), (e.scrollY+center)*next/e.rowHeight-center))
	e.rowHeight = next
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
	return ui.Rect{X: rollKeysWidth + float32(n.Tick)*e.zoom - e.scrollX, Y: rollHeaderHeight + float32(pianoHigh-int(n.Key))*e.rowHeight - e.scrollY + 1, W: max(3, float32(n.Duration)*e.zoom), H: e.rowHeight - 2}
}
func (e *Editor) tickAtX(x float32) float64 { return float64((x - rollKeysWidth + e.scrollX) / e.zoom) }
func (e *Editor) keyAtY(y float32) int {
	return max(pianoLow, min(pianoHigh, pianoHigh-int((y-rollHeaderHeight+e.scrollY)/e.rowHeight)))
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
			e.scrollY = max(0, min(max(0, float32(pianoHigh-pianoLow+1)*e.rowHeight-e.rollHeight+rollHeaderHeight), e.scrollY+ev.DY))
		}
		return true
	case ui.InputPointerDown:
		if ev.Button == 0 && ev.Y >= 2 && ev.Y < rollHeaderHeight-2 && e.rollWidth >= rollKeysWidth+rollZoomControlsWidth {
			for i := range 4 {
				button := rollZoomButton(e.rollWidth, i)
				if ev.X >= button.X && ev.X < button.X+button.W && ev.Y >= button.Y && ev.Y < button.Y+button.H {
					e.changeRollZoom(i)
					return true
				}
			}
		}
		e.dragX, e.dragY, e.dragScrollX, e.dragScrollY = ev.X, ev.Y, e.scrollX, e.scrollY
		e.dragTick = e.tickAtX(ev.X)
		if ev.Y < rollHeaderHeight {
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
		if ev.X < rollKeysWidth {
			return true
		}
		if e.mode == "播放" || ev.Button == 2 {
			e.drag = "pan"
			if e.mode == "播放" {
				e.beginScrub()
			}
			return true
		}
		notes := e.doc.Song.Tracks[e.track].Notes
		hit := -1
		for i := len(notes) - 1; i >= 0; i-- {
			if notes[i].Key < pianoLow || notes[i].Key > pianoHigh {
				continue
			}
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
			e.scrollY = max(0, min(max(0, float32(pianoHigh-pianoLow+1)*e.rowHeight-e.rollHeight+rollHeaderHeight), e.dragScrollY-(ev.Y-e.dragY)))
			if e.mode == "播放" {
				e.seek(e.tickAtX((e.rollWidth + rollKeysWidth) / 2))
			}
		case "box":
			e.boxX, e.boxY = ev.X, ev.Y
			clear(e.selected)
			for id, v := range e.boxBase {
				e.selected[id] = v
			}
			for _, n := range e.doc.Song.Tracks[e.track].Notes {
				if n.Key < pianoLow || n.Key > pianoHigh {
					continue
				}
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
					n.Key = uint8(max(pianoLow, min(pianoHigh, int(orig.Key)+dk)))
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

// The five rows use ANSI key widths; codes match profile binding names.
type keyboardKey struct {
	label, code string
	units       float32
}

var keyboardRows = [5][]keyboardKey{
	{{"`", "Backquote", 1}, {"1", "Digit1", 1}, {"2", "Digit2", 1}, {"3", "Digit3", 1}, {"4", "Digit4", 1}, {"5", "Digit5", 1}, {"6", "Digit6", 1}, {"7", "Digit7", 1}, {"8", "Digit8", 1}, {"9", "Digit9", 1}, {"0", "Digit0", 1}, {"-", "Minus", 1}, {"=", "Equal", 1}, {"Back", "Backspace", 2}},
	{{"Tab", "Tab", 1.5}, {"Q", "KeyQ", 1}, {"W", "KeyW", 1}, {"E", "KeyE", 1}, {"R", "KeyR", 1}, {"T", "KeyT", 1}, {"Y", "KeyY", 1}, {"U", "KeyU", 1}, {"I", "KeyI", 1}, {"O", "KeyO", 1}, {"P", "KeyP", 1}, {"[", "BracketLeft", 1}, {"]", "BracketRight", 1}, {"\\", "Backslash", 1.5}},
	{{"Caps", "CapsLock", 1.75}, {"A", "KeyA", 1}, {"S", "KeyS", 1}, {"D", "KeyD", 1}, {"F", "KeyF", 1}, {"G", "KeyG", 1}, {"H", "KeyH", 1}, {"J", "KeyJ", 1}, {"K", "KeyK", 1}, {"L", "KeyL", 1}, {";", "Semicolon", 1}, {"'", "Quote", 1}, {"Enter", "Enter", 2.25}},
	{{"LSHIFT", "ShiftLeft", 2.25}, {"Z", "KeyZ", 1}, {"X", "KeyX", 1}, {"C", "KeyC", 1}, {"V", "KeyV", 1}, {"B", "KeyB", 1}, {"N", "KeyN", 1}, {"M", "KeyM", 1}, {",", "Comma", 1}, {".", "Period", 1}, {"/", "Slash", 1}, {"RSHIFT", "ShiftRight", 2.75}},
	{{"LCTRL", "ControlLeft", 1.25}, {"LWIN", "MetaLeft", 1.25}, {"LALT", "AltLeft", 1.25}, {"Space", "Space", 6.25}, {"RALT", "AltRight", 1.25}, {"RWIN", "MetaRight", 1.25}, {"Menu", "ContextMenu", 1.25}, {"RCTRL", "ControlRight", 1.25}},
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
	keyboard := ui.Box(c).Focusable().Label("实体键盘；映射按键与音符提示").Clip()
	if e.mode == "练习" {
		keyboard.Grow(1)
	} else {
		keyboard.Height(156)
	}
	keyboard.HandleInput(func(ev ui.InputEvent) bool { return e.keyInput(ev) }).Draw(func(painter *ui.Painter, r ui.Rect) {
		bounds, width, rowHeight := keyboardGeometry(r, e.mode == "练习")
		scale := rowHeight / 30
		now := time.Now()
		for row, keys := range keyboardRows {
			x := bounds.X
			for _, key := range keys {
				rect := ui.Rect{X: x + 2*scale, Y: bounds.Y + 3*scale + float32(row)*rowHeight, W: max(0, key.units*width-4*scale), H: 26 * scale}
				x += key.units * width
				n, _, bound := p.Binding(e.state, key.code)
				color := palette.Surface
				if bound {
					if flash, ok := e.flashes[n]; ok {
						if elapsed := now.Sub(flash); elapsed >= 0 && elapsed < 350*time.Millisecond {
							color = color.Mix(palette.Selected, 1-float32(elapsed)/350e6)
							c.After(time.Second / 60)
						}
					}
				}
				painter.Fill(rect, color, 4*scale)
				outline := palette.Grid
				stroke := float32(1)
				if bound && (e.pressed[n] || active[n]) {
					outline = palette.Accent
					stroke = 2
				}
				painter.Stroke(rect, outline, 4*scale, stroke)
				size := float32(11)
				if key.units <= 1.25 && len(key.label) > 3 {
					size = 8
				}
				painter.Text(rect.X+5*scale, rect.Y+2*scale, key.label, size*scale, palette.Text)
				if bound && rect.W >= 27*scale {
					painter.Text(rect.X+5*scale, rect.Y+13*scale, noteLabel(n, "简谱", int(e.tonic), p, e.state), 10*scale, palette.Muted)
				}
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
		notes := make([]Note, 0, len(t.Notes))
		for _, n := range t.Notes {
			if n.Key >= pianoLow && n.Key <= pianoHigh {
				notes = append(notes, n)
			}
		}
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

const (
	pianoLow  = 21
	pianoHigh = 108
)

// choice is an exclusive, single-click native mode switch.
func choice(c *ui.Context, value *string, options ...string) bool {
	index := 0
	for i, option := range options {
		if option == *value {
			index = i
		}
	}
	if ui.Segmented(c, &index, options...).Changed() {
		*value = options[index]
		return true
	}
	return false
}
