# AGENTS.md

Project instructions for DoMiSo Universal. Update this file after changing the project.

## Architecture

DoMiSo Universal is a native Go desktop MIDI editor using mygo's GPU UI, without WebView, JavaScript, Wails, or a frontend build.

- `main.go`: desktop lifecycle and platform events.
- `native_editor.go`, `native_view.go`: document commands, native controls, multi-track piano roll, recording, keyboard and practice display.
- `song.go`: sole runtime Song/MIDI model, SMF conversion and tempo map.
- `project.go`: JSON v3 projects, relative audio assets, bounded reads and atomic saves.
- `audio_engine.go`: one sample-clock transport, SoundFont/sample/built-in synthesis and Oto PCM output.
- `mapping.go`: mapping profiles, fresh-chord judgement and tempo-aware game key planning.
- `app.go`: existing DoMiSo parser, embedded sheets and legacy file services.
- `performance*.go`: native Windows injection, foreground whitelist, hooks and emergency hotkeys; unsupported-platform stubs.
- `free_sheets/`: committed embedded sheets; `soundfonts/`: optional ignored local SF2 files.

## Musical source and persistence

- Song/MIDI is the only musical truth used by editing, playback, recording, practice and saving.
- Read embedded/imported MIDI first; generate from notation only if MIDI is absent. Explicit regeneration warns before replacing edits.
- Editing music or reference text marks notation out of sync. Do not convert MIDI back to notation.
- A generated unedited notation sheet saves without a redundant MIDI snapshot. Edited MIDI must use JSON; plain-text export is rejected.
- JSON v3 extends v1/v2 with ordered per-track instrument/volume/mute/solo settings. Track order matches the stored SMF; keep metadata-only tracks too.
- Save `song.json` plus `song.assets/`; copy audio by content hash and use relative paths. Copy resources before atomically replacing JSON; never remove old assets during save.
- Missing instruments use built-in sound for preview while preserving their original document references; failed saves retain the existing project.
- SMF 0/1 with PPQ is supported; reject SMPTE/SMF2 before calling gomidi. Preserve non-note MIDI events even without an editor for them.

## UI and audio

- Use mygo `ui` controls and centralized palette values; piano-roll notes are custom drawing, not individual widgets.
- Edit one selected track; show other tracks as ghosts. Playback mode disables note edits. Dragging uses a single undo entry.
- Two negative pre-roll bars are display/transport only; never create or record negative MIDI ticks.
- Cache a sorted note/prefix-end index per track; rebuild on edits, query visible ranges during rendering. Respect DIP/HiDPI scaling.
- Input keys use physical ANSI positions. Text controls own their keyboard input; focus loss releases live notes and finishes gestures.
- Route PC keys through one Go note-input function for sound, recording, practice and highlighting.
- Audio is Go-MeltySynth plus sampled PCM and a built-in sine fallback, mixed to one 48kHz stereo Oto stream. Output buffering is configurable at 20/40/80ms and applies after restart.
- Display position interpolates the transport clock independently of the rendered-ahead PCM frontier. Request at most 60 display updates per second, including while occluded. Scrub gestures preview the position and restore MIDI state/queued audio once on release.
- The audio read path owns the transport sample clock and consumes a compiled event timeline; never schedule musical events from GUI frames.
- Seek flushes queued PCM and restores controller state. Practice wait gates the audio clock before the target chord; mapped notes cannot resume a manual pause.
- Sample sources support WAV/OGG/MP3. A single sample has a root MIDI key; a v1 JSON pack has `name` and `samples[{file, rootKey}]`. Nearest root wins, ties choose the lower key. Natural decay is default; gated release is optional.
- No parameter curves, external USB/Bluetooth MIDI devices, audio export, SFZ, velocity layers or sample loop-point editing in this version.

## Safety and platform support

- Windows and Linux are supported for native editing/playback. Linux needs GTK 3/OpenGL and PulseAudio/PipeWire or ALSA runtime libraries; no WebKit.
- Windows alone supports game global monitoring and auto-play. Preserve whitelist checks, injected-event filtering, stop/error/focus-loss key release and Ctrl+Alt+F8/F9 hotkeys.
- Windows hook and hotkey message loops must stay on locked OS threads.
- Mapping profiles preserve the existing v1 JSON shape, validate keys/pitches/states/transitions/timing, and live under `os.UserConfigDir()/domiso-universal`.
- Sheet/profile/manifest reads: 1MiB; audio files: 256MiB each; decoded instrument budget: 512MiB. Sample-pack paths must stay in their directory, including through symlinks.
- Never commit SoundFonts, build outputs, local backups or generated test previews.
- Go embed rejects ASCII apostrophes in filenames; retain U+02BC filenames and `unescapeSheetName()`.

## Build and checks

Go 1.27.1 is required. Dependencies are pinned in go.mod. No Node, CGo or Wails CLI is needed.

```sh
go run .
go test ./...
go vet ./...
go build -trimpath -o bin/ ./...
task dev
task build
task package
```

`DOMISO_RENDER_PREVIEW=1 go test -run TestNativeView` writes an ignored software-rendered preview. Windows/Linux CI builds and tests the native app. WSLg smoke tests cover the local Linux environment; physical sound latency and a real game need separate hardware validation.

`MYGO_FRAME_STATS=all` with the app's `--benchmark` option runs five seconds of silent 100,000-note playback in a requested 1920×1080 window, then exits. `VALIDATION.md` records local checks and their limits.

Legacy frontend sources are recoverable from commit `812e44c` on `feature/unified-signal-editor`. Local ignored `.legacy-frontend/` and `bin/legacy-frontend-812e44c.zip` preserve prior build resources. Signal's MIT license is retained in `LICENSES/Signal-MIT.txt`.
