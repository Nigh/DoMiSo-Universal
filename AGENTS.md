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
- Input keys use physical ANSI positions. Text controls own their keyboard input; Space toggles playback from other focused controls. Focus loss releases live notes and finishes gestures.
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

Go 1.27.3 is required by mygo v0.7.1. Dependencies are pinned in go.mod. No Node, CGo or Wails CLI is needed.

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


## Piano-roll presentation

- The roll displays A0–C8 (MIDI 21–108) only. Keep out-of-range MIDI in the document; exclude it from the roll index and hit testing.
- Draw white piano keys first and shorter, right-aligned black keys over them. Clamp scrolling and pointer editing to the 88-key range.
- Imported tracks without notes remain in Song for MIDI event and JSON v3 track-order preservation, but are hidden from the track list. A selected newly created empty editing track stays accessible.
- Use native segmented switches for edit, playback and practice modes, snap, note-label and mapping-state choices. Separate adjacent control groups with dividers. Keep file actions beside the title and key mapping controls above the editor.
- Native palette and control colors follow the dark Xianii tokens from https://github.com/Nigh/xianii-theme, converted from OKLCH to sRGB.

- The roll has a bordered note viewport and a separate top ruler. Horizontal and vertical zoom controls live inside the roll.
- Transport lives at the bottom: start, previous bar, play/pause, next bar, end, and follow. Recording-loop controls are not shown. Without follow, playback pages when the playhead leaves the viewport. Dragging does not disable follow.
- The PC keyboard draws the full physical range from ` to right Ctrl, with fading press color and a held-key outline outside wait practice. In wait practice, outlines identify the next unjudged chord instead; held keys keep fill feedback and cannot retain obsolete target outlines.
- Practice key-fill flashes come only from live note input, never from transport note-on previews. This applies to wait and free practice, including the frame before an audio wait gate is consumed. Clear transient flashes on mode changes; preserve playback previews outside practice.
- Piano-roll white-key rows use the darker background and black-key rows the lighter surface; horizontal rules remain visible at fractional vertical scroll positions.
- The upper-right song information area shows existing metadata read-only; extracting metadata from notation is not implemented. BPM, meter and tonic are editable only in edit mode. The tonic field shows C4 without `1=`, but still accepts DoMiSo `1=C4`.
- Keep the sheet selector at the left. Tracks and DoMiSo text share one vertical column in edit/playback modes. The track list shows four rows before scrolling. The text area grows to consume the remaining sidebar height and scrolls internally; keep the regeneration button below it at the sidebar bottom rather than wrapping the whole sidebar in a scroll container.
- Practice replaces the roll with a full-width timeline showing bar numbers and flattened note presence across all tracks. Mark each audible chord onset with a vertical line, including onsets inside sustained/merged spans; do not draw a moving target marker or target label on the timeline. Hide track/text panels, center and enlarge the physical keyboard, and put practice controls below it. Practice input never records or edits notes.
- SoundFont choices come from the loaded SF2 presets and show bank/program/name. A nullable per-track preset preserves original MIDI selection when unset; an explicit preset overrides synthesis without replacing MIDI events. Persist it in JSON. Built-in sound is fallback only, not a source-selection button.
- Keep the header compact: read-only metadata stays at the upper right; BPM, meter, tonic and restart-only audio buffering share a separate settings row. Snap and note labels belong above the roll and are hidden in practice.
- The left editing column is 280 DIP wide. Track structure and track names are read-only during playback; undo/redo and boundary actions expose their disabled state. Opening a document or selecting a track reveals its first in-range note without resetting subsequent manual scrolling.
- Transport uses vector icons, an explicit play/pause label, tempo-aware elapsed/total time and meter-aware bar numbers. Practice shows pending/next chord keys, wait state and empty-state guidance. Wait-practice outlines move to the next chord as soon as the current chord is accepted, even while previous keys remain held; repeated chords still require a fresh press, and completion clears target outlines. Custom editor surfaces draw inset keyboard-focus outlines.
- Bound metadata height with one scroll container so full metadata and note-selection controls cannot compress the physical keyboard. Keep sample-root/gated controls in the instrument row; only single samples expose an editable root pitch, read from the selected track.
- New tracks and imports default to 100% volume; omitted JSON volume also defaults to 100%, while explicit saved values, including zero, remain unchanged. Keep the SoundFont preset selector compact at 280 DIP.
- Transport navigation reveals the selected position once, even while paused or with follow disabled, without changing the follow setting. Playback keyboard outlines clear when transport ends; physical held-key feedback remains independent.
- Roll wheel shortcuts: Ctrl+wheel zooms time around the mouse cursor; Ctrl+Shift+wheel zooms pitch around the note viewport center. Shift+wheel scrolls time; plain wheel scrolls pitch. Existing zoom buttons retain viewport-centered zoom.
- Manual roll scrolling stops at the start of the last occupied bar across in-range notes on all tracks, including sustained notes and meter changes; MIDI events alone do not extend this limit. Empty rolls stop at tick zero on the right; retain two pre-roll bars on the left.
- Align roll grid strokes to physical pixels using the painter scale and keep each stroke's device-pixel width constant while scrolling. Center note labels using measured text height, reduce font size only when rows require it, and draw piano pitch labels after all key shapes.
- Track selection changes display/edit selection only. Release live input on its original track and ignore unmatched key-up events; never silence transport voices when selecting tracks or releasing live input.
