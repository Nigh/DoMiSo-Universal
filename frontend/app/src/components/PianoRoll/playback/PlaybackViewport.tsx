import { createContext, memo, ReactNode, useContext, useLayoutEffect, useMemo, useRef, useState } from "react"
import { useStores } from "../../../hooks/useStores"
import { useTickScroll } from "../../../hooks/useTickScroll"

export function playbackTileIds(left: number, width: number): number[] {
  const first = Math.floor(left / Math.max(1, width)) - 1
  const ids = Array<number>(4)
  for (let id = first; id < first + 4; id++) ids[((id % 4) + 4) % 4] = id
  return ids
}

type Frame = { left: number; dpr: number }
type Viewport = {
  width: number
  ids: number[]
  dpr: number
  read: () => Frame
  subscribe: (listener: (frame: Frame) => void) => () => void
}
const Context = createContext<Viewport | null>(null)
export const usePlaybackViewport = () => useContext(Context)

// One display clock serves the roll, cached ruler, and dynamic overlays.
export function PlaybackViewport({ width, children }: { width: number; children: ReactNode }) {
  const { player } = useStores()
  const { transform } = useTickScroll()
  const frame = useRef<Frame>({ left: transform.getX(player.samplePlaybackPosition()) - width / 2, dpr: window.devicePixelRatio || 1 })
  const listeners = useRef(new Set<(frame: Frame) => void>())
  const [cache, setCache] = useState(() => ({ index: Math.floor(frame.current.left / Math.max(1, width)), dpr: frame.current.dpr }))
  const source = useMemo(() => ({
    read: () => frame.current,
    subscribe: (listener: (frame: Frame) => void) => {
      listeners.current.add(listener)
      listener(frame.current)
      return () => { listeners.current.delete(listener) }
    },
  }), [])
  useLayoutEffect(() => {
    let request = 0
    let index = NaN
    let dpr = NaN
    const update = () => {
      const next = { left: transform.getX(player.samplePlaybackPosition()) - width / 2, dpr: window.devicePixelRatio || 1 }
      const nextIndex = Math.floor(next.left / Math.max(1, width))
      if (nextIndex !== index || next.dpr !== dpr) {
        index = nextIndex
        dpr = next.dpr
        setCache({ index, dpr })
      }
      if (next.left !== frame.current.left || next.dpr !== frame.current.dpr) {
        frame.current = next
        listeners.current.forEach((listener) => listener(next))
      }
      request = requestAnimationFrame(update)
    }
    update()
    return () => cancelAnimationFrame(request)
  }, [player, width, transform])
  const value = useMemo(() => ({ ...source, width, ids: playbackTileIds(cache.index * Math.max(1, width), width), dpr: cache.dpr }), [source, width, cache])
  return <Context.Provider value={value}>{children}</Context.Provider>
}

// Slot keys recycle four surfaces rather than creating a WebGL context at every boundary.
export const PlaybackTile = memo(function PlaybackTile({ origin, children }: { origin: number; children: ReactNode }) {
  const viewport = usePlaybackViewport()!
  const ref = useRef<HTMLDivElement>(null)
  useLayoutEffect(() => viewport.subscribe(({ left }) => {
    if (ref.current) ref.current.style.transform = `translate3d(${origin - left}px,0,0)`
  }), [viewport, origin])
  return <div ref={ref} style={{ position: "absolute", top: 0, left: 0, width: viewport.width, overflow: "hidden", willChange: "transform", pointerEvents: "none" }}>{children}</div>
})
