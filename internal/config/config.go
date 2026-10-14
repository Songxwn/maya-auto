package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

type File struct {
	MayaHost    string `json:"mayaHost"`
	MayaPort    int    `json:"mayaPort"`
	NewAPIBase  string `json:"newAPIBase"`
	NewAPIKey   string `json:"newAPIKey"`
	NewAPIModel string `json:"newAPIModel"`
}

type Store struct {
	mu   sync.Mutex
	path string
	File File
}

func Defaults() File {
	return File{
		MayaHost: "127.0.0.1",
		MayaPort: 7001,
	}
}

func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "maya-auto"), nil
}

func Load() (*Store, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "config.json")
	st := &Store{path: path, File: Defaults()}
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return st, nil
		}
		return nil, err
	}
	if len(b) == 0 {
		return st, nil
	}
	if err := json.Unmarshal(b, &st.File); err != nil {
		return nil, err
	}
	if st.File.MayaHost == "" {
		st.File.MayaHost = "127.0.0.1"
	}
	if st.File.MayaPort == 0 {
		st.File.MayaPort = 7001
	}
	return st, nil
}

func (s *Store) Path() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.path
}

func (s *Store) Snapshot() File {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.File
}

func (s *Store) Update(fn func(*File)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.File)
	if s.File.MayaHost == "" {
		s.File.MayaHost = "127.0.0.1"
	}
	if s.File.MayaPort <= 0 || s.File.MayaPort > 65535 {
		s.File.MayaPort = 7001
	}
	b, err := json.MarshalIndent(s.File, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0o600)
}

func MaskKey(k string) string {
	if k == "" {
		return ""
	}
	if len(k) <= 4 {
		return "****"
	}
	return "****" + k[len(k)-4:]
}
