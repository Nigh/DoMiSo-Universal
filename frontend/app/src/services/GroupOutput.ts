import { TrackId } from "@signal-app/core"
import { SendableEvent, SynthOutput } from "@signal-app/player"
import { makeObservable, observable } from "mobx"
import { METRONOME_TRACK_ID } from "../player/EventSource"
import { TrackMute } from "../trackMute/TrackMute"

export interface SynthEntry {
  synth: SynthOutput
  isEnabled: boolean
}

// Routing of MIDI events to multiple SynthOutputs and muting of tracks
export class GroupOutput implements SynthOutput {
  private playbackListeners = new Set<SynthOutput["sendEvent"]>()

  onPlayback(listener: SynthOutput["sendEvent"]) {
    this.playbackListeners.add(listener)
    return () => { this.playbackListeners.delete(listener) }
  }

  outputs: SynthEntry[] = []
  isMetronomeEnabled: boolean = false
  trackMute = TrackMute.empty

  constructor(private readonly metronomeOutput: SynthOutput) {
    makeObservable(this, {
      isMetronomeEnabled: observable,
    })
  }

  activate() {
    this.outputs.filter((o) => o.isEnabled).forEach((o) => o.synth.activate())
  }

  private getOutputs(trackId: TrackId | undefined): SynthOutput[] {
    if (trackId === METRONOME_TRACK_ID) {
      return this.isMetronomeEnabled ? [this.metronomeOutput] : []
    } else if (
      trackId !== undefined &&
      !TrackMute.shouldPlayTrack(trackId)(this.trackMute)
    ) {
      return []
    } else {
      return this.outputs.filter((o) => o.isEnabled).map((o) => o.synth)
    }
  }

  sendEvent(
    event: SendableEvent,
    delayTime: number,
    timestampNow: number,
    trackId?: TrackId,
  ): void {
    if (trackId !== METRONOME_TRACK_ID &&
      (trackId === undefined || TrackMute.shouldPlayTrack(trackId)(this.trackMute))) {
      this.playbackListeners.forEach((listener) => listener(event, delayTime, timestampNow, trackId))
    }
    this.getOutputs(trackId).forEach((synth) =>
      synth.sendEvent(event, delayTime, timestampNow, trackId),
    )
  }
}
