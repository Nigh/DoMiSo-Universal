# Native migration validation

Validated locally on Windows amd64 and Ubuntu 20.04 amd64 under WSLg, using Go 1.27.1. The old frontend is preserved in Git at `812e44c`; no WebView, Wails or npm dependency remains in the native build.

## Checks

- Windows: `go test ./...`, `go vet ./...`, `go test -race .`, `task build`, `task package`.
- Linux/WSLg: `go test ./...`, `go vet ./...`, native executable build and GUI startup/playback benchmark.
- Linux native software fallback: `MYGO_GPU=0` benchmark starts, renders in memory and exits successfully.
- Actual Windows window creation and accessibility-tree inspection; native controls, roll and track widgets are exposed.
- Headless interactions: selection does not dirty music; copy/delete shortcuts; PC-key note-on/off; moving keyboard focus releases held input; text input does not play notes; HiDPI rendering.
- Data/audio: legacy project loading, MIDI note overlap and raw-controller preservation, tempo/meter changes, safe JSON/assets save and relocation, failed-save preservation, path/symlink escape rejection, WAV and local GeneralUser SF2 synthesis, loops, final note-off cleanup, scrub restoration, practice wait/manual-pause distinction and concurrent engine access.
- Ten minutes of PCM playback are simulated by rendering 600 seconds through the same engine, checking retained heap growth after GC. This is not a ten-minute wall-clock device-latency test.

## Performance observations

AMD Ryzen 5 3600; synthetic 100,000-note song; 1920×1080 requested window. Native benchmark runs five seconds; the first 20 frames are excluded. Frame times include mygo build/layout/paint/present work, not the display-refresh interval.

| Renderer | Observation |
| --- | --- |
| Windows GPU | Median 1.5ms, P95 2.0ms, maximum 4.5ms across 259 sampled frames |
| WSLg OpenGL | Median 5.4ms, P95 6.9ms, maximum 9.4ms across 122 sampled frames |
| Headless software | Approximately 1.70ms/frame in a 50-iteration benchmark |
| Indexed viewport query | Approximately 134ns/query, zero allocations in the same run |

Windows was locked during native measurements. Frame results establish renderer costs in that environment, not a universal FPS or latency guarantee. The app caps scheduled display updates at 60Hz so an occluded window cannot spin unrestricted.

Reproduce with `MYGO_FRAME_STATS=all` and `--benchmark`, or `go test -run '^$' -bench BenchmarkNative -benchmem`. Native binaries, logs, previews and source backups are ignored under `bin/`.

## Remaining manual verification

Physical keyboard-to-speaker latency, live game input/anti-stuck-key behavior, and a normal non-WSLg Linux desktop need manual validation. Windows was locked, so physical GUI clicks and audible checks were replaced by headless interaction tests and window-tree inspection. MIDI/SoundFont advanced SysEx behavior is not guaranteed to match SpessaSynth.

## 2026-10-21 piano-roll and layout update

Go 1.27.1 Linux: full tests, vet and trimpath build passed. Software preview inspected at 1380×900; the 1120×760 view is exercised in the imported-track/range regression check. Native interaction tests cover selection, dragging, keyboard focus and HiDPI. The roll indexes only MIDI 21–108 and hides imported note-free tracks while retaining their MIDI events. This pass did not revalidate physical audio or Windows game injection.
