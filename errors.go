package main

import (
	"fmt"
	"os"
)

type notFoundError struct {
	ref string
}

func (e notFoundError) Error() string {
	return fmt.Sprintf("not found: %s", e.ref)
}

var _ error = notFoundError{}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}

	if os.IsNotExist(err) {
		return true
	}

	_, ok := err.(notFoundError)
	return ok
}
