package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Songxwn/maya-auto/internal/tools"
)

const ProtocolVersion = "2024-11-05"

type Executor interface {
	Connected() bool
	Call(name string, args map[string]any, wait time.Duration) (json.RawMessage, error)
}

type Request struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method"`
	Params  json.RawMessage  `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type Server struct {
	Version  string
	Executor Executor
}

func (s *Server) Handle(req Request) (resp *Response, notify bool) {
	if req.ID == nil {
		if req.Method == "notifications/initialized" || strings.HasPrefix(req.Method, "notifications/") {
			return nil, true
		}
	}
	id := json.RawMessage("null")
	if req.ID != nil {
		id = *req.ID
	}
	switch req.Method {
	case "initialize":
		return &Response{JSONRPC: "2.0", ID: id, Result: map[string]any{
			"protocolVersion": ProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": "maya-auto", "version": s.version()},
		}}, false
	case "ping":
		return &Response{JSONRPC: "2.0", ID: id, Result: map[string]any{}}, false
	case "tools/list":
		listed := make([]map[string]any, 0)
		for _, tool := range tools.All() {
			listed = append(listed, map[string]any{
				"name":        tool.Name,
				"description": tool.Description,
				"inputSchema": tool.Parameters,
			})
		}
		return &Response{JSONRPC: "2.0", ID: id, Result: map[string]any{"tools": listed}}, false
	case "tools/call":
		result, rpcErr := s.call(req.Params)
		if rpcErr != nil {
			return &Response{JSONRPC: "2.0", ID: id, Error: rpcErr}, false
		}
		return &Response{JSONRPC: "2.0", ID: id, Result: result}, false
	default:
		return &Response{JSONRPC: "2.0", ID: id, Error: &RPCError{Code: -32601, Message: "方法不存在: " + req.Method}}, false
	}
}

func (s *Server) version() string {
	if s.Version == "" {
		return "dev"
	}
	return s.Version
}

func (s *Server) call(params json.RawMessage) (any, *RPCError) {
	var in struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if len(params) > 0 {
		if err := json.Unmarshal(params, &in); err != nil {
			return nil, &RPCError{Code: -32602, Message: "参数无法解析"}
		}
	}
	if _, ok := tools.Find(in.Name); !ok {
		return nil, &RPCError{Code: -32602, Message: "未知工具: " + in.Name}
	}
	if s.Executor == nil || !s.Executor.Connected() {
		return map[string]any{
			"content": []map[string]any{{"type": "text", "text": "尚未连接 Maya"}},
			"isError": true,
		}, nil
	}
	raw, err := s.Executor.Call(in.Name, in.Arguments, tools.Timeout(in.Name))
	if err != nil {
		return map[string]any{
			"content": []map[string]any{{"type": "text", "text": err.Error()}},
			"isError": true,
		}, nil
	}
	text := string(raw)
	if len(text) > 12000 {
		text = text[:12000] + "...truncated"
	}
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
		"isError": false,
	}, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req Request
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	resp, notify := s.Handle(req)
	if notify || resp == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(resp)
}

func (s *Server) ServeStdio(in io.Reader, out io.Writer) error {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 64*1024), 2<<20)
	bw := bufio.NewWriter(out)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var req Request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			continue
		}
		resp, notify := s.Handle(req)
		if notify || resp == nil {
			continue
		}
		buf, err := json.Marshal(resp)
		if err != nil {
			return err
		}
		if _, err := bw.Write(append(buf, '\n')); err != nil {
			return err
		}
		if err := bw.Flush(); err != nil {
			return err
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return nil
}

func ProxyStdio(in io.Reader, out io.Writer, endpoint string, client *http.Client) error {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Minute}
	}
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 64*1024), 2<<20)
	bw := bufio.NewWriter(out)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader([]byte(line)))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusNoContent || len(bytes.TrimSpace(body)) == 0 {
			continue
		}
		if _, err := bw.Write(bytes.TrimSpace(body)); err != nil {
			return err
		}
		if err := bw.WriteByte('\n'); err != nil {
			return err
		}
		if err := bw.Flush(); err != nil {
			return err
		}
	}
	return sc.Err()
}

func DecodeID(raw json.RawMessage) string {
	return string(raw)
}

func FormatError(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprint(err)
}
