package config

import (
	"fmt"
	"io/ioutil"
	"os"

	yaml "gopkg.in/yaml.v2"
)

// Config represent this system's configuration
type Config struct {
	Vars []VarDefault

	Layers  []string
	Modules []string

	RootUser struct {
		PasswordHash   string   `yaml:"password_hash"`
		AuthorizedKeys []string `yaml:"authorized_keys"`
	} `yaml:"root_user"`

	Storage StorageConfig

	Groups []GroupDef
	Users  []UserDef

	Files []FileDef

	Networks []NetworkDef
}

type VarDefault struct {
	Name    string
	Default string
}

type StorageConfig struct {
	UdevMatch     string   `yaml:"udev_match"`
	RemoveVolumes []string `yaml:"remove_volumes"`
	Volumes       []VolumeDef
}

type VolumeDef struct {
	Name    string
	Size    string
	Extents string
	FS      string
	Mount   struct {
		Path    string
		Options string
	}
}

type GroupDef struct {
	Name string
	Gid  int
}

type UserDef struct {
	Name string
	Gid  int
	Uid  int
}

type FileDef struct {
	Path    string
	Mode    os.FileMode
	Content string
}

type NetworkDef struct {
	Match struct {
		All  bool
		Name string
		Ping *struct {
			Source  string
			Target  string
			Count   int
			Timeout int
		}
	}
	Optional bool
	Script   string
}

func Load(file string) (config *Config, err error) {
	config = &Config{}

	configData, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %v", file, err)
	}

	err = yaml.Unmarshal(configData, config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s: %v", file, err)
	}

	return
}
