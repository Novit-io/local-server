package clustersconfig

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"path"
	"path/filepath"
	"strings"

	billy "gopkg.in/src-d/go-billy.v4"
	git "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	yaml "gopkg.in/yaml.v2"
)

type Defaults struct {
	repo *git.Repository
	fs   billy.Filesystem
}

type defaultRef struct {
	From string
}

func NewDefaults(path string) (d *Defaults, err error) {
	repo, err := git.PlainOpen(path)
	if err != nil {
		return
	}

	d = &Defaults{
		repo: repo,
	}

	return
}

func (d *Defaults) Load(dir, suffix string, value Rev, data []byte) (err error) {
	ref := defaultRef{}

	if err = yaml.Unmarshal(data, &ref); err != nil {
		return
	}

	if len(ref.From) != 0 {
		if Debug {
			log.Printf("loading defaults %q", ref.From)
		}

		parts := strings.SplitN(ref.From, ":", 2)
		if len(parts) != 2 {
			err = fmt.Errorf("bad default reference: %q", ref.From)
			return
		}
		rev, fileName := parts[0], parts[1]

		if err = d.decodeDefault(rev, path.Join(dir, fileName+suffix), value); err != nil {
			return
		}

		value.SetRev(rev)
	}

	err = yaml.Unmarshal(data, value)

	return
}

func (d *Defaults) Open(rev, filePath string) (rd io.Reader, err error) {
	log.Printf("openning defaults at %s:%s", rev, filePath)
	tree, err := d.treeAt(rev)
	if err != nil {
		return
	}

	file, err := tree.File(filePath)
	if err == object.ErrFileNotFound {
		return nil, nil
	} else if err != nil {
		return
	}

	return file.Reader()
}

func (d *Defaults) ReadAll(rev, filePath string) (ba []byte, err error) {
	rd, err := d.Open(rev, filePath)
	if err != nil || rd == nil {
		return
	}
	return ioutil.ReadAll(rd)
}

func (d *Defaults) List(rev, dir string) (names []string, err error) {
	log.Printf("listing defaults at %s:%s", rev, dir)
	tree, err := d.treeAt(rev)
	if err != nil {
		return
	}

	dirPrefix := dir + "/"
	err = tree.Files().ForEach(func(f *object.File) (err error) {
		if !strings.HasPrefix(f.Name, dirPrefix) {
			return
		}
		if !strings.HasSuffix(f.Name, ".yaml") {
			return
		}
		names = append(names, strings.TrimSuffix(filepath.Base(f.Name), ".yaml"))
		return
	})
	return
}

func (d *Defaults) treeAt(rev string) (tree *object.Tree, err error) {
	h, err := d.repo.ResolveRevision(plumbing.Revision(rev))
	if err != nil {
		return
	}

	obj, err := d.repo.Object(plumbing.AnyObject, *h)
	if err != nil {
		return
	}

	for {
		switch o := obj.(type) {
		case *object.Tag: // tag -> commit
			obj, err = o.Object()

		case *object.Commit: // commit -> tree
			msg := o.Message
			if len(msg) > 30 {
				msg = msg[:27] + "..."
			}
			log.Printf("open defaults at commit %s: %s", o.Hash.String()[:7], msg)
			return o.Tree()

		default:
			err = object.ErrUnsupportedObject
		}

		if err != nil {
			return
		}
	}
}

func (d *Defaults) decodeDefault(rev, filePath string, value Rev) (err error) {
	ba, err := d.ReadAll(rev, filePath)

	if err != nil {
		return
	}

	return yaml.Unmarshal(ba, value)
}
