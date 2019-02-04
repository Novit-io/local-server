package main

import (
	"log"
	"path"

	restful "github.com/emicklei/go-restful"
)

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
		b("config").Doc("Get the host's configuration"),

		// metal/local HDD install
		b("boot.img").Doc("Get the host's boot disk image"),
		b("boot.img.gz").Doc("Get the host's boot disk image (gzip compressed)"),
		b("boot.img.lz4").Doc("Get the host's boot disk image (lz4 compressed)"),

		// metal/local HDD upgrades
		b("boot.tar").Doc("Get the host's /boot archive (ie: for metal upgrades)"),

		// read-only ISO support
		b("boot.iso").Doc("Get the host's boot CD-ROM image"),

		// netboot support
		b("ipxe").Doc("Get the host's IPXE code (for netboot)"),
		b("kernel").Doc("Get the host's kernel (ie: for netboot)"),
		b("initrd").Doc("Get the host's initial RAM disk (ie: for netboot)"),
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
