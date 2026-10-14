package maya

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

//go:embed runtime/maya_auto_rt.py
var runtimeSource string

type Client struct {
	mu         sync.Mutex
	conn       net.Conn
	pending    []byte
	host       string
	port       int
	installed  bool
	connected  bool
}

func New() *Client { return &Client{} }

func (c *Client) Connected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected && c.conn != nil
}

func (c *Client) Address() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.host == "" {
		return ""
	}
	return fmt.Sprintf("%s:%d", c.host, c.port)
}

func (c *Client) Connect(host string, port int) error {
	if host == "" {
		host = "127.0.0.1"
	}
	if port <= 0 {
		port = 7001
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closeLocked()
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), 4*time.Second)
	if err != nil {
		return fmt.Errorf("连接 Maya 命令端口 %s:%d 失败（请先在 Maya 脚本编辑器运行启动脚本）: %w", host, port, err)
	}
	c.conn = conn
	c.host = host
	c.port = port
	c.pending = nil
	if err := c.ensureRuntimeLocked(); err != nil {
		c.closeLocked()
		return err
	}
	c.connected = true
	c.installed = true
	return nil
}

func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closeLocked()
}

func (c *Client) closeLocked() {
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
	c.connected = false
	c.installed = false
	c.pending = nil
}

func (c *Client) Call(name string, args map[string]any, wait time.Duration) (json.RawMessage, error) {
	if wait <= 0 {
		wait = 45 * time.Second
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil || !c.connected {
		return nil, fmt.Errorf("尚未连接 Maya")
	}
	if !c.installed {
		if err := c.ensureRuntimeLocked(); err != nil {
			c.closeLocked()
			return nil, err
		}
		c.installed = true
	}
	expr, err := dispatchExpr(name, args)
	if err != nil {
		return nil, err
	}
	mel, err := melPython(expr)
	if err != nil {
		return nil, err
	}
	raw, err := c.roundTrip(mel, wait)
	if err != nil {
		c.closeLocked()
		return nil, err
	}
	result, err := parseCallResult(raw)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) ensureRuntimeLocked() error {
	mel, err := melPython(versionProbeExpr())
	if err != nil {
		return err
	}
	raw, err := c.roundTrip(mel, 8*time.Second)
	if err != nil {
		return fmt.Errorf("探测 Maya 运行时失败: %w", err)
	}
	if mayaScalar(raw) == runtimeVersion {
		return nil
	}
	steps := []string{resetPartsExpr()}
	for _, part := range chunks(runtimeSource, 1800) {
		steps = append(steps, appendPartExpr(part))
	}
	steps = append(steps, finishInstallExpr())
	for i, expr := range steps {
		mel, err = melPython(expr)
		if err != nil {
			return err
		}
		wait := 8 * time.Second
		if i == len(steps)-1 {
			wait = 20 * time.Second
		}
		raw, err = c.roundTrip(mel, wait)
		if err != nil {
			return fmt.Errorf("安装 Maya 运行时失败: %w", err)
		}
		if strings.Contains(raw, "// Error:") || strings.HasPrefix(strings.TrimSpace(raw), "Error:") {
			return fmt.Errorf("安装 Maya 运行时失败: %s", truncate(raw, 800))
		}
		if i == len(steps)-1 && mayaScalar(raw) != runtimeVersion {
			return fmt.Errorf("Maya 运行时版本异常: %s", truncate(raw, 300))
		}
	}
	return nil
}

func (c *Client) roundTrip(mel string, wait time.Duration) (string, error) {
	if c.conn == nil {
		return "", fmt.Errorf("未连接")
	}
	if _, err := c.conn.Write([]byte(mel + "\n")); err != nil {
		return "", err
	}
	return c.readResponse(wait)
}

func (c *Client) readResponse(wait time.Duration) (string, error) {
	deadline := time.Now().Add(wait)
	buf := append([]byte{}, c.pending...)
	c.pending = nil
	tmp := make([]byte, 8192)
	grace := time.Time{}
	for {
		if msg, rest, ok := splitFrame(buf); ok {
			c.pending = rest
			return msg, nil
		}
		now := time.Now()
		readUntil := deadline
		if !grace.IsZero() && grace.Before(readUntil) {
			readUntil = grace
		}
		if !readUntil.After(now) {
			if len(buf) > 0 {
				return string(buf), nil
			}
			return "", fmt.Errorf("等待 Maya 响应超时")
		}
		_ = c.conn.SetReadDeadline(readUntil)
		n, err := c.conn.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			if !grace.IsZero() {
				continue
			}
			if indexByte(buf, 0) < 0 && indexByte(buf, '\n') >= 0 {
				grace = time.Now().Add(250 * time.Millisecond)
			}
			continue
		}
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				if len(buf) > 0 && (!grace.IsZero() || time.Now().After(deadline)) {
					return string(trimFrame(buf)), nil
				}
				if time.Now().After(deadline) {
					return "", fmt.Errorf("等待 Maya 响应超时")
				}
				continue
			}
			if len(buf) > 0 {
				return string(trimFrame(buf)), nil
			}
			return "", err
		}
	}
}

func splitFrame(buf []byte) (string, []byte, bool) {
	if i := indexByte(buf, 0); i >= 0 {
		rest := append([]byte{}, buf[i+1:]...)
		return string(buf[:i]), rest, true
	}
	return "", nil, false
}

func trimFrame(buf []byte) []byte {
	if i := indexByte(buf, 0); i >= 0 {
		return buf[:i]
	}
	return buf
}

func indexByte(buf []byte, c byte) int {
	for i, b := range buf {
		if b == c {
			return i
		}
	}
	return -1
}
