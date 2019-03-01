package clustersconfig

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"

	yaml "gopkg.in/yaml.v2"
)

// Debug enables debug logs from this package.
var Debug = false

func FromDir(dirPath, defaultsPath string) (*Config, error) {
	if Debug {
		log.Printf("loading config from dir %s (defaults from %s)", dirPath, defaultsPath)
	}

	defaults, err := NewDefaults(defaultsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load defaults: %v", err)
	}

	store := &dirStore{dirPath}
	load := func(dir, name string, out Rev) error {
		ba, err := store.Get(path.Join(dir, name))
		if err != nil {
			return fmt.Errorf("failed to load %s/%s from dir: %v", dir, name, err)
		}
		if err = defaults.Load(dir, ".yaml", out, ba); err != nil {
			return fmt.Errorf("failed to enrich %s/%s from defaults: %v", dir, name, err)
		}
		return nil
	}

	config := &Config{Addons: make(map[string][]*Template)}

	// load clusters
	names, err := store.List("clusters")
	if err != nil {
		return nil, fmt.Errorf("failed to list clusters: %v", err)
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
		return nil, fmt.Errorf("failed to list groups: %v", err)
	}

	read := func(rev, filePath string) (data []byte, fromDefaults bool, err error) {
		data, err = store.Get(filePath)
		if err != nil {
			err = fmt.Errorf("faild to read %s: %v", filePath, err)
			return
		}

		if data != nil {
			return // ok
		}

		if len(rev) == 0 {
			err = fmt.Errorf("entry not found: %s", filePath)
			return
		}

		data, err = defaults.ReadAll(rev, filePath+".yaml")
		if err != nil {
			err = fmt.Errorf("failed to read %s:%s: %v", rev, filePath, err)
			return
		}

		fromDefaults = true
		return
	}

	template := func(rev, dir, name string, templates *[]*Template) (ref string, err error) {
		ref = name
		if len(name) == 0 {
			return
		}

		ba, fromDefaults, err := read(rev, path.Join(dir, name))
		if err != nil {
			return
		}

		if fromDefaults {
			ref = rev + ":" + name
		}

		if !hasTemplate(ref, *templates) {
			if Debug {
				log.Printf("new template in %s: %s", dir, ref)
			}

			*templates = append(*templates, &Template{
				Name:     ref,
				Template: string(ba),
			})
		}

		return
	}

	for _, name := range names {
		group := &Group{Name: name}
		if err := load("groups", name, group); err != nil {
			return nil, err
		}

		group.Config, err = template(group.Rev(), "configs", group.Config, &config.Configs)
		if err != nil {
			return nil, fmt.Errorf("failed to load config for group %q: %v", name, err)
		}

		if Debug {
			log.Printf("group %q: config=%q static_pods=%q", group.Name, group.Config, group.StaticPods)
		}

		group.StaticPods, err = template(group.Rev(), "static-pods", group.StaticPods, &config.StaticPods)
		if err != nil {
			return nil, fmt.Errorf("failed to load static pods for group %q: %v", name, err)
		}

		config.Groups = append(config.Groups, group)
	}

	// load hosts
	names, err = store.List("hosts")
	if err != nil {
		return nil, fmt.Errorf("failed to list hosts: %v", err)
	}

	for _, name := range names {
		o := &Host{Name: name}
		if err := load("hosts", name, o); err != nil {
			return nil, err
		}

		config.Hosts = append(config.Hosts, o)
	}

	// load config templates
	loadTemplates := func(rev, dir string, templates *[]*Template) error {
		names, err := store.List(dir)
		if err != nil {
			return fmt.Errorf("failed to list %s: %v", dir, err)
		}

		if len(rev) != 0 {
			var defaultsNames []string
			defaultsNames, err = defaults.List(rev, dir)
			if err != nil {
				return fmt.Errorf("failed to list %s:%s: %v", rev, dir, err)
			}

			names = append(names, defaultsNames...)
		}

		for _, name := range names {
			if hasTemplate(name, *templates) {
				continue
			}

			ba, _, err := read(rev, path.Join(dir, name))
			if err != nil {
				return err
			}

			*templates = append(*templates, &Template{
				Name:     name,
				Template: string(ba),
			})
		}

		return nil
	}

	for _, cluster := range config.Clusters {
		addonSet := cluster.Addons
		if len(addonSet) == 0 {
			continue
		}

		if _, ok := config.Addons[addonSet]; ok {
			continue
		}

		templates := make([]*Template, 0)
		if err = loadTemplates(cluster.Rev(), path.Join("addons", addonSet), &templates); err != nil {
			return nil, err
		}

		config.Addons[addonSet] = templates
	}

	// load SSL configuration
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

func hasTemplate(name string, templates []*Template) bool {
	for _, tmpl := range templates {
		if tmpl.Name == name {
			return true
		}
	}
	return false
}

type dirStore struct {
	path string
}

// listDir
func (b *dirStore) listDir(prefix string) (subDirs []string, err error) {
	entries, err := ioutil.ReadDir(filepath.Join(b.path, prefix))
	if err != nil {
		return
	}

	subDirs = make([]string, 0, len(entries))

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()

		if len(name) == 0 || name[0] == '.' {
			continue
		}

		subDirs = append(subDirs, name)
	}

	return
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
func (b *dirStore) Get(key string) (ba []byte, err error) {
	ba, err = ioutil.ReadFile(filepath.Join(b.path, filepath.Join(path.Split(key))+".yaml"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	return
}
