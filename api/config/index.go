package handler

import (
	"encoding/json"
	"net/http"
	"os"
	"retro-tool-vercel/internal/cors"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	if cors.Preflight(w, r) {
		return
	}
	cors.Set(w)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"supabase_url":      os.Getenv("SUPABASE_URL"),
		"supabase_anon_key": os.Getenv("SUPABASE_ANON_KEY"),
	})
}
