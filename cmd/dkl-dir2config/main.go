package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"

	yaml "gopkg.in/yaml.v2"
	"novit.nc/direktil/pkg/localconfig"

	"novit.nc/direktil/local-server/pkg/clustersconfig"
)

var (
	dir = flag.String("in", ".", "Source directory")

	src *clustersconfig.Config
	dst *localconfig.Config
)

func loadSrc() {
	var err error

	src, err = clustersconfig.FromDir(*dir)
	if err != nil {
		log.Fatal("failed to load config from dir: ", err)
	}

}

func main() {
	outPath := flag.String("out", "config.yaml", "Output file")
	flag.Parse()

	loadSrc()

	dst = &localconfig.Config{
		SSLConfig: src.SSLConfig,
	}

	// ----------------------------------------------------------------------
	for _, cluster := range src.Clusters {
		dst.Clusters = append(dst.Clusters, &localconfig.Cluster{
			Name:   cluster.Name,
			Addons: renderAddons(cluster),
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

		dst.Hosts = append(dst.Hosts, &localconfig.Host{
			Name: host.Name,
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

func renderAddons(cluster *clustersconfig.Cluster) string {
	addons := src.Addons[cluster.Addons]
	if addons == nil {
		log.Fatalf("cluster %q: no addons with name %q", cluster.Name, cluster.Addons)
	}

	clusterAsMap := asMap(cluster)
	clusterAsMap["kubernetes_svc_ip"] = cluster.KubernetesSvcIP().String()
	clusterAsMap["dns_svc_ip"] = cluster.DNSSvcIP().String()

	buf := &bytes.Buffer{}

	for _, addon := range addons {
		fmt.Fprintf(buf, "---\n# addon: %s\n", addon.Name)
		err := addon.Execute(buf, clusterAsMap, nil)

		if err != nil {
			log.Fatalf("cluster %q: addons %q: failed to render %q: %v",
				cluster.Name, cluster.Addons, addon.Name, err)
		}

		fmt.Fprintln(buf)
	}

	return buf.String()
}
