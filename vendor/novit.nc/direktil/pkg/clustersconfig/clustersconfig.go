package clustersconfig

import (
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"strings"
	"text/template"

	yaml "gopkg.in/yaml.v2"
)

type Config struct {
	Hosts        []*Host
	Groups       []*Group
	Clusters     []*Cluster
	Configs      []*Template
	StaticPods   []*Template    `yaml:"static_pods"`
	SSLConfig    string         `yaml:"ssl_config"`
	CertRequests []*CertRequest `yaml:"cert_requests"`
}

func FromBytes(data []byte) (*Config, error) {
	config := &Config{}
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, err
	}
	return config, nil
}

func FromFile(path string) (*Config, error) {
	ba, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return FromBytes(ba)
}

func (c *Config) Host(name string) *Host {
	for _, host := range c.Hosts {
		if host.Name == name {
			return host
		}
	}
	return nil
}

func (c *Config) HostByIP(ip string) *Host {
	for _, host := range c.Hosts {
		if host.IP == ip {
			return host
		}

		for _, otherIP := range host.IPs {
			if otherIP == ip {
				return host
			}
		}
	}
	return nil
}

func (c *Config) HostByMAC(mac string) *Host {
	// a bit of normalization
	mac = strings.Replace(strings.ToLower(mac), "-", ":", -1)

	for _, host := range c.Hosts {
		if strings.ToLower(host.MAC) == mac {
			return host
		}
	}

	return nil
}

func (c *Config) Group(name string) *Group {
	for _, group := range c.Groups {
		if group.Name == name {
			return group
		}
	}
	return nil
}

func (c *Config) Cluster(name string) *Cluster {
	for _, cluster := range c.Clusters {
		if cluster.Name == name {
			return cluster
		}
	}
	return nil
}

func (c *Config) ConfigTemplate(name string) *Template {
	for _, cfg := range c.Configs {
		if cfg.Name == name {
			return cfg
		}
	}
	return nil
}

func (c *Config) StaticPodsTemplate(name string) *Template {
	for _, s := range c.StaticPods {
		if s.Name == name {
			return s
		}
	}
	return nil
}

func (c *Config) CSR(name string) *CertRequest {
	for _, s := range c.CertRequests {
		if s.Name == name {
			return s
		}
	}
	return nil
}

func (c *Config) SaveTo(path string) error {
	ba, err := yaml.Marshal(c)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(path, ba, 0600)
}

type Template struct {
	Name     string
	Template string

	parsedTemplate *template.Template
}

func (t *Template) Execute(wr io.Writer, data interface{}, extraFuncs map[string]interface{}) error {
	if t.parsedTemplate == nil {
		var templateFuncs = map[string]interface{}{
			"indent": func(indent, s string) (indented string) {
				indented = indent + strings.Replace(s, "\n", "\n"+indent, -1)
				return
			},
		}

		for name, f := range extraFuncs {
			templateFuncs[name] = f
		}

		tmpl, err := template.New(t.Name).
			Funcs(templateFuncs).
			Parse(t.Template)
		if err != nil {
			return err
		}
		t.parsedTemplate = tmpl
	}

	return t.parsedTemplate.Execute(wr, data)
}

// Host represents a host served by this server.
type Host struct {
	Name    string
	MAC     string
	IP      string
	IPs     []string
	Cluster string
	Group   string
	Vars    Vars
}

// Group represents a group of hosts and provides their configuration.
type Group struct {
	Name       string
	Master     bool
	IPXE       string
	Kernel     string
	Initrd     string
	Config     string
	StaticPods string `yaml:"static_pods"`
	Versions   map[string]string
	Vars       Vars
}

// Vars store user-defined key-values
type Vars map[string]interface{}

// Cluster represents a cluster of hosts, allowing for cluster-wide variables.
type Cluster struct {
	Name    string
	Domain  string
	Subnets struct {
		Services string
		Pods     string
	}
	Vars Vars
}

func (c *Cluster) KubernetesSvcIP() net.IP {
	return c.NthSvcIP(1)
}

func (c *Cluster) DNSSvcIP() net.IP {
	return c.NthSvcIP(2)
}

func (c *Cluster) NthSvcIP(n byte) net.IP {
	_, cidr, err := net.ParseCIDR(c.Subnets.Services)
	if err != nil {
		panic(fmt.Errorf("Invalid services CIDR: %v", err))
	}

	ip := cidr.IP
	ip[len(ip)-1] += n

	return ip
}
