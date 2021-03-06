package main

import (
	"flag"
	"log"
	"net/http"
	"path/filepath"

	restful "github.com/emicklei/go-restful"
	"github.com/mcluseau/go-swagger-ui"
	"novit.nc/direktil/pkg/cas"

	"novit.nc/direktil/local-server/pkg/apiutils"
)

const (
	etcDir = "/etc/direktil"
)

var (
	address    = flag.String("address", ":7606", "HTTP listen address")
	tlsAddress = flag.String("tls-address", "", "HTTPS listen address")
	certFile   = flag.String("tls-cert", etcDir+"/server.crt", "Server TLS certificate")
	keyFile    = flag.String("tls-key", etcDir+"/server.key", "Server TLS key")

	casStore cas.Store
)

func main() {
	flag.Parse()

	if *address == "" && *tlsAddress == "" {
		log.Fatal("no listen address given")
	}

	casStore = cas.NewDir(filepath.Join(*dataDir, "cache"))
	go casCleaner()

	apiutils.Setup(func() {
		registerWS(restful.DefaultContainer)
	})

	swaggerui.HandleAt("/swagger-ui/")

	if *address != "" {
		log.Print("HTTP listening on ", *address)
		go log.Fatal(http.ListenAndServe(*address, nil))
	}

	if *tlsAddress != "" {
		log.Print("HTTPS listening on ", *tlsAddress)
		go log.Fatal(http.ListenAndServeTLS(*tlsAddress, *certFile, *keyFile, nil))
	}

	select {}
}
