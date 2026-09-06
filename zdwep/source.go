package zdwep

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Source is a ZDoom mod to migrate from. It is either a pk3 (a zip) or a
// directory holding the same layout.
//
// Lump names in DECORATE are written without regard for case or extension: a
// script says #include "actors/weapons/shotgun.txt" and the archive holds
// ACTORS/WEAPONS/Shotgun.TXT. Source resolves that, so the rest of the
// migration can ask for names the way the scripts write them.
//
// A pk3 is read through archive/zip directly rather than through its fs.FS
// view. Real mods ship archives with the same path stored twice - Final Doomer
// has one - and the fs.FS view refuses to open such an archive at all, while
// the archive itself reads fine. The first entry of a repeated name wins.
type Source struct {
	names []string
	// layers are the places a file can come from, in the order they were
	// given. A mod usually needs more than one: its own pk3 for its weapons,
	// and the game's own data for the stock sprites it takes for granted.
	layers []func(real string) (io.ReadCloser, error)
	close  func() error
	// byLower maps a lowercased path to where it is, and byLowerNoExt does the
	// same ignoring the extension, which is how DECORATE refers to lumps like
	// DECORATE or SNDINFO. The first source to provide a name keeps it.
	byLower      map[string]entry
	byLowerNoExt map[string]entry
}

// entry is where one file lives: which source, under what name.
type entry struct {
	layer int
	real  string
}

// OpenSources opens several pk3s or directories as one, searched in order. The
// first to hold a name provides it, so a mod overrides the game data it sits on
// top of.
func OpenSources(locations []string) (*Source, error) {
	if len(locations) == 0 {
		return nil, fmt.Errorf("no source given")
	}
	combined, err := OpenSource(locations[0])
	if err != nil {
		return nil, err
	}
	for _, location := range locations[1:] {
		next, err := OpenSource(location)
		if err != nil {
			combined.Close()
			return nil, err
		}
		combined.layer(next)
	}
	return combined, nil
}

// layer folds another source in underneath this one.
func (s *Source) layer(other *Source) {
	offset := len(s.layers)
	s.layers = append(s.layers, other.layers...)
	previousClose := s.close
	s.close = func() error {
		other.Close()
		return previousClose()
	}
	for _, name := range other.names {
		found, ok := other.byLower[strings.ToLower(name)]
		if !ok {
			continue
		}
		s.noteAt(name, entry{layer: found.layer + offset, real: found.real})
	}
}

// OpenSource opens a pk3 or a directory. The caller closes it.
func OpenSource(location string) (*Source, error) {
	lower := strings.ToLower(location)
	if strings.HasSuffix(lower, ".pk3") || strings.HasSuffix(lower, ".zip") ||
		strings.HasSuffix(lower, ".ipk3") || strings.HasSuffix(lower, ".pk7") {
		return openZip(location)
	}
	if strings.HasSuffix(lower, ".wad") {
		return openWAD(location)
	}
	info, err := os.Stat(location)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, &fs.PathError{Op: "open", Path: location, Err: fs.ErrInvalid}
	}
	return openDir(location)
}

func openZip(location string) (*Source, error) {
	reader, err := zip.OpenReader(location)
	if err != nil {
		return nil, err
	}
	entries := make(map[string]*zip.File, len(reader.File))
	source := newSource(reader.Close)
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		name := path.Clean(strings.ReplaceAll(file.Name, "\\", "/"))
		if _, repeated := entries[name]; repeated {
			continue // an archive that stores one path twice; the first wins
		}
		entries[name] = file
		source.note(name)
	}
	source.layers = []func(string) (io.ReadCloser, error){
		func(real string) (io.ReadCloser, error) {
			file, ok := entries[real]
			if !ok {
				return nil, &fs.PathError{Op: "open", Path: real, Err: fs.ErrNotExist}
			}
			return file.Open()
		},
	}
	return source, nil
}

func openDir(location string) (*Source, error) {
	source := newSource(func() error { return nil })
	fsys := os.DirFS(location)
	err := fs.WalkDir(fsys, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		source.note(name)
		return nil
	})
	if err != nil {
		return nil, err
	}
	source.layers = []func(string) (io.ReadCloser, error){
		func(real string) (io.ReadCloser, error) {
			return os.Open(filepath.Join(location, filepath.FromSlash(real)))
		},
	}
	return source, nil
}

// NewSourceFS wraps an already open filesystem, which is how the tests supply a
// mod without touching the disk.
func NewSourceFS(fsys fs.FS) (*Source, error) {
	source := newSource(func() error { return nil })
	err := fs.WalkDir(fsys, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		source.note(name)
		return nil
	})
	if err != nil {
		return nil, err
	}
	source.layers = []func(string) (io.ReadCloser, error){
		func(real string) (io.ReadCloser, error) {
			return fsys.Open(real)
		},
	}
	return source, nil
}

func newSource(closer func() error) *Source {
	return &Source{
		close:        closer,
		byLower:      make(map[string]entry),
		byLowerNoExt: make(map[string]entry),
	}
}

// note records one file under every name a script might ask for it by.
func (s *Source) note(name string) {
	s.noteAt(name, entry{layer: 0, real: name})
}

func (s *Source) noteAt(name string, where entry) {
	s.names = append(s.names, name)
	lower := strings.ToLower(name)
	if _, taken := s.byLower[lower]; !taken {
		s.byLower[lower] = where
	}
	// Two lumps whose names differ only by extension are a mod's business, not
	// ours; the first one wins and stays put.
	noExt := strings.TrimSuffix(lower, path.Ext(lower))
	if _, taken := s.byLowerNoExt[noExt]; !taken {
		s.byLowerNoExt[noExt] = where
	}
}

func (s *Source) Close() error { return s.close() }

// Names lists every file in the source, in the order it was found.
func (s *Source) Names() []string { return s.names }

// LayerOf is which of the stacked sources a name came from: 0 is the first
// location given to OpenSources, and the ones layered in underneath it count
// up from there. It is how a caller tells a mod's own lump from the base
// game's, which matters because the two are searched in opposite directions -
// see NewAssetIndex.
func (s *Source) LayerOf(name string) int {
	found, _ := s.find(name)
	return found.layer
}

// Resolve turns a name as a script writes it into the real path in the source,
// trying an exact match, then case-insensitive, then case-insensitive ignoring
// the extension.
func (s *Source) Resolve(name string) (string, bool) {
	found, ok := s.find(name)
	return found.real, ok
}

func (s *Source) find(name string) (entry, bool) {
	name = path.Clean(strings.TrimPrefix(strings.ReplaceAll(name, "\\", "/"), "./"))
	lower := strings.ToLower(name)
	if found, ok := s.byLower[lower]; ok {
		return found, true
	}
	if found, ok := s.byLowerNoExt[lower]; ok {
		return found, true
	}
	return entry{}, false
}

// Open reads a file by a name written the way the scripts write it.
func (s *Source) Open(name string) (io.ReadCloser, error) {
	found, ok := s.find(name)
	if !ok || found.layer >= len(s.layers) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return s.layers[found.layer](found.real)
}

// ReadFile reads a whole file by a name written the way the scripts write it.
func (s *Source) ReadFile(name string) ([]byte, error) {
	file, err := s.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(file)
}
