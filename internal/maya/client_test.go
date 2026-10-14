package maya

import (
	"bufio"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"
)

func TestRuntimeExports(t *testing.T) {
	if !strings.Contains(runtimeSource, `VERSION = "1"`) {
		t.Fatal("runtime version string missing")
	}
	for _, name := range []string{
		"maya_status", "maya_scene", "maya_list", "maya_select", "maya_primitive",
		"maya_transform", "maya_edit", "maya_material", "maya_camera", "maya_fx",
		"maya_simulate", "maya_light", "maya_render_settings", "maya_render", "maya_execute",
	} {
		if !strings.Contains(runtimeSource, `"`+name+`"`) {
			t.Fatalf("runtime missing %s", name)
		}
	}
	joined := strings.Join(chunks(runtimeSource, 3500), "")
	if joined != runtimeSource {
		t.Fatal("chunks do not round-trip")
	}
	for _, expr := range []string{versionProbeExpr(), resetPartsExpr(), finishInstallExpr(), appendPartExpr("print(1)\n")} {
		if _, err := melPython(expr); err != nil {
			t.Fatalf("expr rejected: %v", err)
		}
	}
}

func TestParseCallResult(t *testing.T) {
	raw, err := parseCallResult("prefix\n{\"ok\":true,\"result\":{\"version\":\"2024\"}}\x00")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got["version"] != "2024" {
		t.Fatalf("%s", raw)
	}
	if _, err := parseCallResult(`{"ok":false,"error":"boom"}`); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatal(err)
	}
	if mayaScalar("python(\"x\");\n1\n") != "1" {
		t.Fatal(mayaScalar("python(\"x\");\n1\n"))
	}
}

func TestClientAgainstFakeMaya(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		r := bufio.NewReader(conn)
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			resp := "None"
			switch {
			case strings.Contains(line, "dispatch_b64"):
				resp = `{"ok":true,"result":{"pong":true}}`
			case strings.Contains(line, "m.VERSION"):
				resp = "1"
			case strings.Contains(line, "modules.get"):
				resp = "''"
			}
			_, _ = conn.Write([]byte(resp + "\x00"))
		}
	}()
	host, portStr, _ := strings.Cut(ln.Addr().String(), ":")
	var port int
	for _, c := range portStr {
		port = port*10 + int(c-'0')
	}
	c := New()
	if err := c.Connect(host, port); err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	out, err := c.Call("maya_status", map[string]any{}, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "pong") {
		t.Fatalf("%s", out)
	}
}
