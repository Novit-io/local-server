package localconfig

import (
	"io"

	yaml "gopkg.in/yaml.v2"
)

type Host struct {
	Name string
	MACs []string
	IPs  []string

	IPXE string

	Kernel   string
	Initrd   string
	Versions map[string]string

	Config string
}

func (h *Host) WriteHashDataTo(w io.Writer) error {
	return yaml.NewEncoder(w).Encode(Host{
		Kernel:   h.Kernel,
		Initrd:   h.Initrd,
		Versions: h.Versions,
		Config:   h.Config,
	})
}
