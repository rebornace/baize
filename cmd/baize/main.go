package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/rebornace/baize/internal/config"
	"github.com/rebornace/baize/internal/demo"
)

func main() {
	_ = config.LoadDotEnv(".env")

	if len(os.Args) < 2 {
		fmt.Println("usage: baize <start|serve>")
		os.Exit(2)
	}

	switch os.Args[1] {
	case "start":
		cfg, err := config.Load(localConfigPath())
		if err != nil {
			log.Fatal(err)
		}
		if err := demo.Run(cfg); err != nil {
			log.Fatal(err)
		}
	case "serve":
		fs := flag.NewFlagSet("serve", flag.ExitOnError)
		cfgPath := fs.String("config", localConfigPath(), "path to config yaml")
		_ = fs.Parse(os.Args[2:])
		cfg, err := config.Load(*cfgPath)
		if err != nil {
			log.Fatal(err)
		}
		if err := demo.Serve(cfg); err != nil {
			log.Fatal(err)
		}
	default:
		fmt.Println("usage: baize <start|serve>")
		os.Exit(2)
	}
}

// localConfigPath prefers local override for real-system debugging without touching CI defaults.
func localConfigPath() string {
	const local = "configs/demo.local.yaml"
	if _, err := os.Stat(local); err == nil {
		return local
	}
	return "configs/demo.yaml"
}
