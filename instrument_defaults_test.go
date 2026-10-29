package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestTrackDefaultVolume(t *testing.T) {
	if got := newTrack("New").Settings.Volume; got != 1 {
		t.Fatalf("new track volume = %g, want 1", got)
	}
	e := newEditor()
	if got := e.doc.Song.Tracks[0].Settings.Volume; got != 1 {
		t.Fatalf("new document volume = %g, want 1", got)
	}
	tester := ui.NewTester(e.sidebarTracks, 280, 400)
	if err := tester.Click("新增"); err != nil {
		t.Fatal(err)
	}
	if got := e.doc.Song.Tracks[e.track].Settings.Volume; got != 1 {
		t.Fatalf("added track volume = %g, want 1", got)
	}
}

func TestImportedDefaultAndSavedVolumes(t *testing.T) {
	song := testSong()
	song.Tracks = append(song.Tracks, newTrack("Metadata"))
	midi, err := song.MIDI()
	if err != nil {
		t.Fatal(err)
	}
	encoded := base64.StdEncoding.EncodeToString(midi)
	project := func(version int, tracks string) []byte {
		data, err := json.Marshal(encoded)
		if err != nil {
			t.Fatal(err)
		}
		return []byte(fmt.Sprintf(`{"version":%d,"midi":%s%s}`, version, data, tracks))
	}
	for _, tc := range []struct {
		name string
		data []byte
		want float64
	}{
		{"import.mid", midi, 1},
		{"notation.txt", []byte("1=C 4/4 120\n1 2 3"), 1},
		{"legacy-v1.json", project(1, ""), 1},
		{"legacy-v2.json", project(2, ""), 1},
		{"missing-settings.json", project(3, ""), 1},
		{"missing-volume.json", project(3, `,"tracks":[{"instrument":{},"mute":true}]`), 1},
		{"saved-zero.json", project(3, `,"tracks":[{"instrument":{},"volume":0}]`), 0},
		{"saved-volume.json", project(3, `,"tracks":[{"instrument":{},"volume":0.42}]`), 0.42},
		{"generated.json", []byte(`{"version":3,"notation":"1=C 4/4 120\n1 2 3"}`), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), tc.name)
			if err := os.WriteFile(path, tc.data, 0600); err != nil {
				t.Fatal(err)
			}
			d, err := openDocument(path)
			if err != nil {
				t.Fatal(err)
			}
			for i, track := range d.Song.Tracks {
				want := 1.0
				if i == 0 {
					want = tc.want
				}
				if track.Settings.Volume != want {
					t.Fatalf("track %d volume = %g, want %g", i, track.Settings.Volume, want)
				}
			}
			saved := filepath.Join(filepath.Dir(path), "saved.json")
			if err := d.Save(saved); err != nil {
				t.Fatal(err)
			}
			reopened, err := openDocument(saved)
			if err != nil {
				t.Fatal(err)
			}
			if reopened.Song.Tracks[0].Settings.Volume != tc.want {
				t.Fatal("save/reopen changed volume")
			}
		})
	}
}
