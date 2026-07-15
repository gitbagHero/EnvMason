package main

import (
	"os"

	"github.com/gitbagHero/EnvMason/internal/buildinfo"
	"github.com/gitbagHero/EnvMason/internal/cli"
)

func main() {
	os.Exit(cli.Execute(os.Args[1:], os.Stdout, os.Stderr, buildinfo.Current()))
}
