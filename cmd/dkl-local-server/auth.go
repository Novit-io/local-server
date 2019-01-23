package main

import (
	"flag"
	"log"
	"net/http"
)

var (
	hostsToken = flag.String("hosts-token", "", "Token to give to access /hosts (open is none)")
	adminToken = flag.String("admin-token", "", "Token to give to access to admin actions (open is none)")
)

func authorizeHosts(r *http.Request) bool {
	return authorizeToken(r, *hostsToken)
}

func authorizeAdmin(r *http.Request) bool {
	return authorizeToken(r, *adminToken)
}

func authorizeToken(r *http.Request, token string) bool {
	if token == "" {
		// access is open
		return true
	}

	reqToken := r.Header.Get("Authorization")

	return reqToken == "Bearer "+token
}

func forbidden(w http.ResponseWriter, r *http.Request) {
	log.Printf("denied access to %s from %s", r.RequestURI, r.RemoteAddr)
	http.Error(w, "Forbidden", http.StatusForbidden)
}
