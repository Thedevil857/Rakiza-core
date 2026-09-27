package handlers

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// ScanPayload represents the offline scan synced by an officer
type ScanPayload struct {
	ShipmentID string    `json:"shipmentId"`
	ShipmentName string  `json:"shipmentName"`
	OfficerID   string    `json:"officerId"`
	Lat         float64   `json:"lat"`
	Lng         float64   `json:"lng"`
	PortName    string    `json:"portName"`
	Timestamp   time.Time `json:"timestamp"`
	Status      string    `json:"status"` // e.g., "INSPECTED", "CLEARED", "FLAGGED"
}

// Hub manages active WebSocket/SSE admin connections
type MapHub struct {
	clients   map[chan ScanPayload]bool
	broadcast chan ScanPayload
	mu        sync.Mutex
}

var GlobalMapHub = MapHub{
	clients:   make(map[chan ScanPayload]bool),
	broadcast: make(chan ScanPayload),
}

// Handler for Officers syncing scans once reconnected
func SyncOfficerScansHandler(w http.ResponseWriter, r *http.Request) {
	var scans []ScanPayload
	if err := json.NewDecoder(r.Body).Decode(&scans); err != nil {
		http.Error(w, "Invalid scan payload", http.StatusBadRequest)
		return
	}

	for _, scan := range scans {
		// 1. Update Shipment record in DB (PostgreSQL / SQLite)
		// saveScanToDB(scan)

		// 2. Broadcast to connected Admin Radar Maps
		GlobalMapHub.broadcast <- scan
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"synced"}`))
}

// SSE Handler for Admin Map Real-Time Updates
func AdminMapStreamHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	clientChan := make(chan ScanPayload)
	GlobalMapHub.mu.Lock()
	GlobalMapHub.clients[clientChan] = true
	GlobalMapHub.mu.Unlock()

	defer func() {
		GlobalMapHub.mu.Lock()
		delete(GlobalMapHub.clients, clientChan)
		GlobalMapHub.mu.Unlock()
		close(clientChan)
	}()

	notify := r.Context().Done()
	for {
		select {
		case <-notify:
			return
		case scan := <-clientChan:
			data, _ := json.Marshal(scan)
			w.Write([]byte("data: " + string(data) + "\n\n"))
			w.(http.Flusher).Flush()
		}
	}
}

// Background Worker to route broadcasts
func StartMapHub() {
	go func() {
		for scan := range GlobalMapHub.broadcast {
			GlobalMapHub.mu.Lock()
			for client := range GlobalMapHub.clients {
				client <- scan
			}
			GlobalMapHub.mu.Unlock()
		}
	}()
}