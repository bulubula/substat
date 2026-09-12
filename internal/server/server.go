package server

import (
	"encoding/json"
	"html/template"
	"io/fs"
	"net/http"

	"bulubula/substat/internal/config"
	"bulubula/substat/internal/scheduler"
	"bulubula/substat/internal/store"
	"bulubula/substat/web"
)

type Server struct {
	cfg       *config.Config
	sched     *scheduler.Scheduler
	store     *store.Store
	indexTmpl *template.Template
}

func NewServer(cfg *config.Config, sched *scheduler.Scheduler, s *store.Store) (*Server, error) {
	tmplContent, err := fs.ReadFile(web.Files, "index.html")
	if err != nil {
		return nil, err
	}
	tmpl, err := template.New("index").Parse(string(tmplContent))
	if err != nil {
		return nil, err
	}

	return &Server{
		cfg:       cfg,
		sched:     sched,
		store:     s,
		indexTmpl: tmpl,
	}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// 1. API endpoint
	mux.HandleFunc("/api/status", s.handleAPIStatus)

	// 2. Static asset endpoints
	subFS, _ := fs.Sub(web.Files, ".")
	staticHandler := http.StripPrefix("/static/", http.FileServer(http.FS(subFS)))
	mux.Handle("/static/", staticHandler)

	// 3. Index page
	mux.HandleFunc("/", s.handleIndex)

	// Subpath wrapping
	basePath := s.cfg.Server.BasePath
	if basePath == "" || basePath == "/" {
		return mux
	}

	rootMux := http.NewServeMux()
	// Strip the basePath so internal router only sees "/"
	rootMux.Handle(basePath+"/", http.StripPrefix(basePath, mux))
	rootMux.HandleFunc(basePath, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, basePath+"/", http.StatusMovedPermanently)
	})

	return rootMux
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	data := map[string]string{
		"BasePath": s.cfg.Server.BasePath,
	}
	if data["BasePath"] == "" {
		data["BasePath"] = "."
	}
	_ = s.indexTmpl.Execute(w, data)
}

type MonitorResponse struct {
	scheduler.MonitorState
	History []store.Point `json:"history"`
}

func (s *Server) handleAPIStatus(w http.ResponseWriter, r *http.Request) {
	states := s.sched.GetStates()
	resp := make([]MonitorResponse, len(states))

	for i, st := range states {
		resp[i] = MonitorResponse{
			MonitorState: st,
			History:      s.store.GetRecent(st.Name),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) ListenAndServe() error {
	srv := &http.Server{
		Addr:    s.cfg.Server.Listen,
		Handler: s.Handler(),
	}
	return srv.ListenAndServe()
}
