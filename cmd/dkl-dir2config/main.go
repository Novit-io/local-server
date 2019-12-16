package main

import (
	"flag"
	"log"
	"os"

	yaml "gopkg.in/yaml.v2"
	"novit.nc/direktil/pkg/localconfig"

	"novit.nc/direktil/local-server/pkg/clustersconfig"
)

var (
	dir          = flag.String("in", ".", "Source directory")
	outPath      = flag.String("out", "config.yaml", "Output file")
	defaultsPath = flag.String("defaults", "defaults", "Path to the defaults")

	src *clustersconfig.Config
	dst *localconfig.Config
)

func loadSrc() {
	var err error
	src, err = clustersconfig.FromDir(*dir, *defaultsPath)
	if err != nil {
		log.Fatal("failed to load config from dir: ", err)
	}
}

func main() {
	flag.Parse()

	log.SetFlags(log.Ltime | log.Lmicroseconds | log.Lshortfile)

	loadSrc()

	dst = &localconfig.Config{
		SSLConfig: src.SSLConfig,
	}

	// ----------------------------------------------------------------------
	for _, cluster := range src.Clusters {
		dst.Clusters = append(dst.Clusters, &localconfig.Cluster{
			Name:          cluster.Name,
			Addons:        renderAddons(cluster),
			BootstrapPods: renderBootstrapPodsDS(cluster),
		})
	}

	// ----------------------------------------------------------------------
	for _, host := range src.Hosts {
		loadSrc() // FIXME ugly fix of some template caching or something

		log.Print("rendering host ", host.Name)
		ctx, err := newRenderContext(host, src)

		if err != nil {
			log.Fatal("failed to create render context for host ", host.Name, ": ", err)
		}

		macs := make([]string, 0)
		if host.MAC != "" {
			macs = append(macs, host.MAC)
		}

		ips := make([]string, 0)
		if len(host.IP) != 0 {
			ips = append(ips, host.IP)
		}
		ips = append(ips, host.IPs...)

		if ctx.Group.Versions["modules"] == "" {
			// default modules' version to kernel's version
			ctx.Group.Versions["modules"] = ctx.Group.Kernel
		}

		dst.Hosts = append(dst.Hosts, &localconfig.Host{
			Name: host.Name,

			ClusterName: ctx.Cluster.Name,

			Labels:      ctx.Labels,
			Annotations: ctx.Annotations,

			MACs: macs,
			IPs:  ips,

			IPXE: ctx.Group.IPXE, // TODO render

			Kernel:   ctx.Group.Kernel,
			Initrd:   ctx.Group.Initrd,
			Versions: ctx.Group.Versions,

			Config: ctx.Config(),
		})
	}

	// ----------------------------------------------------------------------
	out, err := os.Create(*outPath)
	if err != nil {
		log.Fatal("failed to create output: ", err)
	}

	defer out.Close()

	if err = yaml.NewEncoder(out).Encode(dst); err != nil {
		log.Fatal("failed to render output: ", err)
	}

}
