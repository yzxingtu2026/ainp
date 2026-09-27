package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"

	"example.com/ainp-downstream/composition"
	"github.com/yzxingtu2026/ainp/platform/framework/buildinfo"
	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
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
	db, err := database.Open(cfg.Database.DSN)
	if err != nil {
		log.Fatal(err)
	}
	app, err := host.New(host.WithModules(composition.Modules...))
	if err != nil {
		log.Fatal(err)
	}
	runner := app.Migrations()
	switch flag.Arg(0) {
	case "status":
		_, err = runner.Inspect(db)
	case "up":
		_, err = runner.Up(db)
	case "baseline":
		_, err = runner.Baseline(db)
	default:
		log.Fatal("usage: migrate [-config path] status|up|baseline")
	}
	if err != nil {
		log.Fatal(err)
	}
}
