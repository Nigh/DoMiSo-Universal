import { act } from "react"
import { createRoot } from "react-dom/client"
import { observable, runInAction } from "mobx"
import { afterEach, beforeEach, expect, it, vi } from "vitest"
import { TrackId } from "@signal-app/core"
import { intersectsTile, useTileNotes } from "./PlaybackRoll"

const fixture = vi.hoisted(() => ({ store: null as any }))
vi.mock("../../../hooks/useStores", () => ({ useStores: () => fixture.store }))
vi.mock("../../../hooks/useTrack", () => ({ useTrack: () => ({ isRhythmTrack: false }) }))
const getRect = vi.fn((note: any) => ({ x: note.tick, y: 0, width: note.duration, height: 16 }))
const transform = { getX: (tick: number) => tick, pixelsPerKey: 16, getRect }
const selectedNoteIds: number[] = []
vi.mock("../../../hooks/usePianoRoll", () => ({ usePianoRoll: () => ({ transform, selectedNoteIds }) }))

const root = createRoot(document.createElement("div"))
beforeEach(() => vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true))
afterEach(() => { act(() => root.render(null)); vi.clearAllMocks(); vi.unstubAllGlobals() })

it("includes long notes across every intersected tile and excludes notes outside the cache", () => {
  expect(intersectsTile(50, 300, 100, 100)).toBe(true)
  expect(intersectsTile(50, 300, 200, 100)).toBe(true)
  expect(intersectsTile(50, 300, 300, 100)).toBe(true)
  expect(intersectsTile(50, 300, 400, 100)).toBe(false)
  expect(intersectsTile(200, 16, 100, 100)).toBe(false)
})

it("reuses geometry on unrelated renders, invalidates on MIDI edits and song replacement, and unsubscribes on exit", () => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true)
  const event = (id: number, tick: number) => ({ type: "channel", subtype: "note", id, tick, duration: 300, velocity: 100, noteNumber: 60 })
  const events = observable.array([event(1, 50)], { deep: false })
  const store = observable({ song: { getTrack: () => ({ events }) } }, { song: observable.ref })
  fixture.store = { songStore: store }
  let notes: ReturnType<typeof useTileNotes>
  function Probe({ revision }: { revision: number }) { notes = useTileNotes(1 as TrackId, 100, 100); return <span>{revision}</span> }
  act(() => root.render(<Probe revision={0} />))
  expect(notes![0]).toMatchObject({ x: -50, width: 300 })
  const geometry = getRect.mock.calls.length
  act(() => root.render(<Probe revision={1} />))
  expect(getRect).toHaveBeenCalledTimes(geometry)
  act(() => runInAction(() => events[0] = event(1, 75)))
  expect(notes![0].x).toBe(-25)
  act(() => runInAction(() => events.push(event(2, 150))))
  expect(notes!).toHaveLength(2)
  const replacement = observable.array([event(3, 1000)], { deep: false })
  act(() => runInAction(() => store.song = { getTrack: () => ({ events: replacement }) }))
  expect(notes!).toEqual([])
  act(() => root.render(null))
  const afterExit = getRect.mock.calls.length
  runInAction(() => events.push(event(4, 175)))
  expect(getRect).toHaveBeenCalledTimes(afterExit)
})
