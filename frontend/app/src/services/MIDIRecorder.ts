import {
  NoteEvent,
  TrackEvent,
  TrackId,
  UNASSIGNED_TRACK_ID,
} from "@signal-app/core"
import { Player } from "@signal-app/player"
import { deserializeSingleEvent, Stream } from "midifile-ts"
import { makeObservable, observable, observe } from "mobx"
import { MIDIDeviceStore } from "../stores/MIDIDeviceStore"
import { SongStore } from "../stores/SongStore"
import { MIDIInputEvent } from "./MIDIInput"

export class MIDIRecorder {
  private recordedNotes: { [key: TrackId]: NoteEvent[] } = {}
  isRecording: boolean = false
  trackId: TrackId = UNASSIGNED_TRACK_ID

  constructor(
    private readonly songStore: SongStore,
    private readonly player: Player,
    private readonly midiDeviceStore: MIDIDeviceStore,
  ) {
    makeObservable(this, {
      isRecording: observable,
      trackId: observable,
    })

    // extend duration while key press
    observe(player, "position", (change) => {
      if (!this.isRecording) {
        return
      }

      const tick = Math.floor(change.object.get())

      Object.entries(this.recordedNotes).forEach(([trackId, notes]) => {
        const track = this.songStore.song.getTrack(parseInt(trackId) as TrackId)
        if (track === undefined) {
          return
        }
        notes.forEach((n) => {
          track.updateEvent<NoteEvent>(n.id, {
            duration: Math.max(0, tick - n.tick),
          })
        })
      })
    })

    observe(this, "isRecording", (change) => {
      this.recordedNotes = {}

      if (!change.newValue) {
        // stop recording
        this.songStore.song.tracks.forEach((track) => {
          const events = track.events
            .filter((e) => e.isRecording === true)
            .map<Partial<TrackEvent>>((e) => ({ ...e, isRecording: false }))
          track.updateEvents(events)
        })
      }
    })
  }

  onMessage(e: MIDIInputEvent) {
    if (!this.isRecording) {
      return
    }

    const stream = new Stream(e.data)
    const message = deserializeSingleEvent(stream)

    if (message.type !== "channel") {
      return
    }

    const tick = Math.floor(this.player.position)

    if (tick < 0) return

    const routing = this.midiDeviceStore.midiInputRouting

    let tracks
    if (routing === "channelRouting") {
      tracks = this.songStore.song.tracks.filter(
        (t) => !t.isConductorTrack && t.channel === message.channel,
      )
    } else {
      // selectedTrack mode
      const track = this.songStore.song.getTrack(this.trackId)
      tracks = track !== undefined ? [track] : []
    }

    switch (message.subtype) {
      case "noteOn": {
        tracks.forEach((track) => {
          const note = track.addEvent<NoteEvent>({
            type: "channel",
            subtype: "note",
            noteNumber: message.noteNumber,
            tick,
            velocity: message.velocity,
            duration: 0,
            isRecording: true,
          })
          if (this.recordedNotes[track.id] === undefined) {
            this.recordedNotes[track.id] = []
          }
          this.recordedNotes[track.id].push(note)
        })
        break
      }
      case "noteOff": {
        tracks.forEach((track) => {
          const recordedNotes = this.recordedNotes[track.id] ?? []

          recordedNotes
            .filter((n) => n.noteNumber === message.noteNumber)
            .forEach((n) => {
              track.updateEvent<NoteEvent>(n.id, {
                duration: Math.max(0, tick - n.tick),
              })
            })

          this.recordedNotes[track.id] = recordedNotes.filter(
            (n) => n.noteNumber !== message.noteNumber,
          )
        })
        break
      }
      default: {
        tracks.forEach((track) => {
          track.addEvent({ ...message, tick, isRecording: true })
        })
        break
      }
    }
  }
}
