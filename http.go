package main

import (
	"encoding/json"
	"flag"
	"log"
	"net"
	"net/http"
	"path"
	"regexp"
	"strings"

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
		xff := r.Header.Get("X-Forwarded-For")
		if xff != "" {
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

	hostNames := make([]string, len(cfg.Hosts))
	for i, host := range cfg.Hosts {
		hostNames[i] = host.Name
	}

	renderJSON(w, hostNames)
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
	ctx := newRenderContext(host, cfg)

	switch what {
	case "ipxe":
		w.Header().Set("Content-Type", "text/x-ipxe")
	case "config":
		w.Header().Set("Content-Type", "text/vnd.yaml")
	default:
		w.Header().Set("Content-Type", "application/octet-stream")
	}

	var err error

	switch what {
	case "ipxe":
		err = renderIPXE(w, ctx)

	case "kernel":
		err = renderKernel(w, r, ctx)

	case "initrd":
		err = renderCtx(w, r, ctx, "initrd", buildInitrd)

	case "boot.iso":
		err = renderCtx(w, r, ctx, "boot.iso", buildBootISO)

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
