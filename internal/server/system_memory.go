package server

import (
	"log"
	"net/http"
)

func (s *Server) handleMemoryStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Cache-Control", "no-store")

	stats, err := s.readSystemStats()
	if err != nil {
		log.Printf("memory status error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to read memory status"})
		return
	}

	writeJSON(w, http.StatusOK, stats)
}
