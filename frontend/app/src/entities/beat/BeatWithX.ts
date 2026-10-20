import { Beat, Measure, Range } from "@signal-app/core"
import { TickTransform } from "../transform/TickTransform"

export type BeatWithX = Beat & {
  readonly x: number
}

export namespace BeatWithX {
  export const createInRange = (
    allMeasures: Measure[],
    transform: TickTransform,
    timebase: number,
    scrollLeft: number,
    width: number,
  ): BeatWithX[] => {
    const first = allMeasures[0] ?? { tick: 0, measure: 0, numerator: 4, denominator: 4 }
    const preRoll = { ...first, tick: Measure.getPreRollStart(allMeasures, timebase), measure: -2 }
    return Beat.createInRange(
      [preRoll, ...allMeasures],
      timebase,
      Range.fromLength(transform.getTick(scrollLeft), transform.getTick(width)),
    ).map((b) => ({
      ...b,
      x: Math.round(transform.getX(b.tick)),
    }))
  }
}
