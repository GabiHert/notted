package infra

// Version is the running release, set at build time with
// -ldflags "-X github.com/GabiHert/phonon/internal/infra.Version=v1.2.3" (make build VERSION=…).
// A build not cut from a release tag counts as older than every release.
var Version = "dev"
