import { LoopSetting } from "@signal-app/player"
import { TickTransform } from "../../entities/transform/TickTransform"
import { RulerBeat, RulerTimeSignature } from "../../hooks/useRuler"
import { Theme } from "../../theme/Theme"
const textPadding = 2

export function drawRuler(
  ctx: CanvasRenderingContext2D,
  height: number,
  beats: RulerBeat[],
  theme: Theme,
) {
  ctx.strokeStyle = theme.secondaryTextColor
  ctx.lineWidth = 1
  ctx.beginPath()

  beats.forEach(({ beat, x, label }) => {
    if (beat === 0) {
      ctx.moveTo(x, height / 2)
      ctx.lineTo(x, height)
    } else {
      ctx.moveTo(x, height * 0.8)
      ctx.lineTo(x, height)
    }
    if (label) {
      ctx.textBaseline = "top"
      ctx.font = `12px ${theme.canvasFont}`
      ctx.fillStyle = theme.secondaryTextColor
      ctx.fillText(label, x + textPadding, textPadding)
    }
  })

  ctx.closePath()
  ctx.stroke()
}

export function drawLoopPoints(
  ctx: CanvasRenderingContext2D,
  loop: LoopSetting,
  height: number,
  transform: TickTransform,
  theme: Theme,
) {
  const flagSize = 8
  ctx.lineWidth = 1
  ctx.fillStyle = loop.enabled ? theme.themeColor : theme.secondaryTextColor
  ctx.strokeStyle = loop.enabled ? theme.themeColor : theme.secondaryTextColor
  ctx.beginPath()

  const beginX = transform.getX(loop.begin)
  const endX = transform.getX(loop.end)

  if (loop.begin !== null) {
    const x = beginX
    ctx.moveTo(x, 0)
    ctx.lineTo(x, height)

    ctx.moveTo(x, 0)
    ctx.lineTo(x + flagSize, 0)
    ctx.lineTo(x, flagSize)
  }

  if (loop.end !== null) {
    const x = endX
    ctx.moveTo(x, 0)
    ctx.lineTo(x, height)

    ctx.moveTo(x, 0)
    ctx.lineTo(x - flagSize, 0)
    ctx.lineTo(x, flagSize)
  }

  ctx.closePath()
  ctx.fill()
  ctx.stroke()
}

function drawFlag(
  ctx: CanvasRenderingContext2D,
  x: number,
  y: number,
  width: number,
  height: number,
  flagSize: number,
) {
  ctx.beginPath()
  ctx.moveTo(x, y)
  ctx.lineTo(x + width + flagSize, y)
  ctx.lineTo(x + width, y + height)
  ctx.lineTo(x, y + height)
  ctx.lineTo(x, y)
  ctx.closePath()
  ctx.fill()
}

export function drawTimeSignatures(
  ctx: CanvasRenderingContext2D,
  height: number,
  events: RulerTimeSignature[],
  theme: Theme,
) {
  ctx.textBaseline = "bottom"
  ctx.font = `11px ${theme.canvasFont}`
  events.forEach((e) => {
    const size = ctx.measureText(e.label)
    const textHeight =
      size.actualBoundingBoxAscent + size.actualBoundingBoxDescent
    ctx.fillStyle = e.isSelected
      ? theme.themeColor
      : theme.secondaryBackgroundColor
    const flagHeight = textHeight + textPadding * 4
    drawFlag(
      ctx,
      e.x,
      height - flagHeight,
      size.width + textPadding * 2,
      flagHeight,
      textHeight,
    )
    ctx.fillStyle = e.isSelected ? theme.onSurfaceColor : theme.textColor
    ctx.fillText(e.label, e.x + textPadding, height - textPadding)
  })
}
