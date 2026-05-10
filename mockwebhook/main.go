package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"sync"
)

type config struct {
	mu       sync.RWMutex
	Status   int    `json:"status"`
	RespBody string `json:"response_body"`
}

var cfg = &config{Status: 200, RespBody: ""}

func configHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg.mu.RLock()
		defer cfg.mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cfg)
	case http.MethodPost:
		body, _ := io.ReadAll(r.Body)
		r.Body.Close()
		var req config
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		cfg.mu.Lock()
		if req.Status != 0 {
			cfg.Status = req.Status
		}
		if req.RespBody != "" {
			cfg.RespBody = req.RespBody
		}
		cfg.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cfg)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func webhookHandler(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	r.Body.Close()

	cfg.mu.RLock()
	status := cfg.Status
	respBody := cfg.RespBody
	cfg.mu.RUnlock()

	if respBody == "" {
		respBody = http.StatusText(status)
	}

	log.Printf("[MOCKWEBHOOK] %s %s status=%d body=%s", r.Method, r.URL.Path, status, string(body))
	w.WriteHeader(status)
	w.Write([]byte(respBody))
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/__config", configHandler)
	mux.HandleFunc("/", webhookHandler)

	log.Println("mockwebhook listening on :8888")
	if err := http.ListenAndServe(":8888", mux); err != nil {
		log.Fatal(err)
	}
}