package health

import (
	"encoding/json"
	"net/http"
)

type Handler struct {
	collector *Collector
}

func NewHandler(collector *Collector) *Handler {
	return &Handler{collector: collector}
}

func (h *Handler) GetAll(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, h.collector.Current())
}

func (h *Handler) GetCPU(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, h.collector.Current().CPU)
}

func (h *Handler) GetMemory(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, h.collector.Current().Memory)
}

func (h *Handler) GetDisk(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, h.collector.Current().Disk)
}

func (h *Handler) GetNetwork(w http.ResponseWriter, r *http.Request) {
	type response struct {
		Current NetworkStats          `json:"current"`
		History []NetworkHistoryEntry `json:"history"`
	}
	stats := h.collector.Current()
	respondJSON(w, response{
		Current: stats.Network,
		History: h.collector.NetworkHistory(),
	})
}

func (h *Handler) GetGPU(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, h.collector.Current().GPU)
}

func (h *Handler) GetDocker(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, h.collector.Current().Docker)
}

func (h *Handler) GetSystem(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, h.collector.Current().System)
}

func respondJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
