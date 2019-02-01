package main

import (
	"io"
	"math/rand"
	"time"

	ulidp "github.com/oklog/ulid"
)

var (
	ulidCtx struct{ entropy io.Reader }
)

func initUlid() {
	entropy := ulidp.Monotonic(rand.New(rand.NewSource(time.Now().UnixNano())), 0)
	ulidCtx.entropy = entropy
}

func ulid() string {
	return ulidp.MustNew(ulidp.Now(), ulidCtx.entropy).String()
}
