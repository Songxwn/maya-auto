package mcp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeMaya struct {
	connected bool
	last      string
}

func (f *fakeMaya) Connected() bool { return f.connected }

func (f *fakeMaya) Call(name string, args map[string]any, _ time.Duration) (json.RawMessage, error) {
	f.last = name
	b, _ := json.Marshal(map[string]any{"tool": name, "args": args})
	return b, nil
}

func TestInitializeAndCall(t *testing.T) {
	fake := &fakeMaya{connected: true}
	s := &Server{Version: "test", Executor: fake}
	var in, out bytes.Buffer
	in.WriteString(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n")
	in.WriteString(`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n")
	in.WriteString(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}` + "\n")
	in.WriteString(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"maya_primitive","arguments":{"kind":"cube"}}}` + "\n")
	if err := s.ServeStdio(&in, &out); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("lines=%d %s", len(lines), out.String())
	}
	if !strings.Contains(lines[2], "maya_primitive") || fake.last != "maya_primitive" {
		t.Fatal(out.String())
	}
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":7,"method":"ping"}`))
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"id":7`) {
		t.Fatal(rr.Code, rr.Body.String())
	}
}
