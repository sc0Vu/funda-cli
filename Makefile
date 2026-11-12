lint:
	go fmt ./...
	go vet ./...
build:
	go build ./cmd/funda
test:
	go test ./...

.PHONY: wasm
wasm:
	GOOS=js GOARCH=wasm go build -o site/funda.wasm ./cmd/funda-wasm
	GOROOT=$$(go env GOROOT); src=$$GOROOT/lib/wasm/wasm_exec.js; [ -f $$src ] || src=$$GOROOT/misc/wasm/wasm_exec.js; cp $$src site/wasm_exec.js
