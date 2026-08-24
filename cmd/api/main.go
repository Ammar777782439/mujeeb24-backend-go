package main

import (
	"log"
	"net/http"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/contract"
)

func main() {
	_, mux := contract.BuildAPI()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","service":"mujeeb24-api"}`))
	})

	server := &http.Server{Addr: ":3001", Handler: mux}
	log.Println("Mujeeb 24 API listening on :3001")
	log.Fatal(server.ListenAndServe())
}
