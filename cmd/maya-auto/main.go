package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Songxwn/maya-auto/internal/config"
	"github.com/Songxwn/maya-auto/internal/httpapi"
	"github.com/Songxwn/maya-auto/internal/logbuf"
	"github.com/Songxwn/maya-auto/internal/maya"
	"github.com/Songxwn/maya-auto/internal/mcp"
)

var version = "dev"

func main() {
	if mcpMode() {
		if err := runMCP(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := runGUI(); err != nil {
		fatal(err)
	}
}

func mcpMode() bool {
	exe, _ := os.Executable()
	base := filepath.Base(exe)
	if len(base) >= 3 && (containsFold(base, "mcp")) {
		return true
	}
	for _, arg := range os.Args[1:] {
		if arg == "--mcp" || arg == "--stdio" {
			return true
		}
	}
	return false
}

func containsFold(s, part string) bool {
	return len(s) >= len(part) && (indexFold(s, part) >= 0)
}

func indexFold(s, part string) int {
	for i := 0; i+len(part) <= len(s); i++ {
		ok := true
		for j := 0; j < len(part); j++ {
			a, b := s[i+j], part[j]
			if a >= 'A' && a <= 'Z' {
				a += 'a' - 'A'
			}
			if b >= 'A' && b <= 'Z' {
				b += 'a' - 'A'
			}
			if a != b {
				ok = false
				break
			}
		}
		if ok {
			return i
		}
	}
	return -1
}

func runMCP() error {
	if target := findExisting(); target != "" {
		return mcp.ProxyStdio(os.Stdin, os.Stdout, target+"mcp", nil)
	}
	store, err := config.Load()
	if err != nil {
		return err
	}
	client := maya.New()
	cfg := store.Snapshot()
	if err := client.Connect(cfg.MayaHost, cfg.MayaPort); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
	}
	defer client.Close()
	srv := mcp.Server{Version: version, Executor: client}
	return srv.ServeStdio(os.Stdin, os.Stdout)
}

func runGUI() error {
	if target := findExisting(); target != "" {
		return openBrowser(target)
	}
	store, err := config.Load()
	if err != nil {
		return err
	}
	dir, err := config.Dir()
	if err != nil {
		return err
	}
	logs := logbuf.New(filepath.Join(dir, "maya-auto.log"), 400)
	defer logs.Close()
	client := maya.New()
	defer client.Close()

	ln, port, err := listen(47321)
	if err != nil {
		return err
	}
	var once sync.Once
	done := make(chan struct{})
	stop := func() { once.Do(func() { close(done) }) }

	api := httpapi.New(&httpapi.Server{
		Version:  version,
		Port:     port,
		LogPath:  filepath.Join(dir, "maya-auto.log"),
		MCPExe:   httpapi.MCPExePath(),
		Config:   store,
		Maya:     client,
		Logs:     logs,
		Shutdown: stop,
	})
	httpServer := &http.Server{Handler: api}
	go func() {
		if err := httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			logs.Error(err.Error())
		}
	}()
	url := fmt.Sprintf("http://127.0.0.1:%d/", port)
	logs.Info("界面 " + url)
	cmd, err := openAppWindow(url)
	if err != nil {
		logs.Error(err.Error())
		_ = openBrowser(url)
	}
	if cmd != nil {
		go func() {
			started := time.Now()
			_ = cmd.Wait()
			if time.Since(started) < 3*time.Second {
				logs.Info("图形窗口进程提前退出，继续保留服务，可在界面里点退出")
				return
			}
			stop()
		}()
	}
	<-done
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(ctx)
	return nil
}

func listen(start int) (net.Listener, int, error) {
	var last error
	for port := start; port < start+20; port++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			return ln, port, nil
		}
		last = err
	}
	if last == nil {
		last = fmt.Errorf("没有可用端口")
	}
	return nil, 0, last
}

func findExisting() string {
	client := &http.Client{Timeout: 400 * time.Millisecond}
	for port := 47321; port < 47341; port++ {
		url := fmt.Sprintf("http://127.0.0.1:%d/api/health", port)
		resp, err := client.Get(url)
		if err != nil {
			continue
		}
		var body struct {
			OK  bool   `json:"ok"`
			App string `json:"app"`
		}
		_ = decodeJSON(resp, &body)
		resp.Body.Close()
		if body.OK && body.App == "maya-auto" {
			return fmt.Sprintf("http://127.0.0.1:%d/", port)
		}
	}
	return ""
}

func fatal(err error) {
	if err == nil {
		return
	}
	messageBox("Maya Auto", err.Error())
	os.Exit(1)
}
