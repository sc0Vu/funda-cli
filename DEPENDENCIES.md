# Go module dependency audit

The following packages are imported directly and therefore belong in the direct `require` group:

- `github.com/North-web-dev/impersonate-http` (`internal/client/client.go`)
- `github.com/klauspost/compress` (`internal/client/client.go`, Zstandard)
- `github.com/mattn/go-isatty` (`internal/app/game.go`)
- `modernc.org/sqlite` (`internal/store/sqlite.go`)

Other modules remain marked indirect because they can be required transitively. Do not delete an indirect dependency just because it does not appear in the project's own imports.

On a machine with Go 1.25 and network access, run:

```sh
go mod tidy
go test ./...
go vet ./...
git diff -- go.mod go.sum
```

The supplied archive has **not** been built or fully tidied in this environment: Go 1.23.2 is installed and Go 1.25 could not be downloaded. Dependency classifications were checked against actual source imports; indirect dependencies were retained conservatively.
