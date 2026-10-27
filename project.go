package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
)

const maxSheetBytes = 1 << 20
const maxAssetBytes = 256 << 20
const maxDecodedBytes = 512 << 20

type Document struct {
	Song                        Song
	Meta                        SheetMeta
	Notation                    string
	Generated, OutOfSync, Dirty bool
	Path                        string
}
type nativeProject struct {
	SheetJSON
	Tracks []TrackSettings `json:"tracks,omitempty"`
}
type SampleSpec struct {
	File    string `json:"file"`
	RootKey int    `json:"rootKey"`
}
type SamplePack struct {
	Version int          `json:"version"`
	Name    string       `json:"name"`
	Samples []SampleSpec `json:"samples"`
}

func boundedRead(path string, limit int64) ([]byte, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, limit+1))
	if e == nil && int64(len(b)) > limit {
		e = fmt.Errorf("文件超过 %d MiB 限制", max(1, limit>>20))
	}
	return b, e
}
func openDocument(path string) (Document, error) {
	b, e := boundedRead(path, maxSheetBytes)
	if e != nil {
		return Document{}, e
	}
	d := Document{Path: path}
	var midi []byte
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mid", ".midi":
		midi = b
	case ".json":
		var p nativeProject
		if e = json.Unmarshal(b, &p); e != nil {
			return d, e
		}
		if p.Version > 3 {
			return d, fmt.Errorf("不支持项目版本 %d", p.Version)
		}
		if p.Notation == "" && p.MIDI == "" {
			return d, fmt.Errorf("项目没有 MIDI 或 DoMiSo 文本")
		}
		d.Meta, d.Notation, d.OutOfSync = p.Meta, p.Notation, p.NotationOutOfSync
		if p.MIDI != "" {
			midi, e = base64.StdEncoding.DecodeString(p.MIDI)
		} else {
			d.Generated = true
			midi, e = (&AppService{}).ParseTextToMIDI(d.Notation)
		}
		if e != nil {
			return d, e
		}
		d.Song, e = readSong(midi)
		if e != nil {
			return d, e
		}
		if d.OutOfSync && p.MIDI == "" {
			return d, fmt.Errorf("参考文本需要 MIDI")
		}
		for i, st := range p.Tracks {
			if i >= len(d.Song.Tracks) {
				return d, fmt.Errorf("音轨设置数量不匹配")
			}
			if e = validateSettings(st); e != nil {
				return d, e
			}
			if st.Instrument.File != "" && !filepath.IsAbs(st.Instrument.File) {
				st.Instrument.File = filepath.Join(filepath.Dir(path), filepath.FromSlash(st.Instrument.File))
			}
			d.Song.Tracks[i].Settings = st
		}
		return d, nil
	default:
		r, err := (&AppService{}).ImportSheet(path)
		if err != nil {
			return d, err
		}
		d.Notation, d.Meta, d.Generated = r.Content, r.Meta, true
		midi, e = (&AppService{}).ParseTextToMIDI(d.Notation)
	}
	if e != nil {
		return d, e
	}
	d.Song, e = readSong(midi)
	return d, e
}
func validateSettings(st TrackSettings) error {
	if math.IsNaN(st.Volume) || math.IsInf(st.Volume, 0) || st.Volume < 0 || st.Volume > 1 {
		return fmt.Errorf("无效音量")
	}
	switch st.Instrument.Kind {
	case "", "builtin", "soundfont", "sample", "pack":
	default:
		return fmt.Errorf("不支持的音源类型")
	}
	if st.Instrument.RootKey < 0 || st.Instrument.RootKey > 127 {
		return fmt.Errorf("无效采样音高")
	}
	if p := st.Instrument.Preset; p != nil && (p.Bank < 0 || p.Bank > 65535 || p.Program < 0 || p.Program > 127) {
		return fmt.Errorf("无效 SoundFont bank/program")
	}
	return nil
}
func atomicWrite(path string, b []byte) error {
	f, e := os.CreateTemp(filepath.Dir(path), ".domiso-*")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if _, e = f.Write(b); e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	return os.Rename(name, path)
}
func (d *Document) Save(path string) error {
	if strings.ToLower(filepath.Ext(path)) != ".json" {
		return fmt.Errorf("项目请保存为 JSON；原文和 MIDI 使用导出")
	}
	p := nativeProject{SheetJSON: SheetJSON{Version: 3, Meta: d.Meta, Notation: d.Notation, NotationOutOfSync: d.OutOfSync}}
	if !d.Generated || d.OutOfSync || d.Notation == "" {
		b, e := d.Song.MIDI()
		if e != nil {
			return e
		}
		p.MIDI = base64.StdEncoding.EncodeToString(b)
	}
	assetDir := strings.TrimSuffix(path, filepath.Ext(path)) + ".assets"
	for _, t := range d.Song.Tracks {
		st := t.Settings
		if e := validateSettings(st); e != nil {
			return e
		}
		if st.Instrument.File != "" {
			file, e := copyInstrument(st.Instrument, assetDir)
			if e != nil {
				return e
			}
			rel, e := filepath.Rel(filepath.Dir(path), file)
			if e != nil {
				return e
			}
			st.Instrument.File = filepath.ToSlash(rel)
		}
		p.Tracks = append(p.Tracks, st)
	}
	b, e := json.MarshalIndent(p, "", "  ")
	if e != nil {
		return e
	}
	if len(b) > maxSheetBytes {
		return fmt.Errorf("项目超过 1 MiB 限制")
	}
	if e = atomicWrite(path, b); e != nil {
		return e
	}
	for i, st := range p.Tracks {
		if st.Instrument.File != "" {
			d.Song.Tracks[i].Settings.Instrument.File = filepath.Join(filepath.Dir(path), filepath.FromSlash(st.Instrument.File))
		}
	}
	d.Path = path
	d.Dirty = false
	return nil
}
func copyAsset(src, dir string) (string, error) {
	b, e := boundedRead(src, maxAssetBytes)
	if e != nil {
		return "", e
	}
	sum := sha256.Sum256(b)
	name := fmt.Sprintf("%x%s", sum[:16], strings.ToLower(filepath.Ext(src)))
	if e = os.MkdirAll(dir, 0755); e != nil {
		return "", e
	}
	dest := filepath.Join(dir, name)
	if _, e = os.Stat(dest); e == nil {
		existing, err := boundedRead(dest, maxAssetBytes)
		if err != nil {
			return "", err
		}
		if sha256.Sum256(existing) != sum {
			return "", fmt.Errorf("资源校验失败: %s", dest)
		}
		return dest, nil
	}
	return dest, atomicWrite(dest, b)
}
func readPack(file string) (SamplePack, error) {
	var p SamplePack
	b, e := boundedRead(file, maxSheetBytes)
	if e != nil {
		return p, e
	}
	if e = json.Unmarshal(b, &p); e != nil {
		return p, e
	}
	if p.Version != 1 || len(p.Samples) == 0 || len(p.Samples) > 128 {
		return p, fmt.Errorf("采样包需要 version=1 和 1–128 个采样")
	}
	seen := map[int]bool{}
	for i := range p.Samples {
		s := &p.Samples[i]
		if s.RootKey < 0 || s.RootKey > 127 || seen[s.RootKey] {
			return p, fmt.Errorf("采样原始音高重复或越界")
		}
		seen[s.RootKey] = true
		if _, e = packPath(file, s.File); e != nil {
			return p, e
		}
	}
	return p, nil
}
func packPath(pack, file string) (string, error) {
	if file == "" || filepath.IsAbs(file) || strings.Contains(file, "\\") || strings.Contains(file, ":") {
		return "", fmt.Errorf("采样路径必须为包内相对路径")
	}
	base, e := filepath.EvalSymlinks(filepath.Dir(pack))
	if e != nil {
		return "", e
	}
	p, e := filepath.EvalSymlinks(filepath.Join(base, filepath.FromSlash(file)))
	if e != nil {
		return "", e
	}
	rel, e := filepath.Rel(base, p)
	if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("采样路径超出包目录")
	}
	return p, nil
}
func copyInstrument(in Instrument, dir string) (string, error) {
	if in.Kind != "pack" {
		return copyAsset(in.File, dir)
	}
	p, e := readPack(in.File)
	if e != nil {
		return "", e
	}
	for i := range p.Samples {
		src, e := packPath(in.File, p.Samples[i].File)
		if e != nil {
			return "", e
		}
		dest, e := copyAsset(src, dir)
		if e != nil {
			return "", e
		}
		p.Samples[i].File = filepath.Base(dest)
	}
	b, e := json.MarshalIndent(p, "", "  ")
	if e != nil {
		return "", e
	}
	sum := sha256.Sum256(b)
	dest := filepath.Join(dir, fmt.Sprintf("%x.json", sum[:16]))
	return dest, atomicWrite(dest, b)
}
