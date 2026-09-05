package zdwep

import (
	"archive/zip"
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
	open  func(real string) (io.ReadCloser, error)
	close func() error
	// byLower maps a lowercased path to the real one, and byLowerNoExt maps a
	// lowercased extension-less path to the real one, which is how DECORATE
	// refers to lumps like DECORATE or SNDINFO.
	byLower      map[string]string
	byLowerNoExt map[string]string
}

// OpenSource opens a pk3 or a directory. The caller closes it.
func OpenSource(location string) (*Source, error) {
	lower := strings.ToLower(location)
	if strings.HasSuffix(lower, ".pk3") || strings.HasSuffix(lower, ".zip") ||
		strings.HasSuffix(lower, ".wad") {
		return openZip(location)
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
	source.open = func(real string) (io.ReadCloser, error) {
		file, ok := entries[real]
		if !ok {
			return nil, &fs.PathError{Op: "open", Path: real, Err: fs.ErrNotExist}
		}
		return file.Open()
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
	source.open = func(real string) (io.ReadCloser, error) {
		return os.Open(filepath.Join(location, filepath.FromSlash(real)))
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
	source.open = func(real string) (io.ReadCloser, error) {
		return fsys.Open(real)
	}
	return source, nil
}

func newSource(closer func() error) *Source {
	return &Source{
		close:        closer,
		byLower:      make(map[string]string),
		byLowerNoExt: make(map[string]string),
	}
}

// note records one file under every name a script might ask for it by.
func (s *Source) note(name string) {
	s.names = append(s.names, name)
	lower := strings.ToLower(name)
	if _, taken := s.byLower[lower]; !taken {
		s.byLower[lower] = name
	}
	// Two lumps whose names differ only by extension are a mod's business, not
	// ours; the first one wins and stays put.
	noExt := strings.TrimSuffix(lower, path.Ext(lower))
	if _, taken := s.byLowerNoExt[noExt]; !taken {
		s.byLowerNoExt[noExt] = name
	}
}

func (s *Source) Close() error { return s.close() }

// Names lists every file in the source, in the order it was found.
func (s *Source) Names() []string { return s.names }

// Resolve turns a name as a script writes it into the real path in the source,
// trying an exact match, then case-insensitive, then case-insensitive ignoring
// the extension.
func (s *Source) Resolve(name string) (string, bool) {
	name = path.Clean(strings.TrimPrefix(strings.ReplaceAll(name, "\\", "/"), "./"))
	lower := strings.ToLower(name)
	if real, ok := s.byLower[lower]; ok {
		return real, true
	}
	if real, ok := s.byLowerNoExt[lower]; ok {
		return real, true
	}
	return "", false
}

// Open reads a file by a name written the way the scripts write it.
func (s *Source) Open(name string) (io.ReadCloser, error) {
	real, ok := s.Resolve(name)
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return s.open(real)
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
