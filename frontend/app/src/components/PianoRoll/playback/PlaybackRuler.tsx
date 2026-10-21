import { useTheme } from "@emotion/react"
import { memo, useCallback, useMemo } from "react"
import { Layout } from "../../../Constants"
import { BeatWithX } from "../../../entities/beat/BeatWithX"
import { useSong } from "../../../hooks/useSong"
import { useTickScroll } from "../../../hooks/useTickScroll"
import DrawCanvas from "../../DrawCanvas"
import { drawRuler, drawTimeSignatures } from "../rulerDrawing"
import { PlaybackTile, usePlaybackViewport } from "./PlaybackViewport"

const RulerTile = memo(function RulerTile({ origin, width, selected }: { origin: number; width: number; selected: Set<number> }) {
  const { measures, timebase, timeSignatures } = useSong()
  const { transform } = useTickScroll()
  const theme = useTheme()
  const beats = useMemo(() => {
    const beats = BeatWithX.createInRange(measures, transform, timebase, origin - 100, width + 100)
    const omit = beats.length > 1 && beats[1].x - beats[0].x <= 5
    return beats.filter((beat) => beat.beat === 0 || !omit).map((beat) => ({ ...beat, x: beat.x - origin, label: beat.beat === 0 && (!omit || beat.measure % 2 === 0) ? `${beat.measure + 1}` : null }))
  }, [measures, transform, timebase, origin, width])
  const signatures = useMemo(() => timeSignatures.filter((event) => {
    const x = transform.getX(event.tick)
    // Include flags starting in the previous tile so text is not cut at seams.
    return x >= origin - 100 && x < origin + width
  }).map((event) => ({ x: transform.getX(event.tick) - origin, label: `${event.numerator}/${event.denominator}`, isSelected: selected.has(event.id) })), [timeSignatures, transform, selected, origin, width])
  const draw = useCallback((ctx: CanvasRenderingContext2D) => {
    ctx.clearRect(0, 0, width, Layout.rulerHeight)
    ctx.save()
    ctx.translate(0.5, 0)
    drawRuler(ctx, Layout.rulerHeight, beats, theme)
    drawTimeSignatures(ctx, Layout.rulerHeight, signatures, theme)
    ctx.restore()
  }, [width, beats, signatures, theme])
  return <DrawCanvas width={width} height={Layout.rulerHeight} draw={draw} />
})

export function PlaybackRuler({ selected }: { selected: Set<number> }) {
  const viewport = usePlaybackViewport()!
  return <div style={{ position: "absolute", inset: 0, overflow: "hidden", pointerEvents: "none" }}>
    {viewport.width > 0 && viewport.ids.map((id, slot) => <PlaybackTile key={slot} origin={id * viewport.width}><RulerTile key={viewport.dpr} origin={id * viewport.width} width={viewport.width} selected={selected} /></PlaybackTile>)}
  </div>
}
