package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/emicklei/go-restful"
	"novit.nc/direktil/local-server/pkg/mime"
	"novit.nc/direktil/pkg/localconfig"
)

func registerWS(rest *restful.Container) {
	// Admin-level APIs
	ws := &restful.WebService{}
	ws.Filter(adminAuth).
		HeaderParameter("Authorization", "Admin bearer token")

	// - configs API
	ws.Route(ws.POST("/configs").To(wsUploadConfig).
		Doc("Upload a new current configuration, archiving the previous one"))

	// - clusters API
	ws.Route(ws.GET("/clusters").To(wsListClusters).
		Doc("List clusters"))

	ws.Route(ws.GET("/clusters/{cluster-name}").To(wsCluster).
		Doc("Get cluster details"))

	ws.Route(ws.GET("/clusters/{cluster-name}/addons").To(wsClusterAddons).
		Produces(mime.YAML).
		Doc("Get cluster addons").
		Returns(http.StatusOK, "OK", nil).
		Returns(http.StatusNotFound, "The cluster does not exists or does not have addons defined", nil))

	ws.Route(ws.GET("/clusters/{cluster-name}/bootstrap-pods").To(wsClusterBootstrapPods).
		Produces(mime.YAML).
		Doc("Get cluster bootstrap pods YAML definitions").
		Returns(http.StatusOK, "OK", nil).
		Returns(http.StatusNotFound, "The cluster does not exists or does not have bootstrap pods defined", nil))

	ws.Route(ws.GET("/clusters/{cluster-name}/passwords").To(wsClusterPasswords).
		Doc("List cluster's passwords"))
	ws.Route(ws.GET("/clusters/{cluster-name}/passwords/{password-name}").To(wsClusterPassword).
		Doc("Get cluster's password"))
	ws.Route(ws.PUT("/clusters/{cluster-name}/passwords/{password-name}").To(wsClusterSetPassword).
		Doc("Set cluster's password"))

	ws.Route(ws.GET("/hosts").To(wsListHosts).
		Doc("List hosts"))

	(&wsHost{
		prefix:  "/hosts/{host-name}",
		hostDoc: "given host",
		getHost: func(req *restful.Request) string {
			return req.PathParameter("host-name")
		},
	}).register(ws, func(rb *restful.RouteBuilder) {
	})

	rest.Add(ws)

	// Hosts API
	ws = &restful.WebService{}
	ws.Path("/me")
	ws.Filter(hostsAuth).
		HeaderParameter("Authorization", "Host or admin bearer token")

	(&wsHost{
		hostDoc: "detected host",
		getHost: detectHost,
	}).register(ws, func(rb *restful.RouteBuilder) {
		rb.Notes("In this case, the host is detected from the remote IP")
	})

	rest.Add(ws)
}

func detectHost(req *restful.Request) string {
	r := req.Request
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
		return ""
	}

	host := cfg.HostByIP(hostIP)

	if host == nil {
		log.Print("no host found for IP ", hostIP)
		return ""
	}

	return host.Name
}

func wsReadConfig(resp *restful.Response) *localconfig.Config {
	cfg, err := readConfig()
	if err != nil {
		log.Print("failed to read config: ", err)
		resp.WriteErrorString(http.StatusServiceUnavailable, "failed to read config")
		return nil
	}

	return cfg
}

func wsNotFound(req *restful.Request, resp *restful.Response) {
	http.NotFound(resp.ResponseWriter, req.Request)
}

func wsError(resp *restful.Response, err error) {
	log.Output(2, fmt.Sprint("request failed: ", err))
	resp.WriteErrorString(
		http.StatusInternalServerError,
		http.StatusText(http.StatusInternalServerError))
}
