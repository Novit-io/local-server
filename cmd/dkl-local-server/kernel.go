package main

import (
	"log"
	"net/http"
)

func renderKernel(w http.ResponseWriter, r *http.Request, ctx *renderContext) error {
	path, err := ctx.distFetch("kernels", ctx.Group.Kernel)
	if err != nil {
		return err
	}

	log.Printf("sending kernel %s for %q", ctx.Group.Kernel, ctx.Host.Name)
	http.ServeFile(w, r, path)
	return nil
}
