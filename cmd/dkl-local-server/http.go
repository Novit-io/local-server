package main

import (
	"flag"
	"log"
	"net/http"
	"regexp"

	"novit.nc/direktil/pkg/localconfig"
)

var (
	reHost = regexp.MustCompile("^/hosts/([^/]+)/([^/]+)$")

	trustXFF = flag.Bool("trust-xff", true, "Trust the X-Forwarded-For header")
)

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
