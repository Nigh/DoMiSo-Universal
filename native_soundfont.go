package main

import "github.com/sinshu/go-meltysynth/meltysynth"

func soundFontHasPreset(sf *meltysynth.SoundFont, preset SoundFontPreset) bool {
	for _, p := range sf.Presets {
		if int(p.BankNumber) == preset.Bank && int(p.PatchNumber) == preset.Program {
			return true
		}
	}
	return false
}

func applySoundFontPreset(in *loadedInstrument) {
	if in.Synth == nil || in.Config.Preset == nil {
		return
	}
	p := in.Config.Preset
	for channel := int32(0); channel < 16; channel++ {
		bank := int32(p.Bank)
		// MeltySynth adds 128 on its percussion channel. Compensate so every
		// source channel uses the exact SF2 bank selected by the track.
		if channel == 9 {
			bank -= 128
		}
		in.Synth.ProcessMidiMessage(channel, 0xb0, 0, bank)
		in.Synth.ProcessMidiMessage(channel, 0xc0, int32(p.Program), 0)
	}
}

// SoundFont metadata is immutable after loading. Returning it does not copy
// sample data or rebuild the choices on every GUI frame.
func (a *AudioEngine) soundFontPresets(track int) (*meltysynth.SoundFont, []string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if track < 0 || track >= len(a.instruments) || a.instruments[track].Synth == nil {
		return nil, nil
	}
	in := a.instruments[track]
	return in.Synth.SoundFont, in.PresetNames
}

func (e *Editor) setSoundFontPreset(preset *SoundFontPreset) {
	if e.busy {
		return
	}
	e.releaseKeys()
	e.checkpoint()
	e.doc.Song.Tracks[e.track].Settings.Instrument.Preset = preset
	e.changed(false)
	e.audio.Update(e.doc.Song)
}
