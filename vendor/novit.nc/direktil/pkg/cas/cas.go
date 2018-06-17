// Package cas provides a content-accessible storage implementation
package cas

import (
	"io"
	"time"
)

// Content is an item's content.
type Content interface {
	io.Reader
	io.Seeker
	io.Closer
}

// Meta is an item's metadata.
type Meta interface {
	Size() int64
	ModTime() time.Time
}

// Store is a CAS store.
type Store interface {
	GetOrCreate(tag, item string, create func(io.Writer) error) (content Content, meta Meta, err error)
	Tags() (tags []string, err error)
	Remove(tag string) error
}
