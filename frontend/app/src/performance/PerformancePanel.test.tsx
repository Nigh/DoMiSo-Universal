import { act } from "react"
import { createRoot } from "react-dom/client"
import { ThemeProvider } from "@emotion/react"
import { afterEach, expect, it, vi } from "vitest"
import { TrackId } from "@signal-app/core"
import { GroupOutput } from "../services/GroupOutput"
import { PerformancePanel } from "./PerformancePanel"

const state = vi.hoisted(() => ({ store: {} as any, track: { events: [] } }))
vi.mock("../hooks/useStores", () => ({ useStores: () => state.store }))
vi.mock("../hooks/usePianoRoll", () => ({ usePianoRoll: () => ({ selectedTrack: state.track }) }))
vi.mock("../hooks/useTrackMute", () => ({ useTrackMute: () => ({ trackMute: { mutes: {}, solos: {} } }) }))
vi.mock("../hooks/useMobxSelector", () => ({ useMobxSelector: (fn: () => unknown) => fn() }))
vi.mock("@wailsio/runtime", () => ({ Dialogs: {}, Events: { On: () => () => {} } }))
vi.mock("../../../bindings/domiso-universal", () => ({ AppService: {
  RegisterPerformanceHotkeys: async () => {}, SetGamePracticeMonitoring: async () => {},
} }))
vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true)
afterEach(() => { vi.useRealTimers(); document.body.innerHTML = "" })

it("flashes scheduled playback only when due, cancels on stop, and never replays stale input flashes", async () => {
  vi.useFakeTimers()
  const midiListeners = new Set<(message: { data: Uint8Array }) => void>()
  const synthGroup = new GroupOutput({ activate() {}, sendEvent() {} })
  const player = { position: 0, isPlaying: true, play: vi.fn(), stop: vi.fn(() => { player.isPlaying = false }) }
  state.store = { player, songStore: { song: {} }, synthGroup, midiInput: {
    on: (_: string, listener: (message: { data: Uint8Array }) => void) => {
      midiListeners.add(listener)
      return () => midiListeners.delete(listener)
    },
  } }
  const host = document.createElement("div")
  host.id = "performance-keyboard"
  const container = document.createElement("div")
  document.body.append(host, container)
  const root = createRoot(container)
  await act(async () => root.render(<ThemeProvider theme={{ secondaryBackgroundColor: '#222' } as any}><PerformancePanel /></ThemeProvider>))
  const note = { type: "channel", subtype: "noteOn", channel: 0, noteNumber: 60, velocity: 100 } as const
  const flashes = () => Array.from(host.querySelectorAll(".press-flash")).map((element) => element.parentElement?.textContent)
  act(() => synthGroup.sendEvent(note, 0.05, performance.now(), 1 as TrackId))
  expect(flashes()).toEqual([])
  act(() => vi.advanceTimersByTime(50))
  expect(flashes()).toEqual(["AC4"])
  act(() => host.querySelector(".press-flash")!.dispatchEvent(new Event("webkitAnimationEnd", { bubbles: true })))
  expect(flashes()).toEqual([])
  act(() => {
    synthGroup.sendEvent({ ...note, noteNumber: 62 }, 0.05, performance.now(), 1 as TrackId)
    player.isPlaying = false
    synthGroup.sendEvent({ type: "channel", subtype: "controller", channel: 0, controllerType: 120, value: 0 }, 0, performance.now())
    vi.advanceTimersByTime(100)
  })
  expect(flashes()).toEqual([])
  act(() => midiListeners.forEach((listener) => listener({ data: new Uint8Array([0x90, 64, 100]) })))
  expect(flashes()).toEqual(["DE4"])
  act(() => host.querySelector(".press-flash")!.dispatchEvent(new Event("webkitAnimationEnd", { bubbles: true })))
  act(() => window.dispatchEvent(new CustomEvent("performance-keyboard-host", { detail: host })))
  expect(flashes()).toEqual([])
  // Enabling practice while manually paused must not start the next group.
  state.track.events = [
    { type: "channel", subtype: "note", tick: 0, noteNumber: 60 },
    { type: "channel", subtype: "note", tick: 480, noteNumber: 62 },
  ] as any
  await act(async () => container.querySelector<HTMLButtonElement>('[role="checkbox"]')!.click())
  act(() => midiListeners.forEach((listener) => listener({ data: new Uint8Array([0x90, 60, 100]) })))
  expect(player.play).not.toHaveBeenCalled()
  expect(flashes()).toEqual(["AC4"])
  act(() => midiListeners.forEach((listener) => listener({ data: new Uint8Array([0x80, 60, 0]) })))
  player.isPlaying = true
  player.position = 1
  await act(async () => root.render(<ThemeProvider theme={{ secondaryBackgroundColor: '#222' } as any}><PerformancePanel /></ThemeProvider>))
  expect(player.stop).toHaveBeenCalled()
  act(() => midiListeners.forEach((listener) => listener({ data: new Uint8Array([0x90, 60, 100]) })))
  expect(player.play).toHaveBeenCalledTimes(1)
  act(() => root.unmount())
})
