//go:build !(js && wasm)

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "funda-wasm must be built with GOOS=js GOARCH=wasm")
	os.Exit(1)
}
