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
	hostDoc string
	getHost func(req *restful.Request) string
}

func (ws *wsHost) register(rws *restful.WebService, alterRB func(*restful.RouteBuilder)) {
	b := func(what string) *restful.RouteBuilder {
		return rws.GET(ws.prefix + "/" + what).To(ws.render)
	}

	for _, rb := range []*restful.RouteBuilder{
		rws.GET(ws.prefix).To(ws.get).
			Doc("Get the "+ws.hostDoc+"'s details").
			Returns(200, "OK", localconfig.Host{}),

		// raw configuration
		b("config").
			Produces(mime.YAML).
			Doc("Get the " + ws.hostDoc + "'s configuration"),

		b("config.json").
			Doc("Get the " + ws.hostDoc + "'s configuration (as JSON)"),

		// metal/local HDD install
		b("boot.img").
			Produces(mime.DISK).
			Doc("Get the " + ws.hostDoc + "'s boot disk image"),

		b("boot.img.gz").
			Produces(mime.DISK + "+gzip").
			Doc("Get the " + ws.hostDoc + "'s boot disk image (gzip compressed)"),

		b("boot.img.lz4").
			Produces(mime.DISK + "+lz4").
			Doc("Get the " + ws.hostDoc + "'s boot disk image (lz4 compressed)"),

		// metal/local HDD upgrades
		b("boot.tar").
			Produces(mime.TAR).
			Doc("Get the " + ws.hostDoc + "'s /boot archive (ie: for metal upgrades)"),

		// read-only ISO support
		b("boot.iso").
			Produces(mime.ISO).
			Param(cmdlineParam).
			Doc("Get the " + ws.hostDoc + "'s boot CD-ROM image"),

		// netboot support
		b("ipxe").
			Produces(mime.IPXE).
			Doc("Get the " + ws.hostDoc + "'s IPXE code (for netboot)"),

		b("kernel").
			Produces(mime.OCTET).
			Doc("Get the " + ws.hostDoc + "'s kernel (ie: for netboot)"),

		b("initrd").
			Produces(mime.OCTET).
			Doc("Get the " + ws.hostDoc + "'s initial RAM disk (ie: for netboot)"),
	} {
		alterRB(rb)
		rws.Route(rb)
	}
}

func (ws *wsHost) host(req *restful.Request, resp *restful.Response) (host *localconfig.Host, cfg *localconfig.Config) {
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

	host = cfg.Host(hostname)
	if host == nil {
		log.Print("no host named ", hostname)
		wsNotFound(req, resp)
		return
	}
	return
}

func (ws *wsHost) get(req *restful.Request, resp *restful.Response) {
	host, _ := ws.host(req, resp)
	if host == nil {
		return
	}

	resp.WriteEntity(host)
}

func (ws *wsHost) render(req *restful.Request, resp *restful.Response) {
	host, cfg := ws.host(req, resp)
	if host == nil {
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
	case "config":
		err = renderConfig(w, r, ctx, false)

	case "config.json":
		err = renderConfig(w, r, ctx, true)

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

	default:
		http.NotFound(w, r)
	}

	if err != nil {
		log.Printf("host %s: %s: failed to render: %v", what, host.Name, err)
		http.Error(w, "", http.StatusServiceUnavailable)
	}
}
