package main

import (
	"log"
	"net/http"
)

func main() {
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","service":"mujeeb24-api"}`))
	})

	log.Println("Mujeeb 24 API listening on :3001")
	log.Fatal(http.ListenAndServe(":3001", nil))
}
