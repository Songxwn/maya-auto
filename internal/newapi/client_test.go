package newapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResolve(t *testing.T) {
	if got := Resolve("https://gw.example/v1/", "/models"); got != "https://gw.example/v1/models" {
		t.Fatal(got)
	}
	if got := Resolve("https://gw.example", "chat/completions"); got != "https://gw.example/v1/chat/completions" {
		t.Fatal(got)
	}
}

func TestListAndProbe(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			w.WriteHeader(401)
			return
		}
		_, _ = io.WriteString(w, `{"data":[{"id":"b-model","owned_by":"lab"},{"id":"a-model","owned_by":"lab"},{"id":"a-model"}]}`)
	})
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		model, _ := body["model"].(string)
		if model == "retry-model" {
			if _, ok := body["max_tokens"]; ok {
				w.WriteHeader(400)
				_, _ = io.WriteString(w, `{"error":{"message":"use max_completion_tokens instead of max_tokens"}}`)
				return
			}
		}
		if model == "missing" {
			w.WriteHeader(404)
			_, _ = io.WriteString(w, `{"error":{"message":"model_not_found"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"OK"}}]}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, APIKey: "secret", HTTP: srv.Client()}
	listed, err := c.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Models) != 2 || listed.Models[0].ID != "a-model" {
		t.Fatalf("%+v", listed.Models)
	}
	ok := c.Probe(context.Background(), "a-model")
	if !ok.OK || ok.Reply != "OK" || ok.Class != "ok" {
		t.Fatalf("%+v", ok)
	}
	missing := c.Probe(context.Background(), "missing")
	if missing.OK || missing.Class != "not_found" {
		t.Fatalf("%+v", missing)
	}
	retry := c.Probe(context.Background(), "retry-model")
	if !retry.OK {
		t.Fatalf("%+v", retry)
	}
	args, err := ParseToolArgs(json.RawMessage(`"{\"kind\":\"cube\"}"`))
	if err != nil || args["kind"] != "cube" {
		t.Fatal(err, args)
	}
	if !strings.Contains(classify(429, []byte("rate limit")), "rate") {
		t.Fatal(classify(429, nil))
	}
}
