package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Songxwn/maya-auto/internal/newapi"
)

func TestRunToolThenAnswer(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		body, _ := io.ReadAll(r.Body)
		if calls == 1 {
			if !json.Valid(body) {
				t.Fatalf("bad body")
			}
			_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"先看场景","tool_calls":[{"id":"c1","type":"function","function":{"name":"maya_status","arguments":"{}"}}]}}]}`)
			return
		}
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"场景是空的"}}]}`)
	}))
	defer srv.Close()
	client := &newapi.Client{BaseURL: srv.URL, APIKey: "k", HTTP: srv.Client()}
	var events []Event
	err := Run(context.Background(), client, "demo", "看一下场景", func(name string, args map[string]any, wait time.Duration) (json.RawMessage, error) {
		if name != "maya_status" || wait <= 0 {
			t.Fatalf("%s %v %s", name, args, wait)
		}
		return json.RawMessage(`{"version":"2024"}`), nil
	}, func(ev Event) { events = append(events, ev) })
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatal(calls)
	}
	found := false
	for _, ev := range events {
		if ev.Kind == "assistant" && ev.Text == "场景是空的" {
			found = true
		}
	}
	if !found {
		t.Fatalf("%+v", events)
	}
}
