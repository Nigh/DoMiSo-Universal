import { expect, it, vi } from "vitest"
import { TrackId } from "@signal-app/core"
import { GroupOutput } from "./GroupOutput"
import { METRONOME_TRACK_ID } from "../player/EventSource"
import { TrackMute } from "../trackMute/TrackMute"

it("observes audible playback with timing, excludes muted tracks and metronome, and unsubscribes", () => {
  const output = new GroupOutput({ activate() {}, sendEvent() {} })
  const listener = vi.fn()
  const off = output.onPlayback(listener)
  const event = { type: "channel", subtype: "noteOn", channel: 0, noteNumber: 60, velocity: 100 } as const
  const track = 1 as TrackId
  output.sendEvent(event, 0.05, 100, track)
  expect(listener).toHaveBeenLastCalledWith(event, 0.05, 100, track)
  output.trackMute = TrackMute.mute(track)(TrackMute.empty)
  output.sendEvent(event, 0, 100, track)
  output.sendEvent(event, 0, 100, METRONOME_TRACK_ID)
  expect(listener).toHaveBeenCalledTimes(1)
  off()
  output.sendEvent(event, 0, 100)
  expect(listener).toHaveBeenCalledTimes(1)
})
