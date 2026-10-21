import { useTheme } from "@emotion/react"
import { TrackEventOf } from "@signal-app/core"
import { TimeSignatureEvent } from "midifile-ts"
import React, { FC, useCallback, useLayoutEffect, useRef, useState } from "react"
import { Layout } from "../../Constants"
import { useContextMenu } from "../../hooks/useContextMenu"
import { usePlayer } from "../../hooks/usePlayer"
import { useRuler } from "../../hooks/useRuler"
import { useTickScroll } from "../../hooks/useTickScroll"
import DrawCanvas from "../DrawCanvas"
import { RulerContextMenu } from "./RulerContextMenu"
import { TimeSignatureDialog } from "./TimeSignatureDialog"
import { usePlaybackViewport } from "./playback/PlaybackViewport"
import { PlaybackRuler } from "./playback/PlaybackRuler"

import { drawRuler, drawLoopPoints, drawTimeSignatures } from "./rulerDrawing"

export interface PianoRulerProps {
  onMouseDown?: React.MouseEventHandler<HTMLCanvasElement>
  style?: React.CSSProperties
  className?: string
}

// null = closed
interface TimeSignatureDialogState {
  numerator: number
  denominator: number
}

const PianoRuler: FC<PianoRulerProps> = ({
  onMouseDown: _onMouseDown,
  style,
  className,
}) => {
  const dragScrollLeft = useRef(0)
  const playback = usePlaybackViewport()
  const canvasRef = useRef<DrawCanvas>(null)
  const theme = useTheme()
  const { onContextMenu, menuProps } = useContextMenu()
  const [timeSignatureDialogState, setTimeSignatureDialogState] =
    useState<TimeSignatureDialogState | null>(null)
  const [rightClickTick, setRightClickTick] = useState(0)
  const { loop, setLoopBegin, setLoopEnd, setPosition } = usePlayer()
  const height = Layout.rulerHeight

  const {
    rulerBeats,
    timeSignatures,
    timeSignatureHitTest,
    selectTimeSignature,
    clearSelectedTimeSignature,
    updateTimeSignature,
    getQuantizedTick,
    getTick,
    selectedTimeSignatureEventIds,
  } = useRuler()

  const { canvasWidth: width, scrollLeft, transform, startTick } = useTickScroll()

  const onClickTimeSignature = useCallback(
    (timeSignature: TrackEventOf<TimeSignatureEvent>, e: React.MouseEvent) => {
      if (e.detail == 2) {
        setTimeSignatureDialogState(timeSignature)
      } else {
        selectTimeSignature(timeSignature.id)
        if (e.button === 2) {
          setRightClickTick(getQuantizedTick(e.nativeEvent.offsetX + (playback ? playback.read().left - scrollLeft : 0)))
          onContextMenu(e)
        }
      }
    },
    [
      selectTimeSignature,
      setTimeSignatureDialogState,
      getQuantizedTick,
      onContextMenu,
      playback,
      scrollLeft,
    ],
  )

  const onClickRuler: React.MouseEventHandler<HTMLCanvasElement> = useCallback(
    (e) => {
      const offset = e.nativeEvent.offsetX + (playback ? playback.read().left - scrollLeft : 0)
      const quantizedTick = getQuantizedTick(offset)
      if (e.nativeEvent.ctrlKey) {
        setLoopBegin(quantizedTick)
      } else if (e.nativeEvent.altKey) {
        setLoopEnd(quantizedTick)
      } else {
        setPosition(getTick(offset))
      }
    },
    [getQuantizedTick, getTick, setLoopBegin, setLoopEnd, setPosition, playback, scrollLeft],
  )

  const onMouseDown: React.MouseEventHandler<HTMLCanvasElement> = useCallback(
    (e) => {
      const offset = e.nativeEvent.offsetX + (playback ? playback.read().left - scrollLeft : 0)
      const timeSignature = timeSignatureHitTest(offset)

      if (timeSignature !== undefined) {
        onClickTimeSignature(timeSignature.event, e)
        onClickRuler(e)
      } else {
        if (e.button == 2) {
          setRightClickTick(getQuantizedTick(offset))
          onContextMenu(e)
        } else {
          clearSelectedTimeSignature()
          onClickRuler(e)
        }
      }

      _onMouseDown?.(e)
    },
    [
      getQuantizedTick,
      timeSignatureHitTest,
      clearSelectedTimeSignature,
      onClickTimeSignature,
      onClickRuler,
      setRightClickTick,
      onContextMenu,
      _onMouseDown,
      playback,
      scrollLeft,
    ],
  )

  const draw = useCallback(
    (ctx: CanvasRenderingContext2D) => {
      ctx.clearRect(0, 0, width, height)
      ctx.save()
      ctx.translate(-(playback ? playback.read().left : scrollLeft) + 0.5, 0)
      if (!playback) drawRuler(ctx, height, rulerBeats, theme)
      if (loop !== null) {
        drawLoopPoints(ctx, loop, height, transform, theme)
      }
      if (!playback) drawTimeSignatures(ctx, height, timeSignatures, theme)
      ctx.restore()
    },
    [
      width,
      height,
      transform,
      scrollLeft,
      rulerBeats,
      timeSignatures,
      loop,
      theme,
      playback,
    ],
  )

  useLayoutEffect(() => playback?.subscribe(() => canvasRef.current?.drawCanvas()), [playback, draw])

  const closeOpenTimeSignatureDialog = useCallback(() => {
    setTimeSignatureDialogState(null)
  }, [])

  const okTimeSignatureDialog = useCallback(
    ({ numerator, denominator }: TimeSignatureDialogState) =>
      updateTimeSignature(numerator, denominator),
    [updateTimeSignature],
  )

  return (
    <div style={{ position: "relative", width, height }}>
      {playback && <PlaybackRuler selected={selectedTimeSignatureEventIds} />}
      <DrawCanvas
        ref={canvasRef}
        draw={draw}
        width={width}
        height={height}
        onMouseDown={onMouseDown}
        onPointerDown={(e) => {
          if (e.button === 0) {
            dragScrollLeft.current = playback ? playback.read().left : scrollLeft
            e.currentTarget.setPointerCapture(e.pointerId)
          }
        }}
        onPointerMove={(e) => {
          if (!e.currentTarget.hasPointerCapture(e.pointerId) || !(e.buttons & 1)) return
          const x = e.clientX - e.currentTarget.getBoundingClientRect().left
          const tick = Math.max(startTick, transform.getTick(x + dragScrollLeft.current))
          if (e.ctrlKey) setLoopBegin(tick)
          else if (e.altKey) setLoopEnd(tick)
          else setPosition(tick)
        }}
        onPointerUp={(e) => {
          if (e.currentTarget.hasPointerCapture(e.pointerId)) e.currentTarget.releasePointerCapture(e.pointerId)
        }}
        onContextMenu={(e) => e.preventDefault()}
        style={{ ...style, touchAction: "none", cursor: "ew-resize" }}
        className={className}
      />
      <RulerContextMenu {...menuProps} tick={rightClickTick} />
      <TimeSignatureDialog
        open={timeSignatureDialogState != null}
        initialNumerator={timeSignatureDialogState?.numerator}
        initialDenominator={timeSignatureDialogState?.denominator}
        onClose={closeOpenTimeSignatureDialog}
        onClickOK={okTimeSignatureDialog}
      />
    </div>
  )
}

export default PianoRuler
