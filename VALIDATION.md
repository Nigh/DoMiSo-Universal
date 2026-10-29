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

## 2026-10-28 layout, practice and SoundFont update

Windows amd64, Go 1.27.3: `go test ./...`, `go vet ./...` and `go build -trimpath -o bin/ ./...` passed. A temporary native UI smoke scenario rendered edit, playback and practice modes at 1380×900 and 1120×760. Inspected the small editing and practice previews, including the real SF2 preset selector and enlarged full-width practice keyboard. Smoke files and previews remain untracked; the temporary source was removed.

Local GeneralUser-GS.sf2 exposes 288 choices including MIDI-follow mode. The smoke scenario selected Bank 0 / Program 1 (Bright Grand Piano) and rendered PCM. Regression checks compare preset PCM with MeltySynth, cover channel 10, seek, project reload, missing-resource fallback, timeline boundaries, follow/page, non-editing practice input, mode transition key release, bottom transport navigation and read-only playback settings.

The native executable completed `--benchmark`; startup reported `register Ctrl+Alt+F8: Hot key is already registered.` Global hotkeys therefore were not validated by this run. Physical sound, real game input and Linux were not revalidated.

## 2026-10-28 UI/UX refinement

Windows amd64: `go test ./...`, `go vet ./...` and `go build -trimpath -o bin/domiso-ux.exe .` passed. A temporary smoke scenario rendered editing, playback and practice at 1380×900 and 1120×760 with the local GeneralUser SF2 loaded. The editing viewport measures 1068×300 DIP and 808×160 DIP respectively. Inspected small editing/practice and large playback previews; notes at C4 remain visible in the small editor. Also inspected the small editor with complete metadata and selected notes; bounded metadata scrolling prevents keyboard/help overlap.

Launched the actual Windows executable and inspected its UI Automation tree (91 descendants). Selected playback, practice and editing through native selection patterns; each reported selected state. Invoked play and pause through the native button pattern and observed the pause button after playback began. `PrintWindow` succeeded and the captured GPU window was inspected. Temporary smoke source/scripts are removed; ignored previews and the executable remain under `bin/`.

Regression checks cover playback protection of track structure/names, selected-track pitch visibility without overriding later scrolling, metadata/sample controls retaining keyboard/transport visibility, tempo/meter-aware transport positions, practice boundary clipping, keyboard focus/input and the distinction between wait-gated and manually paused playback. Existing text-entry/Space, note-editing, MIDI and asset checks still pass. Theme contrast calculations give normal text at least 13.87:1, muted text 7.10:1 and the focus accent 8.10:1 against the two main dark surfaces. No new dependencies or frontend build were added.

This pass does not validate physical audio latency, real-game injection/monitoring or Linux rendering. The UI/UX skill's generated hero-page pattern did not fit a native editor and was not adopted; a focused UX search verified visible keyboard-focus guidance instead. Existing Xianii colors and native controls remain authoritative.

## 2026-10-28 wait-practice hints

Reproduced the keyboard-outline defect with a rendered-input regression: after accepting A+D, the group index advanced but A retained its outline. The regression now passes: partial chords retain the current outlines; accepting A+D immediately outlines S while A and D remain held; repeated S requires release and a fresh press; finishing clears every target outline.

The timeline keeps merged note-presence spans, but each audible chord onset receives a vertical marker independently of the current practice group. Regression coverage includes starts inside a sustained span, omission of muted-track markers, cache invalidation after muting, and an unchanged timeline image when the judged group advances.

Windows Go 1.27.3: `go test ./...`, `go vet ./...` and `go build -trimpath -o bin/ ./...` passed. A temporary 1120×760 full-view smoke scenario rendered initial, advanced-with-previous-keys-held and completed practice states; all three previews were inspected. The actual Windows executable loaded the smoke MIDI; UI Automation selected practice and enabled wait mode. Foreground synthetic A+D input advanced the displayed pending chord to S, confirmed in the captured GPU window. This is not a physical keyboard-latency or real-game test. Temporary smoke source/scripts are removed; ignored previews and MIDI remain under `bin/`.

## 2026-10-28 practice flash isolation

The rendered regression reproduced a false fill on A at tick 0 before the audio reader consumed the first wait gate. The keyboard shared playback note-on flashes with real-input feedback; the audio reader already gates before dispatching the target MIDI event. Practice now generates flashes only from live input, and mode transitions clear stale flashes. Playback-mode note previews are unchanged.

`TestPracticeKeysFlashOnlyFromInput` checks the first target before/after gating, real-key fill feedback, arrival at the next target before its gate is consumed, wait/free practice, playback previews and switching back from playback. It also verifies silent PCM at the first unpressed target. Existing chord-outline advancement, repeated-chord and manual-pause regressions pass.

Windows Go 1.27.3: full tests, vet and `go build -trimpath -o bin/ ./...` passed. A temporary full-view 1120×760 smoke scenario rendered four states: tick 0 before gating, tick 0 waiting, tick 480 before gating and tick 480 waiting. Inspected previews confirm pending A/S stay unfilled while the actual A press retains its fading feedback. Removed the temporary source; ignored previews remain in `bin/`. Physical sound latency, real-game input and Linux were not revalidated.

## 2026-10-28 expanding notation area

Removed the fixed 104-DIP height and 50% height cap from the notation editor. Its column and text area now grow through the remaining sidebar height; the track list and text area own their scrolling, while the regeneration button stays at the bottom.

The layout regression covers edit/playback modes at 1120×760 and 1120×900: increasing window height by 140 DIP increases the text area height and moves the regeneration button by the same amount, without overlap with the bottom transport. Text-entry/Space and metadata/sample layout checks pass. Software previews at 1380×900 and 1120×760 were rendered and visually inspected using `DOMISO_RENDER_PREVIEW=1 go test -run TestNativeView -count=1`.

## 2026-10-29 instrument and transport fixes

Windows amd64, Go 1.27.3: `go test ./...`, `go vet ./...` and `go build -trimpath -o bin/domiso-universal.exe .` passed. Regression coverage includes compact SoundFont selection, 100% defaults and explicit saved volume preservation, paused navigation with follow enabled/disabled, and keyboard-preview release after the final note.

A temporary full-view smoke scenario loaded GeneralUser-GS.sf2 and rendered 1120×760 and 1380×900 views. Both previews were inspected: the preset selector stays compact and the default volume displays 100%. The temporary source was removed; ignored previews remain under `bin/`. The native executable completed `--benchmark`; startup reported `register Ctrl+Alt+F8: Hot key is already registered.` Physical audio latency, real-game injection and Linux were not revalidated.

## 2026-10-29 roll wheel zoom and scroll limits

Windows Go 1.27.3: full tests, vet and native build passed. Rendered interaction regressions verify mouse-anchored Ctrl+wheel horizontal zoom, viewport-centered Ctrl+Shift+wheel vertical zoom, axis independence, zoom bounds, and horizontal scroll bounds with sustained notes and meter changes. Non-note events do not extend the right boundary; empty rolls stop at zero.

A temporary 1120×760 smoke scenario applied both wheel zoom shortcuts and rendered the native view. The preview was inspected; shortcut hints fit without overlapping transport controls. The native executable completed `--benchmark`. Temporary source was removed; the ignored preview remains in `bin/roll-zoom-smoke.png`. Physical mouse-wheel hardware and Linux were not revalidated.

## 2026-10-29 grid stability, compact labels and track selection

The roll's old 1-DIP lines rounded their two edges independently. At fractional device scales, subpixel scrolling alternated the physical width between one and two pixels. Grid strokes now align their origin and keep a fixed physical width. Rendered regressions cover 20 fractional scroll phases at 100%, 125%, 150% and 200%.

At the minimum 10-DIP pitch row, fixed font sizes and offsets clipped note labels and misplaced piano labels. Labels now use font metrics for fitting and centering; piano labels render after key shapes. Regressions verify piano labels and note-name/number labels remain inside their pitch rows with visible glyphs.

Track selection called global audio silence through live-key cleanup. Cleanup now releases only owned live input on its original track and ignores unmatched key-up events. PCM comparisons cover uninterrupted built-in and sampled playback, while state checks preserve clocks, event cursors, sustain, loop and wait state.

Windows Go 1.27.3: `go test ./...`, `go vet ./...`, native build and the actual executable's `--benchmark` passed. A temporary 1120×760 full-view scenario rendered minimum-height labels and fractional scroll positions at 100%, 125% and 200%; inspected the 100% and 125% images. Temporary source was removed; ignored images remain under `bin/render-fix-*.png`. Physical audio, monitor-refresh behavior and Linux were not revalidated.
