package config

import (
	"os"
	"path/filepath"
	"testing"
)

type testConfig struct {
	App struct {
		Name string `yaml:"name"`
	} `yaml:"app"`
	HTTP struct {
		Port int `yaml:"port"`
	} `yaml:"http"`
	DB struct {
		URI string `yaml:"uri"`
	} `yaml:"db"`
	Logging struct {
		Level string `yaml:"level"`
	} `yaml:"logging"`
}

func TestLoadPrecedenceAndMasking(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "configs")
	defaultPath := filepath.Join(configDir, "config.default.yaml")
	envPath := filepath.Join(configDir, "config.dev.yaml")

	defaultContent := `
app:
  name: demo
http:
  port: 8080
db:
  uri: "mongodb://user:******@localhost:27017/db"
logging:
  level: INFO
`
	envContent := `
http:
  port: 7070
`

	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(defaultPath, []byte(defaultContent), 0600); err != nil {
		t.Fatalf("write default: %v", err)
	}
	if err := os.WriteFile(envPath, []byte(envContent), 0600); err != nil {
		t.Fatalf("write env: %v", err)
	}

	t.Setenv("ENV", "dev")
	t.Setenv("APP.HTTP.PORT", "9090")

	opts := NewOptions()
	opts.DefaultConfigPath = defaultPath
	opts.Args = []string{"--http.port=6060"}
	opts.RequiredKeys = []string{"db.uri"}
	cfg, meta, err := Load[testConfig](opts)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.HTTP.Port != 6060 {
		t.Fatalf("port want 6060 got %d", cfg.HTTP.Port)
	}
	if len(meta.Sources) != 4 {
		t.Fatalf("sources want 4 got %v", meta.Sources)
	}
	dbm, ok := meta.Summary["db"].(map[string]any)
	if !ok {
		t.Fatalf("summary db missing")
	}
	if dbm["uri"] != "***" {
		t.Fatalf("summary uri masked: %v", dbm["uri"])
	}
}

func TestEnvFileOptional(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "configs")
	defaultPath := filepath.Join(configDir, "config.default.yaml")

	defaultContent := `
http:
  port: 8080
db:
  uri: "mongodb://user:******@localhost:27017/db"
`
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(defaultPath, []byte(defaultContent), 0600); err != nil {
		t.Fatalf("write default: %v", err)
	}

	t.Setenv("ENV", "prod")
	t.Setenv("APP.HTTP.PORT", "9090")

	opts := NewOptions()
	opts.DefaultConfigPath = defaultPath
	opts.RequiredKeys = []string{"db.uri"}
	cfg, meta, err := Load[testConfig](opts)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.HTTP.Port != 9090 {
		t.Fatalf("port want 9090 got %d", cfg.HTTP.Port)
	}
	for _, s := range meta.Sources {
		if s == "env-file" {
			t.Fatalf("env-file should be skipped when missing")
		}
	}
}

func TestRequiredMissing(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "configs")
	defaultPath := filepath.Join(configDir, "config.default.yaml")

	defaultContent := `
http:
  port: 8080
`
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(defaultPath, []byte(defaultContent), 0600); err != nil {
		t.Fatalf("write default: %v", err)
	}

	opts := NewOptions()
	opts.DefaultConfigPath = defaultPath
	opts.RequiredKeys = []string{"db.uri"}
	_, _, err := Load[testConfig](opts)
	if err == nil {
		t.Fatalf("expected missing error")
	}
}
