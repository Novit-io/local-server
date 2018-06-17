package clustersconfig

import (
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"

	yaml "gopkg.in/yaml.v2"
)

func FromDir(dirPath string) (*Config, error) {
	config := &Config{}

	store := dirStore{dirPath}
	load := func(dir, name string, out interface{}) error {
		ba, err := store.Get(path.Join(dir, name))
		if err != nil {
			return err
		}
		return yaml.Unmarshal(ba, out)
	}

	// load clusters
	names, err := store.List("clusters")
	if err != nil {
		return nil, err
	}

	for _, name := range names {
		cluster := &Cluster{Name: name}
		if err := load("clusters", name, cluster); err != nil {
			return nil, err
		}

		config.Clusters = append(config.Clusters, cluster)
	}

	// load groups
	names, err = store.List("groups")
	if err != nil {
		return nil, err
	}

	for _, name := range names {
		o := &Group{Name: name}
		if err := load("groups", name, o); err != nil {
			return nil, err
		}

		config.Groups = append(config.Groups, o)
	}

	// load hosts
	names, err = store.List("hosts")
	if err != nil {
		return nil, err
	}

	for _, name := range names {
		o := &Host{Name: name}
		if err := load("hosts", name, o); err != nil {
			return nil, err
		}

		config.Hosts = append(config.Hosts, o)
	}

	// load config templates
	loadTemplates := func(dir string, templates *[]*Template) error {
		names, err = store.List(dir)
		if err != nil {
			return err
		}

		for _, name := range names {
			ba, err := store.Get(path.Join(dir, name))
			if err != nil {
				return err
			}

			o := &Template{Name: name, Template: string(ba)}

			*templates = append(*templates, o)
		}

		return nil
	}

	if err := loadTemplates("configs", &config.Configs); err != nil {
		return nil, err
	}
	if err := loadTemplates("static-pods", &config.StaticPods); err != nil {
		return nil, err
	}

	if ba, err := ioutil.ReadFile(filepath.Join(dirPath, "ssl-config.json")); err == nil {
		config.SSLConfig = string(ba)

	} else if !os.IsNotExist(err) {
		return nil, err
	}

	if ba, err := ioutil.ReadFile(filepath.Join(dirPath, "cert-requests.yaml")); err == nil {
		reqs := make([]*CertRequest, 0)
		if err = yaml.Unmarshal(ba, &reqs); err != nil {
			return nil, err
		}

		config.CertRequests = reqs

	} else if !os.IsNotExist(err) {
		return nil, err
	}

	return config, nil
}

type dirStore struct {
	path string
}

// Names is part of the kvStore interface
func (b *dirStore) List(prefix string) ([]string, error) {
	files, err := filepath.Glob(filepath.Join(b.path, filepath.Join(path.Split(prefix)), "*.yaml"))
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(files))
	for _, f := range files {
		f2 := strings.TrimSuffix(f, ".yaml")
		f2 = filepath.Base(f2)

		if f2[0] == '.' { // ignore hidden files
			continue
		}

		names = append(names, f2)
	}

	return names, nil
}

// Load is part of the DataBackend interface
func (b *dirStore) Get(key string) ([]byte, error) {
	return ioutil.ReadFile(filepath.Join(b.path, filepath.Join(path.Split(key))+".yaml"))
}
