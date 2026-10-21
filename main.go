package main

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"log"
	"os"
	"runtime"
	"time"
)

func main() {
	editor := newEditor()
	mygo.App.WhenReady(func() {
		editor.win = mygo.NewWindow(mygo.WindowOptions{Title: "DoMiSo Universal", Width: 1380, Height: 900, MinWidth: 1120, MinHeight: 760, Content: ui.View(editor.view)})
		editor.win.Show()
		editor.win.OnBlur(func() { editor.releaseKeys(); editor.finishDrag(); editor.inputFocused = false })
		editor.win.OnClose(func(ev *mygo.CloseEvent) {
			if editor.busy || !editor.confirmUnsaved() {
				ev.PreventDefault()
				return
			}
			editor.stop()
			_ = platformSetGamePracticeMonitoring(false, nil)
			editor.saveSettings()
		})
		go func() {
			for ev := range performanceEvents {
				editor.win.Update(func() { editor.performanceEvent(ev) })
			}
		}()
		if runtime.GOOS == "windows" {
			if err := (&AppService{}).RegisterPerformanceHotkeys(); err != nil {
				log.Print(err)
			}
		}
		if len(os.Args) > 1 && os.Args[1] == "--benchmark" {
			editor.benchmark = true
			editor.doc.Song = emptySong()
			editor.doc.Song.Tracks[0].Settings.Volume = 0
			for i := range 100000 {
				editor.doc.Song.Tracks[0].Notes = append(editor.doc.Song.Tracks[0].Notes, Note{ID: i + 1, Tick: int64(i * 60), Duration: 120, Key: uint8(48 + i%36), Velocity: 100})
			}
			editor.doc.Song.NextID = 100000
			editor.win.SetSize(1920, 1080)
			editor.configureAudio()
			time.AfterFunc(5*time.Second, func() { editor.win.Update(func() { editor.win.Destroy() }) })
			return
		}
		if len(os.Args) > 1 {
			editor.load(os.Args[1])
		} else {
			editor.configureAudio()
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
