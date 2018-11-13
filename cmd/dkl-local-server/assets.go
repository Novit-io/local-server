package main

import "log"

func asset(name string) []byte {
	ba, err := assets.Find(name)
	if err != nil {
		log.Fatalf("asset find error for %q: %v", name, err)
	}
	return ba
}
