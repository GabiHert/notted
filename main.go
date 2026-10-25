package main

import (
	"embed"
	"log"

	"github.com/GabiHert/notted/internal/infra"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	if err := infra.Run(assets); err != nil {
		log.Fatal(err)
	}
}
