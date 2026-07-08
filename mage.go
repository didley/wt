//go:build ignore

// Zero-install entry point for the mage targets in magefile.go:
//
//	go run mage.go -l
//	go run mage.go build
package main

import (
	"os"

	"github.com/magefile/mage/mage"
)

func main() { os.Exit(mage.Main()) }
