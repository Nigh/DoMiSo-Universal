import { useEffect, useRef } from "react"
import { MouseGesture } from "../../../../gesture/MouseGesture"
import { observeDrag } from "../../../../helpers/observeDrag"
import { useEditorMode } from "../../../../hooks/useEditorMode"
import { useKeyScroll } from "../../../../hooks/useKeyScroll"
import { useStores } from "../../../../hooks/useStores"
import { useTickScroll } from "../../../../hooks/useTickScroll"

export const useDragScrollGesture = (): MouseGesture => {
  const [playbackMode] = useEditorMode()
  const { player } = useStores()
  const { scrollLeft, transform, setScrollLeftInPixels, setAutoScroll } = useTickScroll()
  const { scrollTop, setScrollTopInPixels } = useKeyScroll()
  const finishRef = useRef<(() => void) | null>(null)
  useEffect(() => () => finishRef.current?.(), [])

  return {
    onMouseDown(start) {
      finishRef.current?.()
      const scrubbing = playbackMode && player.beginScrub()
      const startTick = player.position
      let x = start.clientX
      let y = start.clientY
      let frame: number | null = null
      setAutoScroll(false)
      start.preventDefault()
      const update = () => {
        frame = null
        const dx = x - start.clientX
        const dy = y - start.clientY
        if (scrubbing) player.position = startTick - transform.getTick(dx)
        else if (!playbackMode) setScrollLeftInPixels(scrollLeft - dx)
        setScrollTopInPixels(scrollTop - dy)
      }
      const finish = () => {
        if (frame !== null) { cancelAnimationFrame(frame); update() }
        cleanup()
        window.removeEventListener("blur", finish)
        finishRef.current = null
        if (scrubbing) player.endScrub()
      }
      const cleanup = observeDrag({
        onMouseMove: (event) => {
          x = event.clientX
          y = event.clientY
          if (frame === null) frame = requestAnimationFrame(update)
        },
        onMouseUp: (event) => {
          x = event.clientX
          y = event.clientY
          if (frame !== null) cancelAnimationFrame(frame)
          update()
          finish()
        },
      })
      finishRef.current = finish
      window.addEventListener("blur", finish)
    },
  }
}
