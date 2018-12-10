package main

import (
	"io"
	"log"
)

func renderIPXE(out io.Writer, ctx *renderContext) error {
	log.Printf("sending IPXE code for %q", ctx.Host.Name)

	_, err := out.Write([]byte(ctx.Host.IPXE))
	return err
}
