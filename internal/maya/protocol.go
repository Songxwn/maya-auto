package maya

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

const runtimeVersion = "1"

func chunks(s string, size int) []string {
	if size < 256 {
		size = 256
	}
	var out []string
	var b strings.Builder
	for _, r := range s {
		b.WriteRune(r)
		if b.Len() >= size {
			out = append(out, b.String())
			b.Reset()
		}
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}

func melPython(expr string) (string, error) {
	if strings.ContainsAny(expr, "\"\n\r$`") {
		return "", fmt.Errorf("内部表达式含有 MEL 特殊字符")
	}
	return `python("` + expr + `");`, nil
}

func versionProbeExpr() string {
	return `(lambda m: (getattr(m, 'VERSION', '') if m else ''))(__import__('sys').modules.get('maya_auto_rt'))`
}

func resetPartsExpr() string {
	return `setattr(__import__('maya'), '_ma_parts', []) or 'reset'`
}

func appendPartExpr(part string) string {
	b64 := base64.StdEncoding.EncodeToString([]byte(part))
	return `getattr(__import__('maya'), '_ma_parts').append(__import__('base64').b64decode('` + b64 + `').decode('utf-8')) or 'appended'`
}

func finishInstallExpr() string {
	return `(lambda m, s: (m.__dict__.__setitem__('__name__', 'maya_auto_rt'), exec(s, m.__dict__), __import__('sys').modules.__setitem__('maya_auto_rt', m), setattr(__import__('maya'), '_ma_parts', None), m.VERSION)[-1])(__import__('types').ModuleType('maya_auto_rt'), ''.join(getattr(__import__('maya'), '_ma_parts')))`
}

func dispatchExpr(name string, args map[string]any) (string, error) {
	if args == nil {
		args = map[string]any{}
	}
	payload, err := json.Marshal(map[string]any{"name": name, "args": args})
	if err != nil {
		return "", err
	}
	b64 := base64.StdEncoding.EncodeToString(payload)
	return `__import__('maya_auto_rt').dispatch_b64('` + b64 + `')`, nil
}

func mayaScalar(raw string) string {
	raw = strings.Trim(raw, "\x00")
	lines := strings.Split(raw, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := unquoteMaya(lines[i])
		if line == "" || strings.HasPrefix(line, "python(") || strings.HasPrefix(line, "//") {
			continue
		}
		return line
	}
	return ""
}

func unquoteMaya(s string) string {
	s = strings.TrimSpace(strings.Trim(s, "\x00"))
	if len(s) >= 2 {
		if (s[0] == '\'' && s[len(s)-1] == '\'') || (s[0] == '"' && s[len(s)-1] == '"') {
			s = s[1 : len(s)-1]
			s = strings.ReplaceAll(s, `\'`, `'`)
			s = strings.ReplaceAll(s, `\"`, `"`)
		}
	}
	return strings.TrimSpace(s)
}

func extractJSON(raw string) string {
	s := strings.TrimSpace(strings.Trim(raw, "\x00"))
	s = unquoteMaya(s)
	if json.Valid([]byte(s)) && strings.HasPrefix(s, "{") {
		return s
	}
	for i := strings.LastIndex(s, "{"); i >= 0; i = strings.LastIndex(s[:i], "{") {
		frag, ok := sliceJSONObject(s[i:])
		if ok && json.Valid([]byte(frag)) {
			return frag
		}
	}
	return s
}

func sliceJSONObject(s string) (string, bool) {
	depth := 0
	inStr := false
	esc := false
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			return "", false
		}
		ch := s[i]
		if inStr {
			if esc {
				esc = false
			} else if ch == '\\' {
				esc = true
			} else if ch == '"' {
				inStr = false
			}
			i += size
			continue
		}
		switch ch {
		case '"':
			inStr = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[:i+1], true
			}
		}
		i += size
	}
	return "", false
}

type envelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result"`
	Error  string          `json:"error"`
}

func parseCallResult(raw string) (json.RawMessage, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil, fmt.Errorf("Maya 返回空结果")
	}
	if strings.Contains(s, "// Error:") || strings.HasPrefix(strings.TrimSpace(s), "Error:") {
		return nil, fmt.Errorf("%s", truncate(s, 1200))
	}
	body := extractJSON(s)
	var env envelope
	if err := json.Unmarshal([]byte(body), &env); err != nil {
		return nil, fmt.Errorf("无法解析 Maya 返回: %s", truncate(s, 800))
	}
	if !env.OK {
		if env.Error == "" {
			return nil, fmt.Errorf("Maya 执行失败")
		}
		return nil, fmt.Errorf("%s", env.Error)
	}
	if len(env.Result) == 0 {
		return json.RawMessage("null"), nil
	}
	return env.Result, nil
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
