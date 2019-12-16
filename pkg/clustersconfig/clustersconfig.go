package clustersconfig

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	yaml "gopkg.in/yaml.v2"
)

var (
	templateDetailsDir = flag.String("template-details-dir",
		filepath.Join(os.TempDir(), "dkl-dir2config"),
		"write details of template execute in this dir")

	templateID = 0
)

type Config struct {
	Hosts         []*Host
	Groups        []*Group
	Clusters      []*Cluster
	Configs       []*Template
	StaticPods    []*Template            `yaml:"static_pods"`
	BootstrapPods map[string][]*Template `yaml:"bootstrap_pods"`
	Addons        map[string][]*Template
	SSLConfig     string         `yaml:"ssl_config"`
	CertRequests  []*CertRequest `yaml:"cert_requests"`
}

func FromBytes(data []byte) (*Config, error) {
	config := &Config{Addons: make(map[string][]*Template)}
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

func (t *Template) Execute(contextName, elementName string, wr io.Writer, data interface{}, extraFuncs map[string]interface{}) error {
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

	if *templateDetailsDir != "" {
		templateID++

		base := filepath.Join(*templateDetailsDir, contextName, fmt.Sprintf("%s-%03d", elementName, templateID))
		os.MkdirAll(base, 0700)

		base += string(filepath.Separator)
		log.Print("writing template details: ", base, "{in,data,out}")

		if err := ioutil.WriteFile(base+"in", []byte(t.Template), 0600); err != nil {
			return err
		}

		yamlBytes, err := yaml.Marshal(data)
		if err != nil {
			return err
		}

		if err := ioutil.WriteFile(base+"data", yamlBytes, 0600); err != nil {
			return err
		}

		out, err := os.Create(base + "out")
		if err != nil {
			return err
		}

		defer out.Close()

		wr = io.MultiWriter(wr, out)
	}

	return t.parsedTemplate.Execute(wr, data)
}

// Host represents a host served by this server.
type Host struct {
	WithRev

	Name        string
	Labels      map[string]string
	Annotations map[string]string

	MAC     string
	IP      string
	IPs     []string
	Cluster string
	Group   string
	Vars    Vars
}

// Group represents a group of hosts and provides their configuration.
type Group struct {
	WithRev

	Name        string
	Labels      map[string]string
	Annotations map[string]string

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
	WithRev

	Name        string
	Labels      map[string]string
	Annotations map[string]string

	Domain        string
	Addons        string
	BootstrapPods string `yaml:"bootstrap_pods"`
	Subnets       struct {
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
