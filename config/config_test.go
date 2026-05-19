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
	defaultPath := filepath.Join(configDir, "config.yaml")
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
	defaultPath := filepath.Join(configDir, "config.yaml")

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

func TestLocalFileAutoLoadedWithoutEnv(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "configs")
	defaultPath := filepath.Join(configDir, "config.yaml")
	localPath := filepath.Join(configDir, "config.local.yaml")

	defaultContent := `
http:
  port: 8080
db:
  uri: "mongodb://user:******@localhost:27017/db"
logging:
  level: INFO
`
	localContent := `
http:
  port: 7070
logging:
  level: DEBUG
`

	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(defaultPath, []byte(defaultContent), 0600); err != nil {
		t.Fatalf("write default: %v", err)
	}
	if err := os.WriteFile(localPath, []byte(localContent), 0600); err != nil {
		t.Fatalf("write local: %v", err)
	}

	opts := NewOptions()
	opts.DefaultConfigPath = defaultPath
	opts.RequiredKeys = []string{"db.uri"}
	cfg, meta, err := Load[testConfig](opts)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.HTTP.Port != 7070 {
		t.Fatalf("port want 7070 got %d", cfg.HTTP.Port)
	}
	if cfg.Logging.Level != "DEBUG" {
		t.Fatalf("level want DEBUG got %s", cfg.Logging.Level)
	}
	if len(meta.Sources) != 2 {
		t.Fatalf("sources want 2 got %v", meta.Sources)
	}
	if meta.Sources[0] != "default" || meta.Sources[1] != "local" {
		t.Fatalf("sources want [default local] got %v", meta.Sources)
	}
}

func TestLocalOverridesEnvFileButFlagsStillWin(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "configs")
	defaultPath := filepath.Join(configDir, "config.yaml")
	envPath := filepath.Join(configDir, "config.dev.yaml")
	localPath := filepath.Join(configDir, "config.local.yaml")

	defaultContent := `
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
logging:
  level: WARN
`
	localContent := `
http:
  port: 6060
logging:
  level: DEBUG
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
	if err := os.WriteFile(localPath, []byte(localContent), 0600); err != nil {
		t.Fatalf("write local: %v", err)
	}

	t.Setenv("ENV", "dev")

	opts := NewOptions()
	opts.DefaultConfigPath = defaultPath
	opts.Args = []string{"--http.port=5050"}
	opts.RequiredKeys = []string{"db.uri"}
	cfg, meta, err := Load[testConfig](opts)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.HTTP.Port != 5050 {
		t.Fatalf("port want 5050 got %d", cfg.HTTP.Port)
	}
	if cfg.Logging.Level != "DEBUG" {
		t.Fatalf("level want DEBUG got %s", cfg.Logging.Level)
	}
	if len(meta.Sources) != 4 {
		t.Fatalf("sources want 4 got %v", meta.Sources)
	}
	if meta.Sources[0] != "default" || meta.Sources[1] != "env-file" || meta.Sources[2] != "local" || meta.Sources[3] != "flags" {
		t.Fatalf("sources want [default env-file local flags] got %v", meta.Sources)
	}
}

func TestRequiredMissing(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "configs")
	defaultPath := filepath.Join(configDir, "config.yaml")

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

func TestSensitivePlaceholderOnlyAllowedFromDefault(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "configs")
	defaultPath := filepath.Join(configDir, "config.yaml")
	localPath := filepath.Join(configDir, "config.local.yaml")

	defaultContent := `
db:
  uri: "mongodb://user:******@localhost:27017/db"
`
	localContent := `
db:
  uri: "******"
`

	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(defaultPath, []byte(defaultContent), 0600); err != nil {
		t.Fatalf("write default: %v", err)
	}
	if err := os.WriteFile(localPath, []byte(localContent), 0600); err != nil {
		t.Fatalf("write local: %v", err)
	}

	opts := NewOptions()
	opts.DefaultConfigPath = defaultPath
	opts.RequiredKeys = []string{"db.uri"}
	_, _, err := Load[testConfig](opts)
	if err == nil {
		t.Fatalf("expected sensitive placeholder source error")
	}
}
