import { describe, expect, it } from "vitest"
import { Measure } from "@signal-app/core"
import { BeatWithX } from "./BeatWithX"
import { TickTransform } from "../transform/TickTransform"

describe("pre-roll measures", () => {
  it.each([[4, 4, -3840], [3, 4, -2880], [6, 8, -2880]])("uses the initial %i/%i signature", (numerator, denominator, start) => {
    const measures = [{ tick: 0, measure: 0, numerator, denominator }]
    expect(Measure.getPreRollStart(measures, 480)).toBe(start)
    const beats = BeatWithX.createInRange(measures, new TickTransform(1), 480, start - 1000, -start + 1500)
    expect(beats.filter((b) => b.beat === 0).map((b) => b.measure + 1)).toEqual([-1, 0, 1])
    expect(beats[0].tick).toBe(start)
    expect(beats.filter((b) => b.tick >= 0)[0].tick).toBe(0)
  })
})
