package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/rebornace/baize/internal/bootstrap"
	"github.com/rebornace/baize/internal/config"
)

func main() {
	_ = config.LoadDotEnv(".env")

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "start":
		cfgPath := startConfigPath()
		cfg, err := config.Load(cfgPath)
		if err != nil {
			log.Fatal(err)
		}
		if err := config.ValidateStart(cfg); err != nil {
			log.Fatal(err)
		}
		log.Printf("baize start: config=%s agent=%s llm=%s", cfgPath, cfg.Agent.ID, cfg.LLM.Provider)
		if err := bootstrap.Serve(cfg); err != nil {
			log.Fatal(err)
		}
	case "demo":
		cfgPath := demoConfigPath()
		cfg, err := config.Load(cfgPath)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("baize demo: config=%s agent=%s llm=%s", cfgPath, cfg.Agent.ID, cfg.LLM.Provider)
		if err := bootstrap.Run(cfg); err != nil {
			log.Fatal(err)
		}
	case "serve":
		fs := flag.NewFlagSet("serve", flag.ExitOnError)
		cfgPath := fs.String("config", startConfigPath(), "path to config yaml")
		_ = fs.Parse(os.Args[2:])
		cfg, err := config.Load(*cfgPath)
		if err != nil {
			log.Fatal(err)
		}
		if err := bootstrap.Serve(cfg); err != nil {
			log.Fatal(err)
		}
	default:
		printUsage()
		os.Exit(2)
	}
}

func printUsage() {
	fmt.Println("usage: baize <start|demo|serve>")
	fmt.Println("  start  clean Runtime (configs/minimal.yaml); requires BAIZE_API_KEY")
	fmt.Println("  demo   trial stack with mock LLM + demo HTTP (configs/demo.yaml)")
	fmt.Println("  serve  Runtime only with explicit -config")
}

func startConfigPath() string {
	const local = "configs/minimal.local.yaml"
	if _, err := os.Stat(local); err == nil {
		return local
	}
	return "configs/minimal.yaml"
}

func demoConfigPath() string {
	const local = "configs/demo.local.yaml"
	if _, err := os.Stat(local); err == nil {
		return local
	}
	return "configs/demo.yaml"
}
