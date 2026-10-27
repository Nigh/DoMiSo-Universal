package main

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/egoist/mygo/ui"
)

func (e *Editor) changeMode(previous string) {
	next := e.mode
	e.mode = previous
	e.releaseKeys()
	e.finishDrag()
	e.finishRecording()
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
	ui.Row(c).Gap(8).Disabled(e.busy).Children(func() {
		ui.Spacer(c)
		if ui.Button(c, "|◀").Label("移动到开头").Tooltip("移动到开头").Clicked() {
			e.seek(0)
		}
		if ui.Button(c, "◀").Label("往前一小节").Tooltip("往前一小节").Clicked() {
			e.seekBar(tick, -1)
		}
		if playing {
			iconButton(c, pauseIcon, "暂停播放", e.togglePlay)
		} else {
			iconButton(c, playIcon, "播放", e.togglePlay)
		}
		if ui.Button(c, "▶").Label("往后一小节").Tooltip("往后一小节").Clicked() {
			e.seekBar(tick, 1)
		}
		if ui.Button(c, "▶|").Label("移动到结尾").Tooltip("移动到结尾").Clicked() {
			e.seek(float64(e.doc.Song.End()))
		}
		ui.Checkbox(c, &e.follow, "跟随")
		ui.Textf(c, "%.0f", tick).Width(65).Label("播放位置 tick")
		ui.Spacer(c)
	})
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
	ui.Row(c).Gap(8).Wrap().Disabled(e.busy).Children(func() {
		ui.Spacer(c)
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
		for i := e.group; i < min(e.group+4, len(e.groups)); i++ {
			labels := []string{}
			for _, n := range e.groups[i].Notes {
				v := e.profile().KeyFor(e.state, n)
				if v == "" {
					v = fmt.Sprint(n)
				}
				labels = append(labels, strings.TrimPrefix(v, "Key"))
			}
			ui.Text(c, strings.Join(labels, "+")).TextColor(palette.Accent)
		}
		ui.Spacer(c)
	})
}
