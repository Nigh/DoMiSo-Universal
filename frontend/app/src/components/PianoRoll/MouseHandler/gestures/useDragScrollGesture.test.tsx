import { act } from "react"
import { createRoot } from "react-dom/client"
import { afterEach, expect, it, vi } from "vitest"
import { useDragScrollGesture } from "./useDragScrollGesture"

const state = vi.hoisted(() => ({
  playback: true,
  player: { position: 1000, beginScrub: vi.fn(() => true), endScrub: vi.fn() },
  horizontal: vi.fn(), vertical: vi.fn(), auto: vi.fn(),
}))
vi.mock("../../../../hooks/useEditorMode", () => ({ useEditorMode: () => [state.playback] }))
vi.mock("../../../../hooks/useStores", () => ({ useStores: () => ({ player: state.player }) }))
vi.mock("../../../../hooks/useTickScroll", () => ({ useTickScroll: () => ({
  scrollLeft: 200, transform: { getTick: (x: number) => x / 0.5 }, setScrollLeftInPixels: state.horizontal, setAutoScroll: state.auto,
}) }))
vi.mock("../../../../hooks/useKeyScroll", () => ({ useKeyScroll: () => ({ scrollTop: 100, setScrollTopInPixels: state.vertical }) }))
afterEach(() => { vi.unstubAllGlobals(); vi.clearAllMocks() })

it.each([true, false])("pans by CSS pixel displacement, coalesces moves and flushes mouseup (playback=%s)", (playback) => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true)
  state.playback = playback
  state.player.position = 1000
  const frames = new Map<number, FrameRequestCallback>()
  let id = 0
  const request = vi.fn((callback: FrameRequestCallback) => { frames.set(++id, callback); return id })
  vi.stubGlobal("requestAnimationFrame", request)
  vi.stubGlobal("cancelAnimationFrame", (key: number) => frames.delete(key))
  let gesture: ReturnType<typeof useDragScrollGesture>
  function Probe() { gesture = useDragScrollGesture(); return null }
  const root = createRoot(document.createElement("div"))
  act(() => root.render(<Probe />))
  act(() => gesture.onMouseDown(new MouseEvent("mousedown", { clientX: 100, clientY: 100 })))
  for (let x = 101; x <= 400; x++) {
    document.dispatchEvent(new MouseEvent("mousemove", { clientX: x, clientY: 120 }))
  }
  expect(request).toHaveBeenCalledTimes(1)
  expect(state.player.position).toBe(1000)
  act(() => { const callback = frames.get(1)!; frames.delete(1); callback(0) })
  if (playback) expect(state.player.position).toBe(400)
  else expect(state.horizontal).toHaveBeenLastCalledWith(-100)
  expect(state.vertical).toHaveBeenLastCalledWith(80)
  // A final movement before the next frame must not be lost.
  document.dispatchEvent(new MouseEvent("mousemove", { clientX: 110, clientY: 110 }))
  document.dispatchEvent(new MouseEvent("mouseup", { clientX: 120, clientY: 110 }))
  if (playback) {
    expect(state.player.position).toBe(960)
    expect(state.player.endScrub).toHaveBeenCalledTimes(1)
  } else expect(state.horizontal).toHaveBeenLastCalledWith(180)
  expect(frames.size).toBe(0)
  const count = request.mock.calls.length
  document.dispatchEvent(new MouseEvent("mousemove", { clientX: 500 }))
  expect(request).toHaveBeenCalledTimes(count)
  act(() => root.unmount())
})
