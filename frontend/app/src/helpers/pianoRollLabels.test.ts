import { describe, expect, it } from "vitest"
import { genshinWindsongLyre } from "../game-mapping"
import { notationTonic, numberedNote, noteLabel } from "./pianoRollLabels"

describe("piano roll labels", () => {
  it("reads the first score tonic using DoMiSo octaves and ignores metadata", () => {
    expect(notationTonic("Title 1=D\n=====\n1=c\n1 2 3")).toBe(60)
    expect(notationTonic("1=F#")).toBe(66)
    expect(notationTonic("1=B4b")).toBe(58)
    expect(notationTonic("1=C6\n1=D")).toBe(72)
    expect(notationTonic("1 2 3")).toBe(60)
  })
  it("displays chromatic degrees and octaves relative to the chosen tonic", () => {
    expect(numberedNote(62, 62)).toEqual({ text: "1", octave: 0 })
    expect(numberedNote(63, 62)).toEqual({ text: "#1", octave: 0 })
    expect(numberedNote(61, 62)).toEqual({ text: "7", octave: -1 })
    expect(numberedNote(86, 62)).toEqual({ text: "1", octave: 2 })
  })
  it("uses the active profile and marks unmapped notes and state changes", () => {
    expect(noteLabel(60, "name", 60, genshinWindsongLyre, "default").text).toBe("C4")
    expect(noteLabel(60, "key", 60, genshinWindsongLyre, "default").text).toBe("A")
    expect(noteLabel(61, "key", 60, genshinWindsongLyre, "default").text).toBe("—")
    const profile = { ...genshinWindsongLyre, states: [{ id: "other", label: "Other", bindings: { KeyT: 61 } }] }
    expect(noteLabel(61, "key", 60, profile, "default").text).toBe("T·other")
  })
})
