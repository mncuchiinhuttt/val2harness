package proxy

import (
	"encoding/json"
	"net/http"
	"time"
)

type openAIModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type openAIModelsResponse struct {
	Object string        `json:"object"`
	Data   []openAIModel `json:"data"`
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":{"message":"Method not allowed"}}`, http.StatusMethodNotAllowed)
		return
	}

	models, err := s.client.ListModels(r.Context())
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"message": "Failed to fetch models from RMIT Val: " + err.Error(),
				"type":    "upstream_error",
			},
		})
		return
	}

	now := time.Now().Unix()
	resp := openAIModelsResponse{
		Object: "list",
		Data:   make([]openAIModel, 0, len(models)),
	}

	for _, m := range models {
		ownedBy := "rmit-val"
		if m.OwnedBy != "" {
			ownedBy = m.OwnedBy
		}
		resp.Data = append(resp.Data, openAIModel{
			ID:      m.ID,
			Object:  "model",
			Created: now,
			OwnedBy: ownedBy,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}
