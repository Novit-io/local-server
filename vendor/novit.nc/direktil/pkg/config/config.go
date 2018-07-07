package config

import (
	"fmt"
	"io"
	"os"

	yaml "gopkg.in/yaml.v2"
)

// Load a config from a file.
func Load(file string) (config *Config, err error) {
	f, err := os.Open(file)
	if err != nil {
		return
	}

	defer f.Close()

	config, err = Read(f)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s: %v", file, err)
	}

	return
}

// Read a config from a reader.
func Read(reader io.Reader) (config *Config, err error) {
	config = &Config{}

	err = yaml.NewDecoder(reader).Decode(config)

	if err != nil {
		return nil, err
	}

	return
}

// Parse the config in data.
func Parse(data []byte) (config *Config, err error) {
	config = &Config{}

	err = yaml.Unmarshal(data, config)
	if err != nil {
		return nil, err
	}

	return
}

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
