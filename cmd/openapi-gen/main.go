package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/contract"
)

func main() {
	output := flag.String("output", "api/openapi/mujeeb24-dashboard-v1.generated.yaml", "generated OpenAPI YAML output path")
	flag.Parse()

	api, _ := contract.BuildAPI()
	document, err := api.OpenAPI().DowngradeYAML()
	if err != nil {
		log.Fatalf("generate OpenAPI: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(*output), 0o755); err != nil {
		log.Fatalf("create output directory: %v", err)
	}
	if err := os.WriteFile(*output, document, 0o644); err != nil {
		log.Fatalf("write OpenAPI: %v", err)
	}
	log.Printf("generated OpenAPI: %s", *output)
}
