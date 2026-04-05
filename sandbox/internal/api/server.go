// Package api provides REST and MCP API handlers for the sandbox.
package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/redis-fs/sandbox/internal/executor"
)

// Server handles HTTP requests for the sandbox.
type Server struct {
	manager *executor.Manager
	router  *mux.Router
}

// NewServer creates a new API server.
func NewServer(manager *executor.Manager) *Server {
	s := &Server{manager: manager, router: mux.NewRouter()}
	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	s.router.HandleFunc("/health", s.handleHealth).Methods("GET")
	s.router.HandleFunc("/processes", s.handleLaunch).Methods("POST")
	s.router.HandleFunc("/processes", s.handleList).Methods("GET")
	s.router.HandleFunc("/processes/{id}", s.handleRead).Methods("GET")
	s.router.HandleFunc("/processes/{id}/write", s.handleWrite).Methods("POST")
	s.router.HandleFunc("/processes/{id}/wait", s.handleWait).Methods("POST")
	s.router.HandleFunc("/processes/{id}", s.handleKill).Methods("DELETE")
}

// Handler returns the HTTP handler.
func (s *Server) Handler() http.Handler {
	return s.router
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// LaunchRequest is the JSON body for launching a process.
type LaunchRequest struct {
	Command       string `json:"command"`
	Cwd           string `json:"cwd,omitempty"`
	TimeoutSecs   int    `json:"timeout_secs,omitempty"`
	Wait          bool   `json:"wait"`
	KeepStdinOpen bool   `json:"keep_stdin_open,omitempty"`
}

func (s *Server) handleLaunch(w http.ResponseWriter, r *http.Request) {
	var req LaunchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	opts := executor.LaunchOptions{
		Command:       req.Command,
		Cwd:           req.Cwd,
		Wait:          req.Wait,
		KeepStdinOpen: req.KeepStdinOpen,
	}
	if req.TimeoutSecs > 0 {
		opts.Timeout = time.Duration(req.TimeoutSecs) * time.Second
	}

	result, err := s.manager.Launch(r.Context(), opts)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	processes := s.manager.List()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(processes)
}

func (s *Server) handleRead(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	result, err := s.manager.Read(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// WriteRequest is the JSON body for writing to stdin.
type WriteRequest struct {
	Input string `json:"input"`
}

func (s *Server) handleWrite(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var req WriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.manager.Write(id, req.Input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleWait(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	result, err := s.manager.Wait(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleKill(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if err := s.manager.Kill(id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "killed"})
}

