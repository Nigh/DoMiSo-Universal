# DoMiSo Universal

Native Go desktop editor for DoMiSo and MIDI, built with [mygo](https://github.com/egoist/mygo). No browser or WebView is used.

## Features

- Open DoMiSo TXT/DMS, JSON v1–v3 and standard MIDI; embedded example sheets.
- Multi-track piano roll with note editing, selection, copy/paste, transpose, velocity values and undo/redo.
- Tempo-aware playback, seeking, loops, PC-keyboard monitoring/recording and mapped note labels.
- SoundFont SF2, WAV/OGG/MP3 single samples and JSON multi-sample instruments; built-in synthesis works without extra files.
- Editor practice on Windows/Linux; Windows game monitoring and whitelisted automatic key playback.

## Build

Install Go 1.27.1. Then:

```sh
go run .
go test ./...
go vet ./...
go build -trimpath -o bin/ ./...
```

Task users can run `task dev`, `task build`, `task test` and `task package`. Node, Wails and CGo are not required. On Linux install GTK 3 and Mesa/OpenGL runtime libraries; audio uses PulseAudio/PipeWire, with ALSA fallback. On Ubuntu: `sudo apt install libgtk-3-0 libgl1 libasound2` (newer distributions may name the ALSA package `libasound2t64`).

## Editing

Select a track, double-click empty space to add a note, drag notes to move them or drag the right edge to resize. Drag empty space to select; Shift adds to selection. Right-click a note to delete it. Ctrl+C/X/V copies/cuts/pastes at the playhead; Delete removes selected notes; arrows transpose; Ctrl+Z/Y undo/redo. Space plays/pauses, Escape stops. Click the roll or keyboard area before playing PC keys.

Scroll pans vertically; Shift+scroll pans horizontally; Ctrl+scroll zooms. Drag the ruler to seek; Ctrl/Alt click sets loop start/end. Playback mode disables note editing and lets dragging pan/seek. Two pre-roll bars do not change MIDI note positions.

The default Genshin mapping uses Q–U, A–J and Z–M rows. Practice requires the exact chord and a fresh press. Windows auto-play: prepare the sequence, focus a whitelisted game, then Ctrl+Alt+F8 toggles playback; Ctrl+Alt+F9 stops and releases injected keys.

## Instruments and projects

Choose an SF2 or audio file per track. For one audio file set its original MIDI pitch (C4 = 60). The sampler changes playback speed to transpose; it does not preserve sample duration. Natural decay plays the sample to its end, while “随音符结束” fades it on note-off.

For multiple samples, place a JSON manifest and audio files together; see [sample-pack.example.json](sample-pack.example.json). Each sample has a unique `rootKey` from 0 to 127 and a relative file path. Exact pitch wins; otherwise the nearest sample is transposed, with lower root winning ties. SFZ, velocity layers and sample loop points are not supported.

An optional `soundfonts/GeneralUser-GS.sf2` supplies default GM instruments; SoundFonts are not distributed in this repository. Source-file limit is 256MiB, decoded instrument budget 512MiB. Buffer setting 20/40/80ms applies after restart; actual latency depends on the device and OS.

Save projects as `song.json` with `song.assets/` beside it; move/share both together. The app copies referenced audio assets into that folder. MIDI export preserves musical events but does not carry custom sample settings. Editing MIDI makes DoMiSo text reference-only; regenerate explicitly to replace it. Parameter curves, external MIDI-device connections and audio export are omitted.

Existing v1/v2 sheets remain readable. Export old custom keyboard profiles as JSON before migrating them from the WebView version, then import them in the native app.

## Validation

Native tests cover MIDI conversion, tempo clocks, assets, sample playback, practice rules, editing gestures and headless UI rendering. `DOMISO_RENDER_PREVIEW=1 go test -run TestNativeView` writes `bin/native-preview.png`. Windows and Linux CI runs tests, vet and builds.

Run the app with `--benchmark` and `MYGO_FRAME_STATS=all` to measure five seconds of muted playback with 100,000 notes in a 1920×1080 window. It exits automatically. `go test -run '^$' -bench BenchmarkNative -benchmem` measures the software-rendered view and indexed viewport queries separately.

The former Signal frontend remains recoverable from commit `812e44c` on `feature/unified-signal-editor`. Its MIT license is retained in `LICENSES/Signal-MIT.txt`.
