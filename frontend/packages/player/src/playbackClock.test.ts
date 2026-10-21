import { afterEach, expect, it, vi } from "vitest"
import { EventScheduler } from "./EventScheduler"
import { Player } from "./Player"

afterEach(() => { vi.useRealTimers(); vi.unstubAllGlobals(); vi.restoreAllMocks() })

it("samples between scheduling calls without reading events or advancing the lookahead frontier", () => {
  const read = vi.fn(() => [])
  const scheduler = new EventScheduler(read, () => [], 0, 480, 100)
  scheduler.readNextEvents(120, 1000)
  expect(scheduler.scheduledTick).toBe(96)
  expect(scheduler.getPlaybackTick(1000)).toBe(0)
  expect(scheduler.getPlaybackTick(1016)).toBeCloseTo(15.36)
  expect(scheduler.getPlaybackTick(1032)).toBeCloseTo(30.72)
  expect(read).toHaveBeenCalledTimes(1)
  expect(scheduler.scheduledTick).toBe(96)
  scheduler.readNextEvents(60, 1050)
  expect(scheduler.getPlaybackTick(1050)).toBeCloseTo(48)
  expect(scheduler.getPlaybackTick(1066)).toBeCloseTo(55.68)
})

it("does not wrap the displayed clock when lookahead crosses the loop early", () => {
  const scheduler = new EventScheduler(() => [], () => [], 900, 480, 100)
  scheduler.loop = { begin: 0, end: 960 }
  scheduler.readNextEvents(120, 0)
  expect(scheduler.scheduledTick).toBe(36)
  expect(scheduler.getPlaybackTick(0)).toBe(900)
  expect(scheduler.getPlaybackTick(50)).toBe(948)
  scheduler.readNextEvents(120, 50)
  expect(scheduler.getPlaybackTick(62.5)).toBe(0)
  expect(scheduler.getPlaybackTick(75)).toBe(12)
  scheduler.readNextEvents(120, 100)
  expect(scheduler.getPlaybackTick(100)).toBe(36)
  scheduler.seek(-480)
  expect(scheduler.getPlaybackTick(1000)).toBe(-480)
  scheduler.readNextEvents(120, 1000)
  expect(scheduler.getPlaybackTick(1250)).toBe(-240)
})

it("pauses at the sampled position, scrubs without scheduling, and stops only at the audible end", () => {
  vi.useFakeTimers({ toFake: ["setInterval", "clearInterval", "performance"] })
  vi.stubGlobal("window", globalThis)
  const source = { startTick: -960, timebase: 480, endOfSong: 192, getEvents: vi.fn(() => []), getCurrentStateEvents: vi.fn(() => []) }
  const output = { activate() {}, sendEvent: vi.fn() }
  const player = new Player(output, source)
  player.play()
  vi.advanceTimersByTime(66)
  expect(player.samplePlaybackPosition()).toBeCloseTo(15.36)
  expect(player.position).toBe(0)
  player.stop()
  expect(player.position).toBeCloseTo(15.36)
  vi.advanceTimersByTime(100)
  expect(player.samplePlaybackPosition()).toBeCloseTo(15.36)
  player.position = -100
  player.play()
  vi.advanceTimersByTime(66)
  player.beginScrub()
  player.position = 20
  const reads = source.getEvents.mock.calls.length
  vi.advanceTimersByTime(500)
  expect(player.samplePlaybackPosition()).toBe(20)
  expect(source.getEvents).toHaveBeenCalledTimes(reads)
  player.endScrub()
  player.position = 0
  vi.advanceTimersByTime(150)
  expect(player.isPlaying).toBe(true)
  expect(player.samplePlaybackPosition()).toBeLessThan(192)
  vi.advanceTimersByTime(150)
  expect(player.isPlaying).toBe(false)
  expect(player.position).toBe(192)
})
