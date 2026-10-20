import { noteNameWithOctString } from "./noteNumberString"
import { pitchToKey, type GameInstrumentProfile } from "../game-mapping"

export type NoteLabelMode = "name" | "number" | "key"
const semitones: Record<string, number> = { C: 0, D: 2, E: 4, F: 5, G: 7, A: 9, B: 11 }

// Match the DoMiSo parser: C / C5 is MIDI 60; metadata before ===== is ignored.
export function notationTonic(text: string): number {
  const separator = /^\s*={5,}\s*$/m.exec(text)
  const body = separator ? text.slice(separator.index + separator[0].length) : text
  const match = /\b1\s*=\s*([a-g])(\d+)?([#b])?/i.exec(body)
  if (!match) return 60
  const value = 12 * Number(match[2] ?? 5) + semitones[match[1].toUpperCase()] + (match[3] === "#" ? 1 : match[3]?.toLowerCase() === "b" ? -1 : 0)
  return value >= 0 && value <= 127 ? value : 60
}

export function numberedNote(note: number, tonic: number) {
  const delta = note - tonic
  return {
    text: ["1", "#1", "2", "#2", "3", "4", "#4", "5", "#5", "6", "#6", "7"][((delta % 12) + 12) % 12],
    octave: Math.floor(delta / 12),
  }
}

export function noteLabel(note: number, mode: NoteLabelMode, tonic: number, profile: GameInstrumentProfile, state: string) {
  if (mode === "number") return numberedNote(note, tonic)
  if (mode === "name") return { text: noteNameWithOctString(note), octave: 0 }
  const mapped = pitchToKey(profile, state, note)
  return { text: mapped.type === "key" ? mapped.keyCode.replace(/^(Key|Digit)/, "") + (mapped.state !== state ? `·${mapped.state}` : "") : "—", octave: 0 }
}
