// Not a module anyone builds, and nothing here is ours.
//
// It exists so that the parent module's ./... stops at this directory. Go walks
// every subdirectory that is not a module of its own, and 3rd-party/ holds whole
// checkouts of other people's engines - gzdoom vendors a Go file inside cppdap -
// so without this `go list ./...`, `go vet ./...` and `go test ./...` all pick up
// packages from them and report them as ours.
module thirdparty.local/not-ours

go 1.23
