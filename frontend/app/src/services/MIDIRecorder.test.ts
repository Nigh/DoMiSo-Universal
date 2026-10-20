import { expect, it } from "vitest"
import { Player } from "@signal-app/player"
import { MIDIRecorder } from "./MIDIRecorder"
import { SongStore } from "../stores/SongStore"
import { EventSource } from "../player/EventSource"
import { MIDIDeviceStore } from "../stores/MIDIDeviceStore"

it("ignores note recording in pre-roll and records from MIDI tick zero", () => {
  const songStore = new SongStore()
  const player = new Player({ activate() {}, sendEvent() {} }, new EventSource(songStore))
  const recorder = new MIDIRecorder(songStore, player, { midiInputRouting: 'selectedTrack' } as MIDIDeviceStore)
  const track = songStore.song.tracks.find((track) => !track.isConductorTrack)!
  recorder.trackId = track.id
  recorder.isRecording = true
  const before = track.events.length
  player.position = -480
  recorder.onMessage({ data: new Uint8Array([0x90, 60, 100]) })
  expect(track.events).toHaveLength(before)
  player.position = 0
  recorder.onMessage({ data: new Uint8Array([0x90, 60, 100]) })
  expect(track.events).toHaveLength(before + 1)
  expect(track.events.filter((e) => e.type === 'channel' && e.subtype === 'note').map((e) => e.tick)).toEqual([0])
})
