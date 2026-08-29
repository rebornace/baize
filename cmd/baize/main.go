package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/rebornace/baize/internal/bootstrap"
	"github.com/rebornace/baize/internal/config"

	// Register weixin Channel factory (Start/Runtime wiring in Task 6).
	_ "github.com/rebornace/baize/internal/channel/weixin"
)

func main() {
	_ = config.LoadDotEnv(".env")

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "start":
		basePath := "configs/minimal.yaml"
		cfgPath := startConfigPath()
		cfg, err := config.Load(cfgPath)
		if err != nil {
			log.Fatal(err)
		}
		if err := config.ValidateStart(cfg); err != nil {
			log.Fatal(err)
		}
		log.Printf("baize start: config=%s agent=%s llm=%s", cfgPath, cfg.Agent.ID, cfg.LLM.Provider)
		if err := bootstrap.Serve(cfg, basePath); err != nil {
			log.Fatal(err)
		}
	case "demo":
		cfg, cfgPaths, err := loadDemoConfig()
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("baize demo: config=%v agent=%s llm=%s", cfgPaths, cfg.Agent.ID, cfg.LLM.Provider)
		warnIfLLMKeyMissing(cfg)
		if err := bootstrap.Run(cfg, "configs/demo.yaml"); err != nil {
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
		if err := bootstrap.Serve(cfg, *cfgPath); err != nil {
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
	fmt.Println("  demo   trial stack (configs/demo.yaml + optional default.local.yaml / demo.local.yaml)")
	fmt.Println("  serve  Runtime only with explicit -config")
}

func startConfigPath() string {
	const local = "configs/minimal.local.yaml"
	if _, err := os.Stat(local); err == nil {
		return local
	}
	return "configs/minimal.yaml"
}

func loadDemoConfig() (config.Config, []string, error) {
	return config.LoadLayered(
		"configs/demo.yaml",
		"configs/default.local.yaml",
		"configs/demo.local.yaml",
	)
}

func warnIfLLMKeyMissing(cfg config.Config) {
	prov := strings.ToLower(strings.TrimSpace(cfg.LLM.Provider))
	if prov != "openai_compatible" {
		return
	}
	env := cfg.LLM.APIKeyEnv
	if env == "" {
		env = "BAIZE_API_KEY"
	}
	if strings.TrimSpace(os.Getenv(env)) == "" {
		log.Printf("warning: %s is not set; real LLM calls will fail (set it in .env)", env)
	}
}
