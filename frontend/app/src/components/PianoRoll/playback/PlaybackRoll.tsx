import { useTheme } from "@emotion/react"
import { GLCanvas, GLFallback, Transform } from "@ryohey/webgl-react"
import { isNoteEvent, TrackId } from "@signal-app/core"
import { memo, useEffect, useMemo } from "react"
import { comparer } from "mobx"
import { BeatWithX } from "../../../entities/beat/BeatWithX"
import { matrixFromTranslation } from "../../../helpers/matrix"
import { useGhostNoteColor } from "../../../hooks/useGhostNoteColor"
import { useKeyScroll } from "../../../hooks/useKeyScroll"
import { usePianoRoll } from "../../../hooks/usePianoRoll"
import { useSong } from "../../../hooks/useSong"
import { useTickScroll } from "../../../hooks/useTickScroll"
import { useTrack } from "../../../hooks/useTrack"
import { useStores } from "../../../hooks/useStores"
import { useMobxSelector } from "../../../hooks/useMobxSelector"
import { useDragScrollGesture } from "../MouseHandler/gestures/useDragScrollGesture"
import { Beats } from "../../GLNodes/Beats"
import { ReleaseContext } from "../../GLNodes/ReleaseContext"
import { Lines } from "../PianoRollCanvas/Lines"
import { NoteLabelsContent } from "../PianoRollCanvas/NoteLabels"
import { NotesContent } from "../PianoRollCanvas/Notes"
import { NoteCircles } from "../PianoRollCanvas/NoteCircles"
import { NoteRectangles, NoteRectanglesProps } from "../PianoRollCanvas/NoteRectangles"
import { NoteCircles as LegacyCircles } from "../PianoRollCanvas/lagacy/NoteCircles"
import { NoteRectangles as LegacyRectangles } from "../PianoRollCanvas/lagacy/NoteRectangles"
import { PlaybackTile, usePlaybackViewport } from "./PlaybackViewport"

export function intersectsTile(x: number, width: number, origin: number, tileWidth: number) {
  return x < origin + tileWidth && x + width >= origin
}

export function useTileNotes(trackId: TrackId, origin: number, width: number, ghost = false) {
  const { isRhythmTrack } = useTrack(trackId)
  const { songStore } = useStores()
  // Track.events is a shallow observable array mutated in place by MIDI edits.
  const events = useMobxSelector(() => songStore.song.getTrack(trackId)?.events.slice() ?? [], [songStore, trackId], comparer.shallow)
  const { transform, selectedNoteIds } = usePianoRoll()
  return useMemo(() => {
    const selected = new Set(selectedNoteIds)
    return events.filter(isNoteEvent).flatMap((event) => {
      // Cull before making geometry, including drum circles and notes spanning several tiles.
      const x = transform.getX(event.tick) - (isRhythmTrack ? transform.pixelsPerKey / 2 : 0)
      const size = isRhythmTrack ? transform.pixelsPerKey : transform.getX(event.duration)
      if (!intersectsTile(x, size, origin, width)) return []
      const rect = isRhythmTrack ? transform.getDrumRect(event) : transform.getRect(event)
      return [{ ...rect, x: rect.x - origin, id: event.id, noteNumber: event.noteNumber, velocity: ghost ? 127 : event.velocity, isSelected: !ghost && selected.has(event.id) }]
    })
  }, [events, isRhythmTrack, transform, selectedNoteIds, origin, width, ghost])
}

const GhostTile = memo(function GhostTile({ trackId, origin, width }: { trackId: TrackId; origin: number; width: number }) {
  const notes = useTileNotes(trackId, origin, width, true)
  const { isRhythmTrack } = useTrack(trackId)
  const style = useGhostNoteColor(trackId)
  return isRhythmTrack
    ? <GLFallback component={NoteCircles} fallback={GhostLegacyCircles} rects={notes} {...style} zIndex={2} />
    : <GLFallback component={NoteRectangles} fallback={GhostLegacyRectangles} rects={notes} {...style} zIndex={2} />
})

function GhostLegacyRectangles({ rects, activeColor, strokeColor, zIndex }: NoteRectanglesProps) {
  const colored = useMemo(() => rects.map((note) => ({ ...note, color: activeColor })), [rects, activeColor])
  return <LegacyRectangles rects={colored} strokeColor={strokeColor} zIndex={zIndex} />
}

function GhostLegacyCircles({ rects, activeColor, strokeColor, zIndex }: NoteRectanglesProps) {
  const colored = useMemo(() => rects.map((note) => ({ ...note, color: activeColor })), [rects, activeColor])
  return <LegacyCircles rects={colored} strokeColor={strokeColor} zIndex={zIndex} />
}

const RollTile = memo(function RollTile({ origin, width, height, dpr }: { origin: number; width: number; height: number; dpr: number }) {
  const { selectedTrackId, ghostTrackIds } = usePianoRoll()
  const notes = useTileNotes(selectedTrackId, origin, width)
  const { scrollTop } = useKeyScroll()
  const { measures, timebase } = useSong()
  const { transform } = useTickScroll()
  const theme = useTheme()
  const matrix = useMemo(() => matrixFromTranslation(0, -scrollTop), [scrollTop])
  const beats = useMemo(() => BeatWithX.createInRange(measures, transform, timebase, origin, width).map((beat) => ({ ...beat, x: beat.x - origin })), [measures, transform, timebase, origin, width])
  return <div style={{ position: "relative", width, height }}>
    <GLCanvas key={dpr} width={width} height={height} style={{ backgroundColor: theme.editorBackgroundColor }}>
      <ReleaseContext />
      <Transform matrix={matrix}>
        <Lines zIndex={0} />
        {ghostTrackIds.map((trackId) => <GhostTile key={trackId} trackId={trackId} origin={origin} width={width} />)}
        <NotesContent notes={notes} zIndex={3} />
      </Transform>
      <Beats height={height} beats={beats} zIndex={1} />
    </GLCanvas>
    <NoteLabelsContent key={`labels-${dpr}`} notes={notes} width={width} height={height} scrollLeft={0} />
  </div>
})

export const PlaybackRoll = memo(function PlaybackRoll({ width, height }: { width: number; height: number }) {
  const viewport = usePlaybackViewport()!
  const { setCanvasWidth } = useTickScroll()
  const { setCanvasHeight } = useKeyScroll()
  const drag = useDragScrollGesture()
  useEffect(() => setCanvasWidth(width), [setCanvasWidth, width])
  useEffect(() => setCanvasHeight(height), [setCanvasHeight, height])
  return <div style={{ position: "relative", width, height: Math.max(0, height), overflow: "hidden", cursor: "grab" }} onMouseDown={(event) => { if (event.button === 0) drag.onMouseDown?.(event.nativeEvent) }}>
    {width > 0 && height > 0 && viewport.ids.map((id, slot) => <PlaybackTile key={slot} origin={id * width}><RollTile origin={id * width} width={width} height={height} dpr={viewport.dpr} /></PlaybackTile>)}
    <div style={{ position: "absolute", left: width / 2, top: 0, bottom: 0, width: 1, background: "red", pointerEvents: "none" }} />
  </div>
})
