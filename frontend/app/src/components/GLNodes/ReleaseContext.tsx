import { useRenderer } from "@ryohey/webgl-react"
import { useEffect, useRef } from "react"

// The renderer exposes no disposal API; a dedicated canvas context owns all of
// its programs, VAOs and buffers and can release them together on unmount.
export function ReleaseContext() {
  const renderer = useRenderer()
  const generation = useRef(0)
  useEffect(() => {
    const current = ++generation.current
    return () => queueMicrotask(() => {
      // StrictMode replays effects on the live context before the next microtask.
      if (generation.current === current) renderer?.gl.getExtension("WEBGL_lose_context")?.loseContext()
    })
  }, [renderer])
  return null
}
