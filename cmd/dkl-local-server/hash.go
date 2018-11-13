package main

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
)

func hash(values ...interface{}) string {
	ba, err := json.Marshal(values)
	if err != nil {
		panic(err) // should not happen
	}

	h := sha1.Sum(ba)

	enc := base64.StdEncoding.WithPadding(base64.NoPadding)
	return enc.EncodeToString(h[:])
}
