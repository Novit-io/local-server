package main

import (
	"flag"
	"log"
	"net/http"
	"path"

	restful "github.com/emicklei/go-restful"

	"novit.nc/direktil/local-server/pkg/mime"
	"novit.nc/direktil/pkg/localconfig"
)

var trustXFF = flag.Bool("trust-xff", true, "Trust the X-Forwarded-For header")

type wsHost struct {
	prefix  string
	getHost func(req *restful.Request) string
}

func (ws *wsHost) register(rws *restful.WebService, alterRB func(*restful.RouteBuilder)) {
	b := func(what string) *restful.RouteBuilder {
		return rws.GET(ws.prefix + "/" + what).To(ws.render)
	}

	for _, rb := range []*restful.RouteBuilder{
		// raw configuration
		b("config").
			Produces(mime.YAML).
			Doc("Get the host's configuration"),

		// metal/local HDD install
		b("boot.img").
			Produces(mime.DISK).
			Doc("Get the host's boot disk image"),

		b("boot.img.gz").
			Produces(mime.DISK + "+gzip").
			Doc("Get the host's boot disk image (gzip compressed)"),

		b("boot.img.lz4").
			Produces(mime.DISK + "+lz4").
			Doc("Get the host's boot disk image (lz4 compressed)"),

		// metal/local HDD upgrades
		b("boot.tar").
			Produces(mime.TAR).
			Doc("Get the host's /boot archive (ie: for metal upgrades)"),

		// read-only ISO support
		b("boot.iso").
			Produces(mime.ISO).
			Doc("Get the host's boot CD-ROM image"),

		// netboot support
		b("ipxe").
			Produces(mime.IPXE).
			Doc("Get the host's IPXE code (for netboot)"),

		b("kernel").
			Produces(mime.OCTET).
			Doc("Get the host's kernel (ie: for netboot)"),

		b("initrd").
			Produces(mime.OCTET).
			Doc("Get the host's initial RAM disk (ie: for netboot)"),
	} {
		alterRB(rb)
		rws.Route(rb)
	}
}

func (ws *wsHost) render(req *restful.Request, resp *restful.Response) {
	hostname := ws.getHost(req)
	if hostname == "" {
		wsNotFound(req, resp)
		return
	}

	cfg, err := readConfig()
	if err != nil {
		wsError(resp, err)
		return
	}

	host := cfg.Host(hostname)
	if host == nil {
		log.Print("no host named ", hostname)
		wsNotFound(req, resp)
		return
	}

	what := path.Base(req.Request.URL.Path)

	renderHost(resp.ResponseWriter, req.Request, what, host, cfg)
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
