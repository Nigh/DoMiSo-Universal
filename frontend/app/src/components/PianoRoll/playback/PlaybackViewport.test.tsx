import { act, memo, StrictMode } from "react"
import { createRoot } from "react-dom/client"
import { afterEach, beforeEach, expect, it, vi } from "vitest"
import { PlaybackTile, PlaybackViewport, playbackTileIds, usePlaybackViewport } from "./PlaybackViewport"

const state = vi.hoisted(() => ({ tick: 0, scale: 1 }))
vi.mock("../../../hooks/useStores", () => ({ useStores: () => ({ player }) }))
const player = { samplePlaybackPosition: () => state.tick }
const transform = { getX: (tick: number) => tick * state.scale }
vi.mock("../../../hooks/useTickScroll", () => ({ useTickScroll: () => ({ transform }) }))

const frames = new Map<number, FrameRequestCallback>()
let id = 0
const container = document.createElement("div")
const root = createRoot(container)
beforeEach(() => vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true))
afterEach(() => { act(() => root.render(null)); frames.clear(); vi.unstubAllGlobals(); state.tick = 0; state.scale = 1 })

it("keeps four reusable slots across negative pre-roll, boundaries, and arbitrary seeks", () => {
  expect(playbackTileIds(-50, 100)).toEqual([0, 1, -2, -1])
  const before = playbackTileIds(10, 100)
  const after = playbackTileIds(110, 100)
  expect(after.filter((id, slot) => id !== before[slot])).toHaveLength(1)
  for (const left of [-5000, 0, 1000000]) {
    const ids = playbackTileIds(left, 100)
    expect(new Set(ids).size).toBe(4)
    expect(ids).toContain(Math.floor(left / 100))
    expect(ids).toContain(Math.floor(left / 100) + 1)
  }
})

it("moves cached surfaces every frame without rerendering their contents, refreshes one slot at a boundary, and cancels on unmount", () => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true)
  vi.stubGlobal("requestAnimationFrame", (fn: FrameRequestCallback) => { frames.set(++id, fn); return id })
  vi.stubGlobal("cancelAnimationFrame", (key: number) => frames.delete(key))
  const draw = vi.fn()
  const CachedContent = memo(({ origin, dpr }: { origin: number; dpr: number }) => { draw(origin, dpr); return <canvas /> })
  function Tiles() {
    const viewport = usePlaybackViewport()!
    return <>{viewport.ids.map((id, slot) => <PlaybackTile key={slot} origin={id * 100}><CachedContent origin={id * 100} dpr={viewport.dpr} /></PlaybackTile>)}</>
  }
  const nextFrame = () => act(() => {
    const pending = [...frames.values()]
    frames.clear()
    pending.forEach((fn) => fn(0))
  })
  act(() => root.render(<StrictMode><PlaybackViewport width={100}><Tiles /></PlaybackViewport></StrictMode>))
  expect(frames.size).toBe(1)
  const initial = draw.mock.calls.length
  for (let i = 1; i < 40; i++) { state.tick = i; nextFrame() }
  expect(draw).toHaveBeenCalledTimes(initial)
  expect(container.firstElementChild?.getAttribute("style")).toContain("translate3d(11px,0,0)")
  state.tick = 60
  nextFrame()
  expect(draw.mock.calls.length - initial).toBe(2) // StrictMode invokes the one changed slot twice.
  const atBoundary = draw.mock.calls.length
  state.tick = 75
  nextFrame()
  expect(draw).toHaveBeenCalledTimes(atBoundary)
  vi.stubGlobal("devicePixelRatio", 2)
  nextFrame()
  expect(draw.mock.calls.length).toBeGreaterThan(atBoundary)
  act(() => root.render(null))
  expect(frames.size).toBe(0)
})
