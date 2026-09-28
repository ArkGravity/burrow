package burrow

import "net/http"

// Metadata and UserInfo are fetched cross-origin by registered SPA clients.
// UserInfo additionally binds the token to its own client's origin in storage.
func (b *Server) readCORS(w http.ResponseWriter, r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin != "" && origin != b.Config.Issuer {
		var apps []Application
		if err := b.DB.Where("enabled = ? AND client_type = ?", true, "spa").Find(&apps).Error; err != nil {
			write(w, 503, map[string]string{"error": "server_error"})
			return false
		}
		allowed := false
		for _, app := range apps {
			if contains(app.Origins, origin) {
				allowed = true
				break
			}
		}
		if !allowed {
			write(w, 403, map[string]string{"error": "invalid_request"})
			return false
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	}
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusNoContent)
		return false
	}
	return true
}
