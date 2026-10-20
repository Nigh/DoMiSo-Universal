import { act } from "react"
import { createRoot } from "react-dom/client"
import { createStore } from "jotai"
import { afterEach, expect, it, vi } from "vitest"
import { createTickScrollScope, TickScrollProvider, useTickScroll } from "./useTickScroll"

const state = vi.hoisted(() => ({ playback: false, player: { position: 0, isPlaying: false } }))
vi.mock("./useEditorMode", () => ({ useEditorMode: () => [state.playback] }))
vi.mock("./usePlayer", () => ({ usePlayer: () => state.player }))
vi.mock("./useSong", () => ({ useSong: () => ({ endOfSong: 10000, timebase: 480, measures: [{ tick: 0, measure: 0, numerator: 4, denominator: 4 }] }) }))
vi.mock("./useStores", () => ({ useStores: () => ({ player: state.player }) }))
vi.mock("./useMobxSelector", () => ({ useMobxSelector: (fn: () => unknown) => fn() }))

vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true)

const container = document.createElement("div")
const root = createRoot(container)
afterEach(() => { act(() => root.render(null)); state.playback = false })

it("shows pre-roll in edit mode and centers seeking/playback/zoom even when auto-scroll is off", () => {
  const scope = createTickScrollScope(createStore())
  let scroll: Pick<ReturnType<typeof useTickScroll>, "scrollLeft" | "cursorX" | "transform" | "startTick" | "setCanvasWidth" | "setAutoScroll" | "setScaleX" | "setScrollLeftInTicks">
  function Probe() {
    const { scrollLeft, cursorX, transform, startTick, setCanvasWidth, setAutoScroll, setScaleX, setScrollLeftInTicks } = useTickScroll()
    scroll = { scrollLeft, cursorX, transform, startTick, setCanvasWidth, setAutoScroll, setScaleX, setScrollLeftInTicks }
    return null
  }
  const render = () => act(() => root.render(<TickScrollProvider scope={scope} minScaleX={0.15} maxScaleX={15} pianoRoll><Probe /></TickScrollProvider>))
  render()
  act(() => scroll.setCanvasWidth(800))
  expect(scroll!.startTick).toBe(-3840)
  expect(scroll!.scrollLeft).toBe(scroll!.transform.getX(-3840))
  act(() => scroll.setScrollLeftInTicks(-10000))
  expect(scroll!.scrollLeft).toBe(scroll!.transform.getX(-3840))
  act(() => scroll.setAutoScroll(false))
  state.playback = true
  render()
  expect(scroll!.cursorX - scroll!.scrollLeft).toBeCloseTo(400)
  state.player.position = -3840
  render()
  expect(scroll!.cursorX - scroll!.scrollLeft).toBeCloseTo(400)
  act(() => scroll.setScaleX(2))
  expect(scroll!.cursorX - scroll!.scrollLeft).toBeCloseTo(400)
  state.player.isPlaying = true
  state.player.position = 480
  render()
  expect(scroll!.cursorX - scroll!.scrollLeft).toBeCloseTo(400)
  act(() => root.render(null))
})
