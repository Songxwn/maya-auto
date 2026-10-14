package httpapi

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Songxwn/maya-auto/internal/agent"
	"github.com/Songxwn/maya-auto/internal/config"
	"github.com/Songxwn/maya-auto/internal/logbuf"
	"github.com/Songxwn/maya-auto/internal/maya"
	"github.com/Songxwn/maya-auto/internal/mcp"
	"github.com/Songxwn/maya-auto/internal/newapi"
	"github.com/Songxwn/maya-auto/internal/tools"
)

//go:embed web/*
var webFS embed.FS

type Server struct {
	Version  string
	Port     int
	LogPath  string
	MCPExe   string
	Config   *config.Store
	Maya     *maya.Client
	Logs     *logbuf.Buffer
	Shutdown func()
	mcp      mcp.Server
	mux      http.Handler
}

func New(s *Server) *Server {
	if s.Version == "" {
		s.Version = "dev"
	}
	s.mcp = mcp.Server{Version: s.Version, Executor: s.Maya}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/config", s.getConfig)
	mux.HandleFunc("POST /api/config", s.postConfig)
	mux.HandleFunc("POST /api/maya/connect", s.connect)
	mux.HandleFunc("POST /api/maya/disconnect", s.disconnect)
	mux.HandleFunc("GET /api/maya/status", s.status)
	mux.HandleFunc("GET /api/maya/bootstrap", s.bootstrap)
	mux.HandleFunc("POST /api/maya/call", s.call)
	mux.HandleFunc("GET /api/tools", s.toolList)
	mux.HandleFunc("POST /api/newapi/models", s.models)
	mux.HandleFunc("POST /api/newapi/probe", s.probe)
	mux.HandleFunc("POST /api/agent/run", s.runAgent)
	mux.HandleFunc("GET /api/logs", s.logs)
	mux.HandleFunc("POST /api/shutdown", s.shutdown)
	mux.HandleFunc("/mcp", s.mcp.ServeHTTP)
	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		panic(err)
	}
	mux.Handle("/", http.FileServer(http.FS(sub)))
	s.mux = mux
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"app":     "maya-auto",
		"version": s.Version,
		"port":    s.Port,
		"logPath": s.LogPath,
		"mcpExe":  s.MCPExe,
		"maya":    s.Maya.Address(),
	})
}

func (s *Server) getConfig(w http.ResponseWriter, _ *http.Request) {
	cfg := s.Config.Snapshot()
	writeJSON(w, http.StatusOK, map[string]any{
		"mayaHost":    cfg.MayaHost,
		"mayaPort":    cfg.MayaPort,
		"newAPIBase":  cfg.NewAPIBase,
		"newAPIModel": cfg.NewAPIModel,
		"hasKey":      cfg.NewAPIKey != "",
		"keyMask":     config.MaskKey(cfg.NewAPIKey),
	})
}

func (s *Server) postConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MayaHost    *string `json:"mayaHost"`
		MayaPort    *int    `json:"mayaPort"`
		NewAPIBase  *string `json:"newAPIBase"`
		NewAPIKey   *string `json:"newAPIKey"`
		NewAPIModel *string `json:"newAPIModel"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := s.Config.Update(func(f *config.File) {
		if req.MayaHost != nil && strings.TrimSpace(*req.MayaHost) != "" {
			f.MayaHost = strings.TrimSpace(*req.MayaHost)
		}
		if req.MayaPort != nil && *req.MayaPort > 0 {
			f.MayaPort = *req.MayaPort
		}
		if req.NewAPIBase != nil {
			f.NewAPIBase = strings.TrimSpace(*req.NewAPIBase)
		}
		if req.NewAPIModel != nil {
			f.NewAPIModel = strings.TrimSpace(*req.NewAPIModel)
		}
		if req.NewAPIKey != nil {
			key := strings.TrimSpace(*req.NewAPIKey)
			if key != "" && !strings.HasPrefix(key, "****") {
				f.NewAPIKey = key
			}
		}
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	s.Logs.Info("已保存配置")
	s.getConfig(w, r)
}

func (s *Server) connect(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	}
	_ = decode(r, &req)
	cfg := s.Config.Snapshot()
	host := cfg.MayaHost
	port := cfg.MayaPort
	if strings.TrimSpace(req.Host) != "" {
		host = strings.TrimSpace(req.Host)
	}
	if req.Port > 0 {
		port = req.Port
	}
	if err := s.Config.Update(func(f *config.File) {
		f.MayaHost = host
		f.MayaPort = port
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if err := s.Maya.Connect(host, port); err != nil {
		s.Logs.Error(err.Error())
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	s.Logs.Info(fmt.Sprintf("已连接 Maya %s:%d", host, port))
	s.status(w, r)
}

func (s *Server) disconnect(w http.ResponseWriter, _ *http.Request) {
	s.Maya.Close()
	s.Logs.Info("已断开 Maya")
	writeJSON(w, http.StatusOK, map[string]any{"connected": false})
}

func (s *Server) status(w http.ResponseWriter, _ *http.Request) {
	if !s.Maya.Connected() {
		writeJSON(w, http.StatusOK, map[string]any{"connected": false, "address": s.Maya.Address()})
		return
	}
	raw, err := s.Maya.Call("maya_status", map[string]any{}, tools.Timeout("maya_status"))
	if err != nil {
		s.Logs.Error(err.Error())
		writeJSON(w, http.StatusOK, map[string]any{"connected": false, "error": err.Error()})
		return
	}
	var detail any
	_ = json.Unmarshal(raw, &detail)
	writeJSON(w, http.StatusOK, map[string]any{"connected": true, "address": s.Maya.Address(), "detail": detail})
}

func (s *Server) bootstrap(w http.ResponseWriter, _ *http.Request) {
	cfg := s.Config.Snapshot()
	port := cfg.MayaPort
	if port <= 0 {
		port = 7001
	}
	script := fmt.Sprintf(`import maya.cmds as cmds
port = "127.0.0.1:%d"
try:
    if not cmds.commandPort(port, query=True):
        cmds.commandPort(name=port, sourceType="mel", echoOutput=False)
    print("maya-auto commandPort %%s" %% port)
except Exception as exc:
    print("maya-auto commandPort failed: %%s" %% exc)
    raise
`, port)
	writeJSON(w, http.StatusOK, map[string]any{"port": port, "script": script})
}

func (s *Server) call(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string         `json:"name"`
		Args map[string]any `json:"args"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if _, ok := tools.Find(req.Name); !ok {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("未知工具 %s", req.Name))
		return
	}
	if !s.Maya.Connected() {
		writeErr(w, http.StatusConflict, fmt.Errorf("尚未连接 Maya"))
		return
	}
	s.Logs.Info("工具 " + req.Name)
	raw, err := s.Maya.Call(req.Name, req.Args, tools.Timeout(req.Name))
	if err != nil {
		s.Logs.Error(req.Name + ": " + trimErr(err))
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	var result any
	if err := json.Unmarshal(raw, &result); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"result": string(raw)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": result})
}

func (s *Server) toolList(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"tools": tools.All()})
}

func (s *Server) models(w http.ResponseWriter, r *http.Request) {
	client := s.client()
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	listed, err := client.ListModels(ctx)
	if err != nil {
		s.Logs.Error("NewAPI 模型列表: " + trimErr(err))
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	s.Logs.Info(fmt.Sprintf("NewAPI 模型 %d 个，耗时 %dms", len(listed.Models), listed.LatencyMS))
	writeJSON(w, http.StatusOK, listed)
}

func (s *Server) probe(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Models []string `json:"models"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	models := make([]string, 0, len(req.Models))
	seen := map[string]bool{}
	for _, name := range req.Models {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		models = append(models, name)
		if len(models) >= 60 {
			break
		}
	}
	if len(models) == 0 {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("没有要探测的模型"))
		return
	}
	prepareSSE(w)
	flusher, _ := w.(http.Flusher)
	var mu sync.Mutex
	client := s.client()
	sem := make(chan struct{}, 3)
	var wg sync.WaitGroup
	for _, model := range models {
		if r.Context().Err() != nil {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(model string) {
			defer wg.Done()
			defer func() { <-sem }()
			ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
			defer cancel()
			result := client.Probe(ctx, model)
			writeSSE(w, flusher, &mu, "probe", result)
		}(model)
	}
	wg.Wait()
	writeSSE(w, flusher, &mu, "done", map[string]any{"count": len(models)})
	s.Logs.Info(fmt.Sprintf("已探测 %d 个模型", len(models)))
}

func (s *Server) runAgent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Prompt string `json:"prompt"`
		Model  string `json:"model"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	cfg := s.Config.Snapshot()
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = cfg.NewAPIModel
	}
	prepareSSE(w)
	flusher, _ := w.(http.Flusher)
	var mu sync.Mutex
	err := agent.Run(r.Context(), s.client(), model, req.Prompt, func(name string, args map[string]any, wait time.Duration) (json.RawMessage, error) {
		if !s.Maya.Connected() {
			return nil, fmt.Errorf("尚未连接 Maya")
		}
		s.Logs.Info("指令工具 " + name)
		return s.Maya.Call(name, args, wait)
	}, func(ev agent.Event) {
		writeSSE(w, flusher, &mu, ev.Kind, ev)
	})
	if err != nil {
		s.Logs.Error("指令: " + trimErr(err))
		writeSSE(w, flusher, &mu, "error", map[string]any{"error": err.Error()})
	}
}

func (s *Server) logs(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"lines": s.Logs.Snapshot()})
}

func (s *Server) shutdown(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	if s.Shutdown != nil {
		go s.Shutdown()
	}
}

func (s *Server) client() *newapi.Client {
	cfg := s.Config.Snapshot()
	return &newapi.Client{BaseURL: cfg.NewAPIBase, APIKey: cfg.NewAPIKey, HTTP: &http.Client{}}
}

func decode(r *http.Request, dest any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 2<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dest); err != nil {
		if err == io.EOF {
			return nil
		}
		return fmt.Errorf("请求不是有效 JSON")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeErr(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func prepareSSE(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
}

func writeSSE(w http.ResponseWriter, flusher http.Flusher, mu *sync.Mutex, event string, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
	if flusher != nil {
		flusher.Flush()
	}
}

func trimErr(err error) string {
	text := err.Error()
	if len(text) > 400 {
		return text[:400] + "..."
	}
	return text
}

func MCPExePath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(exe), "maya-auto-mcp.exe")
}
