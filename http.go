package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"log"
	"net"
	"net/http"
	"path"
	"regexp"
	"strings"

	yaml "gopkg.in/yaml.v2"
	"novit.nc/direktil/pkg/clustersconfig"
)

var (
	hostsToken = flag.String("hosts-token", "", "Token to give to access /hosts (open is none)")

	reHost = regexp.MustCompile("^/hosts/([^/]+)/([^/]+)$")

	trustXFF = flag.Bool("trust-xff", true, "Trust the X-Forwarded-For header")
)

func authorizeHosts(r *http.Request) bool {
	if *hostsToken == "" {
		// access is open
		return true
	}

	reqToken := r.Header.Get("Authorization")

	return reqToken == "Bearer "+*hostsToken
}

func forbidden(w http.ResponseWriter, r *http.Request) {
	log.Printf("denied access to %s from %s", r.RequestURI, r.RemoteAddr)
	http.Error(w, "Forbidden", http.StatusForbidden)
}

func serveHostByIP(w http.ResponseWriter, r *http.Request) {
	host, cfg := hostByIP(w, r)
	if host == nil {
		return
	}

	what := path.Base(r.URL.Path)

	renderHost(w, r, what, host, cfg)
}

func hostByIP(w http.ResponseWriter, r *http.Request) (*clustersconfig.Host, *clustersconfig.Config) {
	remoteAddr := r.RemoteAddr

	if *trustXFF {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			remoteAddr = strings.Split(xff, ",")[0]
		}
	}

	hostIP, _, err := net.SplitHostPort(remoteAddr)

	if err != nil {
		hostIP = remoteAddr
	}

	cfg, err := readConfig()
	if err != nil {
		http.Error(w, "", http.StatusServiceUnavailable)
		return nil, nil
	}

	host := cfg.HostByIP(hostIP)

	if host == nil {
		log.Print("no host found for IP ", hostIP)
		http.NotFound(w, r)
		return nil, nil
	}

	return host, cfg
}

func serveHosts(w http.ResponseWriter, r *http.Request) {
	if !authorizeHosts(r) {
		forbidden(w, r)
		return
	}

	cfg, err := readConfig()
	if err != nil {
		http.Error(w, "", http.StatusServiceUnavailable)
		return
	}

	renderJSON(w, cfg.Hosts)
}

func serveHost(w http.ResponseWriter, r *http.Request) {
	if !authorizeHosts(r) {
		forbidden(w, r)
		return
	}

	match := reHost.FindStringSubmatch(r.URL.Path)
	if match == nil {
		http.NotFound(w, r)
		return
	}

	hostName, what := match[1], match[2]

	cfg, err := readConfig()
	if err != nil {
		http.Error(w, "", http.StatusServiceUnavailable)
		return
	}

	host := cfg.Host(hostName)

	if host == nil {
		host = cfg.HostByMAC(hostName)
	}

	if host == nil {
		log.Printf("no host with name or MAC %q", hostName)
		http.NotFound(w, r)
		return
	}

	renderHost(w, r, what, host, cfg)
}

func renderHost(w http.ResponseWriter, r *http.Request, what string, host *clustersconfig.Host, cfg *clustersconfig.Config) {
	ctx, err := newRenderContext(host, cfg)
	if err != nil {
		log.Printf("host %s: %s: failed to render: %v", what, host.Name, err)
		http.Error(w, "", http.StatusServiceUnavailable)
		return
	}

	switch what {
	case "ipxe":
		w.Header().Set("Content-Type", "text/x-ipxe")
	case "config":
		w.Header().Set("Content-Type", "text/vnd.yaml")
	default:
		w.Header().Set("Content-Type", "application/octet-stream")
	}

	switch what {
	case "ipxe":
		err = renderIPXE(w, ctx)

	case "kernel":
		err = renderKernel(w, r, ctx)

	case "initrd":
		err = renderCtx(w, r, ctx, "initrd", buildInitrd)

	case "boot.iso":
		err = renderCtx(w, r, ctx, "boot.iso", buildBootISO)

	case "boot.tar":
		err = renderCtx(w, r, ctx, "boot.tar", buildBootTar)

	case "config":
		err = renderConfig(w, r, ctx)

	case "static-pods":
		if ctx.Group.StaticPods == "" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		err = renderStaticPods(w, r, ctx)

	default:
		http.NotFound(w, r)
	}

	if err != nil {
		if isNotFound(err) {
			log.Printf("host %s: %s: %v", what, host.Name, err)
			http.NotFound(w, r)
		} else {
			log.Printf("host %s: %s: failed to render: %v", what, host.Name, err)
			http.Error(w, "", http.StatusServiceUnavailable)
		}
	}
}

func renderJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Add("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func serveClusters(w http.ResponseWriter, r *http.Request) {
	cfg, err := readConfig()
	if err != nil {
		http.Error(w, "", http.StatusServiceUnavailable)
		return
	}

	clusterNames := make([]string, len(cfg.Clusters))
	for i, cluster := range cfg.Clusters {
		clusterNames[i] = cluster.Name
	}

	renderJSON(w, clusterNames)
}

func serveCluster(w http.ResponseWriter, r *http.Request) {
	// "/clusters/<name>/<what>" split => "", "clusters", "<name>", "<what>"
	p := strings.Split(r.URL.Path, "/")

	if len(p) != 4 {
		http.NotFound(w, r)
		return
	}

	clusterName := p[2]

	p = strings.SplitN(p[3], ".", 2)
	what := p[0]
	format := ""
	if len(p) > 1 {
		format = p[1]
	}

	cfg, err := readConfig()
	if err != nil {
		http.Error(w, "", http.StatusServiceUnavailable)
		return
	}

	cluster := cfg.Cluster(clusterName)
	if cluster == nil {
		http.NotFound(w, r)
		return
	}

	switch what {
	case "addons":
		if cluster.Addons == "" {
			log.Printf("cluster %q has no addons defined", clusterName)
			http.NotFound(w, r)
			return
		}

		addons := cfg.Addons[cluster.Addons]
		if addons == nil {
			log.Printf("cluster %q: no addons with name %q", clusterName, cluster.Addons)
			http.NotFound(w, r)
			return
		}

		clusterAsMap := asMap(cluster)
		clusterAsMap["kubernetes_svc_ip"] = cluster.KubernetesSvcIP().String()
		clusterAsMap["dns_svc_ip"] = cluster.DNSSvcIP().String()

		cm := newConfigMap("cluster-addons")

		for _, addon := range addons {
			buf := &bytes.Buffer{}
			err := addon.Execute(buf, clusterAsMap, nil)

			if err != nil {
				log.Printf("cluster %q: addons %q: failed to render %q: %v",
					clusterName, cluster.Addons, addon.Name, err)
				http.Error(w, "", http.StatusServiceUnavailable)
				return
			}

			cm.Data[addon.Name] = buf.String()
		}

		switch format {
		case "yaml":
			for name, data := range cm.Data {
				w.Write([]byte("\n# addon: " + name + "\n---\n\n"))
				w.Write([]byte(data))
			}

		default:
			yaml.NewEncoder(w).Encode(cm)
		}

	default:
		http.NotFound(w, r)
	}
}
