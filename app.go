package main

import (
	"bytes"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	domisoparser "github.com/Nigh/domiso-parser"
)

//go:embed all:free_sheets
var sampleSheets embed.FS

var sheetNameReplacements = map[rune]string{
	'\u02BC': "'",
}

func unescapeSheetName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if replacement, ok := sheetNameReplacements[r]; ok {
			b.WriteString(replacement)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

type AppService struct{}

type SheetInfo struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type SheetMeta struct {
	Title       string `json:"title"`
	Composer    string `json:"composer"`
	Arranger    string `json:"arranger"`
	Description string `json:"description"`
	Tempo       string `json:"tempo"`
	TimeSig     string `json:"timeSig"`
	Key         string `json:"key"`
}

type SheetJSON struct {
	Version           int       `json:"version"`
	Meta              SheetMeta `json:"meta"`
	Notation          string    `json:"notation,omitempty"`
	MIDI              string    `json:"midi,omitempty"`
	NotationOutOfSync bool      `json:"notationOutOfSync,omitempty"`
}

type SheetStats struct {
	NoteCount    int     `json:"noteCount"`
	MeasureCount float64 `json:"measureCount"`
	DurationSec  float64 `json:"durationSec"`
	BPM          int     `json:"bpm"`
	TicksPerBeat int     `json:"ticksPerBeat"`
}

type ImportResult struct {
	Content           string    `json:"content"`
	Meta              SheetMeta `json:"meta"`
	Format            string    `json:"format"`
	MIDI              string    `json:"midi,omitempty"`
	NotationOutOfSync bool      `json:"notationOutOfSync,omitempty"`
}

func (a *AppService) ParseTextToMIDI(text string) ([]byte, error) {
	score, err := domisoparser.Parse(text)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}
	var buf bytes.Buffer
	err = domisoparser.WriteMIDI(&buf, score)
	if err != nil {
		return nil, fmt.Errorf("MIDI write error: %w", err)
	}
	return buf.Bytes(), nil
}

func (a *AppService) ParseSheetStats(text string) (SheetStats, error) {
	score, err := domisoparser.Parse(text)
	if err != nil {
		return SheetStats{}, fmt.Errorf("parse error: %w", err)
	}

	noteCount := 0
	maxTick := 0
	for _, track := range score.Tracks {
		noteCount += len(track.Notes)
		for _, note := range track.Notes {
			end := note.Tick + note.Duration
			if end > maxTick {
				maxTick = end
			}
		}
	}

	bpm := score.BPM
	if bpm <= 0 {
		bpm = 80
	}
	tpb := score.TicksPerBeat
	if tpb <= 0 {
		tpb = 480
	}

	beatsPerMeasure := 4.0
	ticksPerMeasure := float64(tpb) * beatsPerMeasure
	measureCount := float64(maxTick) / ticksPerMeasure

	totalBeats := float64(maxTick) / float64(tpb)
	durationSec := totalBeats / (float64(bpm) / 60.0)

	return SheetStats{
		NoteCount:    noteCount,
		MeasureCount: measureCount,
		DurationSec:  durationSec,
		BPM:          bpm,
		TicksPerBeat: tpb,
	}, nil
}

func (a *AppService) ImportSheet(filePath string) (ImportResult, error) {
	data, err := boundedRead(filePath, maxSheetBytes)
	if err != nil {
		return ImportResult{}, fmt.Errorf("failed to read file: %w", err)
	}

	const maxFileSize = 1024 * 1024
	if len(data) > maxFileSize {
		return ImportResult{}, fmt.Errorf("file too large: %d bytes (max %d)", len(data), maxFileSize)
	}

	ext := strings.ToLower(filepath.Ext(filePath))

	if ext == ".dms" || domisoparser.IsDMSFile(data) {
		plaintext, err := domisoparser.DecryptDMS(data)
		if err != nil {
			return ImportResult{}, fmt.Errorf("failed to decrypt DMS file: %w", err)
		}
		return ImportResult{
			Content: plaintext,
			Meta:    SheetMeta{},
			Format:  "dms",
		}, nil
	}

	if ext == ".json" {
		var sheet SheetJSON
		if err := json.Unmarshal(data, &sheet); err != nil {
			return ImportResult{}, fmt.Errorf("invalid JSON format: %w", err)
		}
		if sheet.Notation == "" {
			if sheet.MIDI == "" {
				return ImportResult{}, fmt.Errorf("JSON sheet has neither MIDI nor notation content")
			}
		}
		if sheet.MIDI != "" {
			midi, err := base64.StdEncoding.DecodeString(sheet.MIDI)
			if err != nil || len(midi) < 4 || string(midi[:4]) != "MThd" {
				return ImportResult{}, fmt.Errorf("JSON sheet contains invalid MIDI data")
			}
		}
		if sheet.NotationOutOfSync && sheet.MIDI == "" {
			return ImportResult{}, fmt.Errorf("out-of-sync notation requires MIDI data")
		}
		return ImportResult{
			Content:           sheet.Notation,
			Meta:              sheet.Meta,
			Format:            "json",
			MIDI:              sheet.MIDI,
			NotationOutOfSync: sheet.NotationOutOfSync,
		}, nil
	}

	content := string(data)
	if len(strings.TrimSpace(content)) == 0 {
		return ImportResult{}, fmt.Errorf("file is empty")
	}

	return ImportResult{
		Content: content,
		Meta:    SheetMeta{},
		Format:  "txt",
	}, nil
}

func (a *AppService) ExportSheet(filePath string, meta SheetMeta, notation, midi string, notationOutOfSync bool) error {
	ext := strings.ToLower(filepath.Ext(filePath))
	if notationOutOfSync && midi == "" {
		return fmt.Errorf("out-of-sync notation requires MIDI data")
	}
	if midi != "" {
		data, err := base64.StdEncoding.DecodeString(midi)
		if err != nil || len(data) < 4 || string(data[:4]) != "MThd" {
			return fmt.Errorf("invalid MIDI data")
		}
	}

	if ext == ".json" {
		sheet := SheetJSON{
			Version:           2,
			Meta:              meta,
			Notation:          notation,
			MIDI:              midi,
			NotationOutOfSync: notationOutOfSync,
		}
		data, err := json.MarshalIndent(sheet, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to encode JSON: %w", err)
		}
		return os.WriteFile(filePath, data, 0644)
	}
	if midi != "" {
		return fmt.Errorf("edited MIDI must be saved as a JSON project")
	}

	var buf strings.Builder
	if meta.Title != "" {
		buf.WriteString(fmt.Sprintf("标题: %s\n", meta.Title))
	}
	if meta.Composer != "" {
		buf.WriteString(fmt.Sprintf("记谱: %s\n", meta.Composer))
	}
	if meta.Arranger != "" {
		buf.WriteString(fmt.Sprintf("编曲: %s\n", meta.Arranger))
	}
	if meta.Description != "" {
		buf.WriteString(meta.Description + "\n")
	}
	buf.WriteString("\n==================================\n\n")
	buf.WriteString(notation)
	buf.WriteString("\n")

	return os.WriteFile(filePath, []byte(buf.String()), 0644)
}

func (a *AppService) ListFreeSheets() ([]SheetInfo, error) {
	entries, err := fs.ReadDir(sampleSheets, "free_sheets")
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded free_sheets: %w", err)
	}

	var sheets []SheetInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		lower := strings.ToLower(name)
		if strings.HasSuffix(lower, ".txt") || strings.HasSuffix(lower, ".json") || strings.HasSuffix(lower, ".dms") {
			sheets = append(sheets, SheetInfo{
				Name: unescapeSheetName(name),
				Path: path.Join("free_sheets", name),
			})
		}
	}

	sort.Slice(sheets, func(i, j int) bool {
		return sheets[i].Name < sheets[j].Name
	})

	return sheets, nil
}

func (a *AppService) LoadSheet(filename string) (string, error) {
	safeName := path.Base(filename)
	embedPath := path.Join("free_sheets", safeName)

	data, err := sampleSheets.ReadFile(embedPath)
	if err != nil {
		return "", fmt.Errorf("failed to read embedded sheet: %w", err)
	}
	return string(data), nil
}

func (a *AppService) FormatDuration(seconds float64) string {
	d := time.Duration(seconds * float64(time.Second))
	mins := int(d.Minutes())
	secs := int(d.Seconds()) % 60
	return fmt.Sprintf("%d:%02d", mins, secs)
}

func (a *AppService) ImportGameProfile(filePath string) (string, error) {
	data, err := boundedRead(filePath, maxSheetBytes)
	if err != nil {
		return "", fmt.Errorf("failed to read profile: %w", err)
	}
	if len(data) > 1024*1024 {
		return "", fmt.Errorf("profile is larger than 1MB")
	}
	if !json.Valid(data) {
		return "", fmt.Errorf("invalid profile JSON")
	}
	return string(data), nil
}

func (a *AppService) ExportGameProfile(filePath, profileJSON string) error {
	if len(profileJSON) > 1024*1024 || !json.Valid([]byte(profileJSON)) {
		return fmt.Errorf("invalid profile JSON")
	}
	return os.WriteFile(filePath, []byte(profileJSON), 0644)
}
