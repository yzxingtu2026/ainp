package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"

	"example.com/ainp-downstream/composition"
	"github.com/yzxingtu2026/ainp/platform/framework/buildinfo"
	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/host"
)

func main() {
	version := flag.Bool("version", false, "print immutable build identity")
	path := flag.String("config", "configs/config.yaml", "application config")
	flag.Parse()
	if *version {
		_ = json.NewEncoder(os.Stdout).Encode(buildinfo.Current())
		return
	}
	cfg, err := config.Load(*path)
	if err != nil {
		log.Fatal(err)
	}
	p, err := host.OpenAPI(context.Background(), cfg, host.WithModules(composition.Modules...))
	if err != nil {
		log.Fatal(err)
	}
	defer p.Close()
	if err := p.App.Run(); err != nil {
		log.Fatal(err)
	}
}
