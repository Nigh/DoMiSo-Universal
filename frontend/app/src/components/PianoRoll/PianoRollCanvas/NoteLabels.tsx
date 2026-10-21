import { useTheme } from "@emotion/react"
import { useAtomValue } from "jotai"
import { useCallback } from "react"
import { noteLabel } from "../../../helpers/pianoRollLabels"
import { useNoteLabels } from "../../../hooks/useNoteLabels"
import { PianoNoteItem, useNotes } from "../../../hooks/useNotes"
import { usePianoRoll } from "../../../hooks/usePianoRoll"
import { useSettings } from "../../../hooks/useSettings"
import { useTickScroll } from "../../../hooks/useTickScroll"
import { useKeyScroll } from "../../../hooks/useKeyScroll"
import { activeProfileAtom, mappingStateAtom } from "../../../performance/mappingState"
import DrawCanvas from "../../DrawCanvas"
import { useTrack } from "../../../hooks/useTrack"

// A transparent canvas shares labels across both WebGL renderers and supports Unicode.
export function NoteLabels({ width, height }: { width: number; height: number }) {
  const notes = useNotes()
  const { scrollLeft } = useTickScroll()
  return <NoteLabelsContent notes={notes} width={width} height={height} scrollLeft={scrollLeft} />
}

export function NoteLabelsContent({ notes, width, height, scrollLeft }: { notes: PianoNoteItem[]; width: number; height: number; scrollLeft: number }) {
  const { selectedTrackId } = usePianoRoll()
  const { isRhythmTrack } = useTrack(selectedTrackId)
  const { showNoteLabels } = useSettings()
  const { mode, tonic } = useNoteLabels()
  const profile = useAtomValue(activeProfileAtom)
  const state = useAtomValue(mappingStateAtom)
  const { scrollTop } = useKeyScroll()
  const theme = useTheme()
  const draw = useCallback((ctx: CanvasRenderingContext2D) => {
    ctx.clearRect(0, 0, width, height)
    if (!showNoteLabels || isRhythmTrack) return
    for (const note of notes) {
      const x = note.x - scrollLeft
      const y = note.y - scrollTop
      if (x + note.width < 0 || x > width || y + note.height < 0 || y > height || note.width < 5) continue
      const label = noteLabel(note.noteNumber, mode, tonic, profile, state)
      ctx.save()
      ctx.beginPath()
      ctx.rect(x + 1, y + 1, Math.max(0, note.width - 2), note.height - 2)
      ctx.clip()
      const size = Math.max(7, Math.min(12, note.height * (mode === "number" ? 0.55 : 0.7)))
      ctx.font = `${size}px ${theme.canvasFont}`
      ctx.textBaseline = "middle"
      ctx.fillStyle = note.isSelected ? (theme.isLightContent ? theme.backgroundColor : theme.textColor) : theme.onSurfaceColor
      ctx.fillText(label.text, x + 3, y + note.height / 2)
      // Octave dots above/below the scale degree; multiple octaves use horizontal dots.
      const dots = Math.abs(label.octave)
      const center = x + 3 + ctx.measureText(label.text).width / 2
      for (let i = 0; i < dots; i++) {
        ctx.beginPath()
        ctx.arc(center + (i - (dots - 1) / 2) * 2.5, y + note.height / 2 + (label.octave > 0 ? -1 : 1) * (size / 2 + 2), 0.8, 0, Math.PI * 2)
        ctx.fill()
      }
      ctx.restore()
    }
  }, [notes, width, height, showNoteLabels, isRhythmTrack, mode, tonic, profile, state, scrollLeft, scrollTop, theme])
  return <DrawCanvas width={width} height={height} draw={draw} style={{ position: "absolute", inset: 0, pointerEvents: "none" }} />
}
