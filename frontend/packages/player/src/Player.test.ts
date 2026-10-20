import { describe, expect, it, vi } from "vitest"
import { Player } from "./Player"
import { EventScheduler } from "./EventScheduler"

describe("pre-roll transport", () => {
  it("clamps to two blank measures without changing MIDI tick zero", () => {
    const source = { startTick: -3840, timebase: 480, endOfSong: 960, getEvents: () => [], getCurrentStateEvents: vi.fn(() => []) }
    const player = new Player({ activate() {}, sendEvent() {} }, source)
    player.position = -5000
    expect(player.position).toBe(-3840)
    player.position = -100.2
    expect(player.position).toBe(-101)
    expect(source.getCurrentStateEvents).toHaveBeenLastCalledWith(-101)
    player.position = 0
    expect(player.position).toBe(0)
    player.position = Infinity
    expect(player.position).toBe(0)
  })

  it("seeks into pre-roll and schedules tick zero only when reached", () => {
    const scheduler = new EventScheduler((start, end) => start <= 0 && end > 0 ? [{ tick: 0 }] : [], () => [], 0, 480, 0)
    scheduler.readNextEvents(120, 0)
    scheduler.seek(-960)
    expect(scheduler.readNextEvents(120, 1000)).toEqual([])
    expect(scheduler.scheduledTick).toBe(-960)
    expect(scheduler.readNextEvents(120, 1500)).toEqual([])
    expect(scheduler.readNextEvents(120, 2001)).toEqual([{ event: { tick: 0 }, timestamp: 2001 }])
  })
})
