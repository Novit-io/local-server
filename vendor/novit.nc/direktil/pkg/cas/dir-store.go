package cas

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
)

// NewDirStore create a new store backed by the given directory.
func NewDir(path string) *DirStore {
	return &DirStore{
		path: path,
	}
}

// DirStore is a Store backed by a directory.
type DirStore struct {
	path  string
	mutex sync.Mutex
}

var _ Store = &DirStore{}

// GetOrCreate is part of the Store interface.
func (s *DirStore) GetOrCreate(tag, item string, create func(io.Writer) error) (content Content, meta Meta, err error) {
	fullPath := filepath.Join(s.path, tag, item)

	f, err := os.Open(fullPath)
	if err != nil {
		s.mutex.Lock()
		defer s.mutex.Unlock()

		f, err = os.Open(fullPath)
	}

	if err != nil {
		if !os.IsNotExist(err) {
			return
		}

		err = os.MkdirAll(filepath.Dir(fullPath), 0700)
		if err != nil {
			return
		}

		partFile := fullPath + ".part"
		os.Remove(partFile)

		var out *os.File
		out, err = os.OpenFile(partFile, os.O_CREATE|os.O_TRUNC|os.O_WRONLY|os.O_EXCL, 0600)
		if err != nil {
			return
		}

		err = create(out)

		out.Close()

		if err != nil {
			os.Remove(fullPath + ".part")
			return
		}

		if err = os.Rename(fullPath+".part", fullPath); err != nil {
			return
		}

		f, err = os.Open(fullPath)
		if err != nil {
			return
		}
	}

	stat, err := os.Stat(fullPath)
	if err != nil {
		return
	}

	return f, stat, nil
}

// Tags is part of the Store interface.
func (s *DirStore) Tags() (tags []string, err error) {
	entries, err := ioutil.ReadDir(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return
	}

	tags = make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()

		if name[0] == '.' {
			continue
		}

		tags = append(tags, name)
	}

	return
}

// Remove is part of the Store interface.
func (s *DirStore) Remove(tag string) error {
	return os.RemoveAll(filepath.Join(s.path, tag))
}
