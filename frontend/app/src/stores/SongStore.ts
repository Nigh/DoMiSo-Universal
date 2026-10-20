import { notationTonic } from "../helpers/pianoRollLabels"
import { Song, emptySong, songFromMidi, songToMidi } from "@signal-app/core"
import { action, makeObservable, observable } from "mobx"

export class SongStore {
  song: Song = emptySong()
  labelTonic = 60
  notation = ""
  generatedFromNotation = false
  notationOutOfSync = false

  constructor() {
    makeObservable(this, {
      song: observable.ref,
      labelTonic: observable,
      setLabelTonic: action,
      notation: observable,
      generatedFromNotation: observable,
      notationOutOfSync: observable,
      loadMIDI: action,
      markMIDIEdited: action,
      markSaved: action,
      replaceSong: action,
    })
  }

  setLabelTonic(value: number) {
    this.labelTonic = Math.max(0, Math.min(127, value))
  }

  serialize() {
    return this.song.serialize()
  }

  loadMIDI(
    bytes: Uint8Array,
    options: { notation?: string; generatedFromNotation?: boolean; notationOutOfSync?: boolean } = {},
  ) {
    this.song = songFromMidi(bytes)
    this.song.isSaved = true
    this.notation = options.notation ?? ""
    this.labelTonic = notationTonic(this.notation)
    this.generatedFromNotation = options.generatedFromNotation ?? false
    this.notationOutOfSync = options.notationOutOfSync ?? false
  }

  markMIDIEdited() {
    if (!this.song.isSaved) {
      this.notationOutOfSync ||= this.notation.length > 0
    }
  }

  markSaved() {
    this.song.isSaved = true
  }

  get midiBytes() {
    return new Uint8Array(songToMidi(this.song))
  }

  replaceSong(song: Song) {
    this.song = song
    this.labelTonic = 60
    this.notation = ""
    this.generatedFromNotation = false
    this.notationOutOfSync = false
  }
}
