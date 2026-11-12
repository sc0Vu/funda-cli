//go:build js && wasm

// Command funda-wasm exposes the offline demo engine to JavaScript.
package main

import (
	"syscall/js"

	"github.com/sc0vu/funda-cli/internal/webdemo"
)

func main() {
	js.Global().Set("fundaExec", js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) == 0 {
			return ""
		}
		return webdemo.Exec(args[0].String())
	}))
	js.Global().Set("fundaReady", true)
	select {}
}
