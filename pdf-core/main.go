package main

import (
	"bufio"
	"log"
	"net/http"
	"os"
	"strings"

	compiler "example.com/m/V2/compiler"
)

// loadEnvFile reads KEY=VALUE pairs from path and sets them in the environment.
// Lines starting with # and blank lines are ignored.
// Missing file is silently ignored.
func loadEnvFile(path string) error {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if err := os.Setenv(strings.TrimSpace(key), strings.TrimSpace(value)); err != nil {
			return err
		}
	}
	return scanner.Err()
}

// applyEnvToCompiler re-initialises the compiler's ontology IRI after .env
// has been loaded, so DCS_PDF_CORE_ONTOLOGY_BASE_URL set in .env takes effect
// even though the compiler's init() runs before main() loads the file.
func applyEnvToCompiler() {
	compiler.InitOntologyIRI(os.Getenv("DCS_PDF_CORE_ONTOLOGY_BASE_URL"))
}

func main() {
	if err := loadEnvFile(".env"); err != nil {
		log.Fatalf("load .env: %v", err)
	}
	applyEnvToCompiler()

	addr := os.Getenv("DCS_PDF_CORE_ADDR")
	if addr == "" {
		addr = "0.0.0.0:8080"
	}
	server := &http.Server{
		Addr:    addr,
		Handler: newServer(),
	}

	log.Printf("DCS-PDF-CORE listening on %s", server.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
