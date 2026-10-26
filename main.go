package main

import (
	"embed"
	"log"
	"os"

	"github.com/GabiHert/notted/internal/infra"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	if len(os.Args) > 1 && os.Args[1] == "mcp" {
		os.Exit(infra.RunMCP(os.Args[2:]))
	}
	if err := infra.Run(assets); err != nil {
		log.Fatal(err)
	}
}
