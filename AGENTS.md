# AGENTS.md

Project instructions for DoMiSo Universal.

**After changing this project, update this file so it matches the actual project state.**

## Overview

DoMiSo Universal is a Wails 3 desktop MIDI editor. DoMiSo text is an optional source format: when a sheet has no MIDI, the Go parser creates it once. The in-memory Signal `Song`/MIDI is then the only musical source used by editing, playback, practice, auto-play, and saving.

## Architecture

```text
DoMiSo-genshin/
├── main.go                    Wails entry; embeds frontend/dist
├── app.go                     Sheet parsing/import/export and profile file I/O
├── performance.go             Platform-neutral auto-play scheduler
├── performance_windows.go     SendInput, foreground whitelist, global key hook/hotkeys
├── performance_unsupported.go Non-Windows stubs
├── free_sheets/               Embedded sample sheets
├── frontend/                  Single npm workspace; no iframe/submodule
│   ├── app/                   React 19 DoMiSo + Signal application
│   │   └── src/
│   │       ├── domiso/        DoMiSo editor/project UI
│   │       ├── game-mapping/  Pure key ↔ MIDI pitch mapping
│   │       ├── performance/   Practice judgement, PC-key display, and auto-play planning/UI
│   │       └── ...            Signal editor, stores, MIDI and rendering
│   ├── packages/core/         Signal song/MIDI domain code
│   ├── packages/player/       Signal playback and SoundFont synth
│   ├── packages/dialog-hooks/ Signal dialog hooks
│   ├── bindings/              Generated Wails bindings; never edit/commit
│   └── dist/                  Build output embedded by Go; never commit
└── Taskfile.yml
```

Signal was vendored from fork commit `884ae7791dfe0349c6ca160e1d8827c0a1a6d72d`. Its MIT license is retained at `frontend/SIGNAL-LICENSE`. There is no Signal git submodule, iframe, postMessage bridge, or copied Signal dist.

## Musical Source Rules

- MIDI/Song is the sole runtime truth.
- Load embedded/imported MIDI first. Generate MIDI from DoMiSo only when MIDI is absent.
- `SongStore.generatedFromNotation` records generated MIDI; editing it sets `notationOutOfSync` and the text becomes reference-only.
- MIDI changes include mouse edits, PC-key/MIDI recording, track changes, and tempo changes. Selection, playback, scroll, and zoom are not changes.
- Never convert MIDI back to DoMiSo or silently overwrite edited MIDI from text.
- A generated, unedited text sheet saves without an unnecessary MIDI snapshot.
- Edited MIDI with reference text must be saved as JSON; TXT/DMS export is rejected to prevent data loss.

## Sheet JSON v2

```json
{
  "version": 2,
  "meta": { "title": "Song" },
  "notation": "1=C 4/4 120\n1 2 3 4",
  "midi": "optional base64 standard MIDI",
  "notationOutOfSync": true
}
```

Version 1 remains readable. `midi` and `notation` are each optional, but at least one must exist. `notationOutOfSync: true` requires MIDI.

## Game Mapping and Performance

- `game-mapping` is pure conversion only. It maps `KeyboardEvent.code`/scan code to MIDI pitch and MIDI pitch back to a required key/state; it must not play, wait, schedule, or inject input.
- Profiles contain states, state transitions, scan codes, process whitelist, and calibrated timing. The built-in profile is the Genshin Windsong Lyre 21-key layout.
- `performance` owns practice progression, upcoming notes, the synchronized PC-key display, state-transition planning, tempo-aware key timelines, and native auto-play calls.
- Practice chords require an exact pressed-note set and at least one fresh press after entering the target group; held notes must never advance through later groups.
- Editor practice routes mapped PC input through Signal MIDI monitoring/recording and produces local sound.
- Game practice uses the Windows global hook for display/judgement and does not produce local sound or block the physical key.
- Wait mode pauses for the target chord and resumes only a practice-induced wait; mapped notes never resume a manual pause. Non-wait mode follows the original MIDI timing. The UI shows the current group plus three upcoming groups.
- Auto-play submits one complete scan-code timeline to Go. Go only injects while a profile-whitelisted game is foreground and releases every held key on stop/error/focus loss.
- Windows hotkeys: `Ctrl+Alt+F8` start/stop, `Ctrl+Alt+F9` emergency stop.
- Windows alone supports native game monitoring/auto-play. Other platforms retain editor playback and Web MIDI.

## Go Service Methods

- Sheet: `ParseTextToMIDI`, `ParseSheetStats`, `ImportSheet`, `ExportSheet`, `ListFreeSheets`, `LoadSheet`, `FormatDuration`.
- Profiles: `ImportGameProfile`, `ExportGameProfile`.
- Performance: `PrepareKeySequence`, `StartAutoPlay`, `PauseAutoPlay`, `StopAutoPlay`, `SetGamePracticeMonitoring`, `RegisterPerformanceHotkeys`.
- File reads and profile/sheet imports are limited to 1 MB. MIDI base64 is validated for an `MThd` header.
- After service signature changes run `wails3 generate bindings -clean=true -ts`; never edit generated bindings.

## Frontend Conventions

- React 19, Vite, MobX/Jotai, Emotion, and existing Signal dependencies; do not add a second UI framework.
- UI colors, fonts, radii, and shared controls use the centralized xianii theme tokens; side panels reuse Signal UI primitives instead of styling native controls independently.
- Store xianii palette values in the theme as sRGB hex/rgba equivalents, not `oklch()`: existing Signal components call the `color` package at render time, and that parser rejects OKLCH and leaves the window blank.
- Jotai is declared at the npm workspace root so hoisted peer dependencies resolve one native package instance; do not alias its package directory in Vite.
- Import Wails bindings from `frontend/bindings/domiso-universal` using the correct relative path.
- `ParseTextToMIDI` returns base64 because Wails serializes `[]byte` that way.
- Signal `RootStore` is the shared song/player/MIDI state. Do not duplicate it into React state or add a message bridge.
- PC keyboard note events must enter `MIDIInput` so monitor, recorder, practice, and piano-key highlighting observe one event stream.
- Piano-roll note labels can be hidden or switched between note names, movable-do numbered notation (with octave dots), and active-profile key names in the toolbar. The first DoMiSo tonic initializes display-only `SongStore.labelTonic` on load (DoMiSo C5 = MIDI C4); the toolbar can override it without editing MIDI. Labels use a transparent canvas shared by modern and legacy WebGL rendering.
- The ruler supports pointer-captured, unsnapped continuous seeking while dragging; Ctrl/Alt drag adjusts loop begin/end.
- The roll toolbar uses segmented controls for labels and edit/playback mode. Playback mode disables pencil/selection and note-edit shortcuts; dragging pans vertically and seeks horizontally with a centered playhead, including while paused. Dragging uses absolute CSS-pixel displacement and coalesces updates per animation frame; Player scrubbing suspends audio scheduling and restores controller state once on release or focus loss.
- Both modes show two empty pre-roll bars (-1 and 0), sized from the initial time signature. Negative transport ticks never shift or add MIDI notes; creation, paste, and recording ignore the pre-roll area.
- Roll shaders use high-precision coordinates and initialize every fragment output to retain thin lines at large coordinates and HiDPI scales.
- The PC keyboard uses staggered Q/A/Z rows and a retriggered short flash for each note-on, including MIDI monitoring and scheduled, unmuted playback. Held-key borders follow MIDI note spans at the current transport tick (including paused seeking), independently of note-on flashes. Playback display is separate from practice input, suppresses flashes while scrubbing, cancels pending flashes on stop/seek, and keeps a stable portal host to avoid replaying old flashes. Active profile/state are shared Jotai atoms for keyboard and roll labels.
- The piano-roll lower pane defaults to the synchronized PC keyboard; Velocity and other MIDI controls remain available on the alternate tab.
- Custom mapping profiles are Zod-validated, stored in localStorage, and may be imported/exported as JSON.

## Build and Test

```bash
task dev
task build
go test ./...
cd frontend && npm install
cd frontend && npm run build
cd frontend/app && npx vitest run src/game-mapping/mapper.test.ts src/performance/planner.test.ts
```

The frontend build order is player package, dialog-hooks package, then app. `frontend/app` builds directly into `frontend/dist`; there is no separate Signal build/copy step.
The root frontend `dev` script forwards CLI arguments to the app workspace. On Windows, `task dev` selects a free loopback port and passes it to Wails and Vite through `WAILS_VITE_PORT`; an explicitly set value overrides automatic selection. Vite must stay on `127.0.0.1` because Wails alpha proxies over IPv4. The dev config waits for Vite before launching the app because Wails otherwise gives the frontend only about five seconds and may open blank or exit during cold dependency optimisation. Do not return to a fixed default port: a stale Vite process can make Wails connect to the wrong server.

The ignored `frontend/app/public/soundfonts/GeneralUser-GS.sf2` supplies the optional local GM/GS instruments and drums; missing files disable those sounds without blocking startup. Do not commit SoundFonts, node_modules, package dist folders, Wails bindings, or frontend/dist.

## Important Notes

- `free_sheets/` is committed and embedded with `//go:embed all:free_sheets`.
- Go embed rejects ASCII apostrophes in filenames; use U+02BC and retain `unescapeSheetName()`.
- The parser is `github.com/Nigh/domiso-parser`.
- Go version is 1.25+ and the Wails version is currently v3 alpha.
- Native input is a sensitive boundary: retain whitelist checks, injected-event filtering, validation, emergency stop, and key-up cleanup.
