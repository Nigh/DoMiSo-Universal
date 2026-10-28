package main

import (
	"fmt"
	"math"
	"runtime"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"
)

var (
	startIcon       = ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24"><path d="M4 4h3v16H4zm16 0v16L8 12z"/></svg>`))
	previousBarIcon = ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24"><path d="M12 4v16L1 12zm11 0v16l-11-8z"/></svg>`))
	nextBarIcon     = ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24"><path d="M1 4v16l11-8zm11 0v16l11-8z"/></svg>`))
	endIcon         = ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24"><path d="M17 4h3v16h-3zM4 4v16l12-8z"/></svg>`))
)

func (e *Editor) changeMode(previous string) {
	next := e.mode
	e.mode = previous
	e.releaseKeys()
	e.finishDrag()
	e.finishRecording()
	clear(e.flashes)
	e.mode = next
	e.practice = next == "练习"
	e.groups = nil
	clear(e.fresh)
	e.practiceWait = false
	if !e.practice && e.gamePractice {
		if err := (&AppService{}).SetGamePracticeMonitoring(false, nil); err != nil {
			e.message = err.Error()
		}
		e.gamePractice = false
	}
	if !e.practice && e.audio.Waiting() {
		e.audio.WaitAt(-1)
		e.audio.Play(true)
	}
}

func (e *Editor) transportBar(c *ui.Context, tick float64, playing bool) {
	seconds, total, bar := e.transportPosition(tick)
	ui.Row(c).Gap(12).Wrap().Justify(ui.Center).Padding(6, 8).Background(palette.Surface).Disabled(e.busy).Children(func() {
		ui.Row(c).Gap(4).Children(func() {
			iconButton(c, startIcon, "移动到开头", func() { e.seek(0) })
			iconButton(c, previousBarIcon, "往前一小节", func() { e.seekBar(tick, -1) })
			icon, text, label := playIcon, "播放", "播放"
			if playing {
				icon, text, label = pauseIcon, "暂停", "暂停播放"
			}
			if ui.Button(c, "").Size(96, 36).Gap(6).Label(label).Tooltip("Space：播放 / 暂停；文本输入时不触发").Children(func() {
				ui.Icon(c, icon).Size(18, 18)
				ui.Text(c, text).Bold()
			}).Clicked() {
				e.togglePlay()
			}
			iconButton(c, nextBarIcon, "往后一小节", func() { e.seekBar(tick, 1) })
			iconButton(c, endIcon, "移动到结尾", func() { e.seek(float64(e.doc.Song.End())) })
		})
		state := "已暂停"
		if playing {
			state = "正在播放"
		} else if e.practice && e.audio.Waiting() {
			state = "等待按键"
		}
		ui.Text(c, state).MinWidth(64).TextColor(palette.Selected)
		ui.Text(c, transportClock(seconds)+" / "+transportClock(total)).Font("monospace").Label("播放时间").Tooltip("当前位置 / 总时长；按曲谱速度变化换算，负值为预备时间")
		ui.Textf(c, "第 %d 小节", bar).Label("播放小节").Tooltip(fmt.Sprintf("按曲谱拍号变化编号；当前位置 %.0f tick", tick))
		ui.Checkbox(c, &e.follow, "跟随").Label("跟随").Tooltip("开启：播放位置保持居中；关闭：超出视口时翻页")
		ui.Text(c, "Space 播放 / 暂停").FontSize(12).TextColor(palette.Muted)
	})
}

// Reuse the audio tempo map and roll meter cache instead of rebuilding them per frame.
func (e *Editor) transportPosition(tick float64) (seconds, total float64, bar int64) {
	e.audio.mu.Lock()
	seconds = secondsAt(e.audio.tempos, e.audio.ppq, tick)
	total = float64(e.audio.end) / sampleRate
	e.audio.mu.Unlock()
	e.ensureRollIndex()
	i := max(0, sort.Search(len(e.meters), func(i int) bool { return float64(e.meters[i].Tick) > tick })-1)
	meter := e.meters[i]
	bar = meter.Bar + int64(math.Floor((tick-float64(meter.Tick))/float64(meter.Size)))
	return
}

func transportClock(seconds float64) string {
	sign := ""
	if seconds < 0 {
		sign = "-"
	}
	tenths := int64(math.Abs(seconds) * 10)
	return fmt.Sprintf("%s%d:%02d.%d", sign, tenths/600, tenths/10%60, tenths%10)
}

func (e *Editor) seekBar(tick float64, direction int64) {
	size := e.doc.Song.PPQ * 4
	for _, meter := range e.doc.Song.Meters() {
		if float64(meter.Tick) > tick || direction < 0 && meter.Tick > 0 && float64(meter.Tick) == tick {
			break
		}
		size = meter.Size
	}
	e.seek(tick + float64(direction*size))
}

func (e *Editor) practiceControls(c *ui.Context) {
	ui.Column(c).Gap(6).Disabled(e.busy).Children(func() {
		ui.Row(c).Gap(10).Wrap().Justify(ui.Center).Children(func() {
			if ui.Checkbox(c, &e.waitMode, "等音练习").Label("等音").Tooltip("播放到目标和弦时暂停，重新按齐全部按键后继续；关闭等音会解除等音等待，手动暂停仍需点击播放").Changed() {
				if !e.waitMode && e.audio.Waiting() {
					e.audio.WaitAt(-1)
					e.audio.Play(true)
				}
			}
			waitStatus := "自由跟弹 · 不等待按键"
			if e.waitMode {
				waitStatus = "到目标暂停 · 按齐和弦继续"
				if e.audio.Waiting() {
					waitStatus = "等待重新按齐当前和弦"
				}
			}
			ui.Text(c, waitStatus).FontSize(12).TextColor(palette.Muted)
			ui.Divider(c)
			if ui.Checkbox(c, &e.gamePractice, "监听游戏按键").Label("游戏监听").Tooltip("仅 Windows；只监听映射配置允许的前台游戏，不向游戏发送按键；关闭后可在此窗口练习").Disabled(runtime.GOOS != "windows").Changed() {
				e.releaseKeys()
				if err := (&AppService{}).SetGamePracticeMonitoring(e.gamePractice, e.profile().ProcessNames); err != nil {
					e.gamePractice = false
					e.message = err.Error()
				}
			}
			gameStatus := "已关闭 · 在此窗口练习"
			if runtime.GOOS != "windows" {
				gameStatus = "仅 Windows 支持"
			} else if e.gamePractice {
				gameStatus = "已开启 · 切换到允许的游戏窗口"
			}
			ui.Text(c, gameStatus).FontSize(12).TextColor(palette.Muted)
		})
		ui.Row(c).Gap(10).Wrap().Justify(ui.Center).Children(func() {
			ui.Text(c, "待按和弦").Bold().Tooltip("等音练习时，键盘外框标出这组按键；按齐后立即切换到下一组")
			if e.group >= len(e.groups) {
				ui.Text(c, "此位置后无练习音符").TextColor(palette.Muted)
				return
			}
			profile := e.profile()
			for i := e.group; i < min(e.group+4, len(e.groups)); i++ {
				if i == e.group+1 {
					ui.Text(c, "接下来").FontSize(12).TextColor(palette.Muted)
				}
				labels := make([]string, 0, len(e.groups[i].Notes))
				for _, n := range e.groups[i].Notes {
					key := profile.KeyFor(e.state, n)
					if key == "" {
						key = tonicName(n) + "（未映射）"
					}
					labels = append(labels, strings.TrimPrefix(key, "Key"))
				}
				chord := ui.Text(c, strings.Join(labels, " + "))
				if i == e.group {
					chord.Bold().TextColor(palette.Accent)
				} else {
					chord.TextColor(palette.Muted)
				}
			}
		})
	})
}
