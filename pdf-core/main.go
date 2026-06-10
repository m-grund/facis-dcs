package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
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
