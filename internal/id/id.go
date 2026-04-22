package id

import (
	gonanoid "github.com/jaevor/go-nanoid"
)

var generate func() string

func init() {
	var err error
	generate, err = gonanoid.Standard(12)
	if err != nil {
		panic("failed to init nanoid: " + err.Error())
	}
}

// New returns a URL-safe random ID (12 chars, ~71 bits of entropy).
func New() string {
	return generate()
}
