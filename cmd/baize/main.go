package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/rebornace/baize/internal/bootstrap"
	"github.com/rebornace/baize/internal/config"

	// Register redis middleware driver (Streams / PubSub / Lua limiter).
	_ "github.com/rebornace/baize/internal/middleware/redis"
	// Register s3 blob driver (S3/MinIO/OSS/COS via minio-go).
	_ "github.com/rebornace/baize/internal/blob/s3"
)

func main() {
	_ = config.LoadDotEnv(".env")

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "serve":
		fs := flag.NewFlagSet("serve", flag.ExitOnError)
		cfgPath := fs.String("config", defaultConfigPath, "path to config yaml")
		_ = fs.Parse(os.Args[2:])
		cfg, err := config.Load(*cfgPath)
		if err != nil {
			log.Fatal(err)
		}
		if err := config.Validate(cfg); err != nil {
			log.Fatal(err)
		}
		log.Printf("baize serve: config=%s agent=%s llm=%s", *cfgPath, cfg.Agent.ID, cfg.LLM.Provider)
		if err := bootstrap.Serve(cfg, *cfgPath); err != nil {
			log.Fatal(err)
		}
	case "reset-credentials":
		if err := runResetCredentials(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
	default:
		printUsage()
		os.Exit(2)
	}
}

const defaultConfigPath = "configs/config.yaml"

func printUsage() {
	fmt.Println("usage: baize <serve|reset-credentials>")
	fmt.Println("  serve              run the Runtime (configs/config.yaml; override with -config)")
	fmt.Println("  reset-credentials  clear hot-updated control-plane tokens (fall back to YAML break-glass)")
}
