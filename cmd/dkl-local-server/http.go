package main

import (
	"encoding/json"
	"flag"
	"log"
	"net"
	"net/http"
	"regexp"
	"strings"

	"novit.nc/direktil/pkg/localconfig"
)

var (
	reHost = regexp.MustCompile("^/hosts/([^/]+)/([^/]+)$")

	trustXFF = flag.Bool("trust-xff", true, "Trust the X-Forwarded-For header")
)

func serveHostByIP(w http.ResponseWriter, r *http.Request) {
	host, cfg := hostByIP(w, r)
	if host == nil {
		return
	}

	what := strings.TrimLeft(r.URL.Path, "/")

	renderHost(w, r, what, host, cfg)
}

func hostByIP(w http.ResponseWriter, r *http.Request) (*localconfig.Host, *localconfig.Config) {
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

func renderHost(w http.ResponseWriter, r *http.Request, what string, host *localconfig.Host, cfg *localconfig.Config) {
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
		err = renderCtx(w, r, ctx, what, buildInitrd)

	case "boot.iso":
		err = renderCtx(w, r, ctx, what, buildBootISO)

	case "boot.tar":
		err = renderCtx(w, r, ctx, what, buildBootTar)

	case "boot.img":
		err = renderCtx(w, r, ctx, what, buildBootImg)

	case "boot.img.gz":
		err = renderCtx(w, r, ctx, what, buildBootImgGZ)

	case "boot.img.lz4":
		err = renderCtx(w, r, ctx, what, buildBootImgLZ4)

	case "config":
		err = renderConfig(w, r, ctx)

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
	what := p[3]

	cfg, err := readConfig()
	if err != nil {
		log.Print("failed to read config: ", err)
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
		if len(cluster.Addons) == 0 {
			log.Printf("cluster %q has no addons defined", clusterName)
			http.NotFound(w, r)
			return
		}

		w.Write([]byte(cluster.Addons))

	default:
		http.NotFound(w, r)
	}
}

func writeError(w http.ResponseWriter, err error) {
	log.Print("request failed: ", err)
	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte(http.StatusText(http.StatusInternalServerError)))
}
