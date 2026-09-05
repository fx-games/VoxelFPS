package zdwep

import (
	"archive/zip"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
)

// Source is a ZDoom mod to migrate from. It is either a pk3 (a zip) or a
// directory holding the same layout, and both arrive here as an fs.FS.
//
// Lump names in DECORATE are written without regard for case or extension:
// a script says #include "actors/weapons/shotgun.txt" and the archive holds
// ACTORS/WEAPONS/Shotgun.TXT. Source resolves that, so the rest of the
// migration can ask for names the way the scripts write them.
type Source struct {
	fsys  fs.FS
	close func() error
	// byLower maps a lowercased path to the real one, and byLowerNoExt maps a
	// lowercased extension-less path to the real one, which is how DECORATE
	// refers to lumps like DECORATE or SNDINFO.
	byLower      map[string]string
	byLowerNoExt map[string]string
	names        []string
}

// OpenSource opens a pk3 or a directory. The caller closes it.
func OpenSource(location string) (*Source, error) {
	lower := strings.ToLower(location)
	if strings.HasSuffix(lower, ".pk3") || strings.HasSuffix(lower, ".zip") ||
		strings.HasSuffix(lower, ".wad") {
		reader, err := zip.OpenReader(location)
		if err != nil {
			return nil, err
		}
		return newSource(reader, reader.Close)
	}
	info, err := os.Stat(location)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, &fs.PathError{Op: "open", Path: location, Err: fs.ErrInvalid}
	}
	return newSource(os.DirFS(location), func() error { return nil })
}

// NewSourceFS wraps an already open filesystem, which is how the tests supply a
// mod without touching the disk.
func NewSourceFS(fsys fs.FS) (*Source, error) {
	return newSource(fsys, func() error { return nil })
}

func newSource(fsys fs.FS, closer func() error) (*Source, error) {
	s := &Source{
		fsys:         fsys,
		close:        closer,
		byLower:      make(map[string]string),
		byLowerNoExt: make(map[string]string),
	}
	err := fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		s.names = append(s.names, name)
		lower := strings.ToLower(name)
		s.byLower[lower] = name
		// Two lumps whose names differ only by extension are a mod bug we do not
		// have to solve; the first one wins and stays put.
		noExt := strings.TrimSuffix(lower, path.Ext(lower))
		if _, taken := s.byLowerNoExt[noExt]; !taken {
			s.byLowerNoExt[noExt] = name
		}
		return nil
	})
	if err != nil {
		_ = closer()
		return nil, err
	}
	return s, nil
}

func (s *Source) Close() error { return s.close() }

// Names lists every file in the source, in walk order.
func (s *Source) Names() []string { return s.names }

// Resolve turns a name as a script writes it into the real path in the source,
// trying an exact match, then case-insensitive, then case-insensitive ignoring
// the extension.
func (s *Source) Resolve(name string) (string, bool) {
	name = strings.TrimPrefix(strings.ReplaceAll(name, "\\", "/"), "./")
	if _, err := fs.Stat(s.fsys, name); err == nil {
		return name, true
	}
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
func (s *Source) Open(name string) (fs.File, error) {
	real, ok := s.Resolve(name)
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return s.fsys.Open(real)
}

// ReadFile reads a whole file by a name written the way the scripts write it.
func (s *Source) ReadFile(name string) ([]byte, error) {
	f, err := s.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}
