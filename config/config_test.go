package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dev-ofa/core-go/model/datax"
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

type sensitiveOptionalConfig struct {
	Auth struct {
		Wechat struct {
			MiniProgramSecret string `yaml:"mini_program_secret" mapstructure:"mini_program_secret"`
		} `yaml:"wechat" mapstructure:"wechat"`
	} `yaml:"auth" mapstructure:"auth"`
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
	t.Setenv("APP__HTTP__PORT", "9090")
	t.Setenv("APP__DB__URI", "mongodb://env-user:env-pass@localhost:27017/db")

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
	t.Setenv("APP__HTTP__PORT", "9090")
	t.Setenv("APP__DB__URI", "mongodb://env-user:env-pass@localhost:27017/db")

	opts := NewOptions()
	opts.DefaultConfigPath = defaultPath
	opts.SensitiveKeys = []string{}
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
	opts.SensitiveKeys = []string{}
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

func TestEnvOverridesFilesButFlagsStillWin(t *testing.T) {
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
	t.Setenv("APP__DB__URI", "mongodb://env-user:env-pass@localhost:27017/db")
	t.Setenv("APP__LOGGING__LEVEL", "ERROR")

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
	if cfg.Logging.Level != "ERROR" {
		t.Fatalf("level want ERROR got %s", cfg.Logging.Level)
	}
	if len(meta.Sources) != 5 {
		t.Fatalf("sources want 5 got %v", meta.Sources)
	}
	if meta.Sources[0] != "default" || meta.Sources[1] != "env-file" || meta.Sources[2] != "local" || meta.Sources[3] != "env" || meta.Sources[4] != "flags" {
		t.Fatalf("sources want [default env-file local env flags] got %v", meta.Sources)
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
	if got := datax.CodeOf(err); got != datax.ErrCodeValidate {
		t.Fatalf("want validation error code got %d", got)
	}
}

func TestSensitivePlaceholderRejectedFromLocal(t *testing.T) {
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
                t.Fatalf("expected sensitive placeholder error")
	}
	if got := datax.CodeOf(err); got != datax.ErrCodeValidate {
		t.Fatalf("want validation error code got %d", got)
	}
}

func TestSensitiveValueAllowedFromLocal(t *testing.T) {
        dir := t.TempDir()
        configDir := filepath.Join(dir, "configs")
        defaultPath := filepath.Join(configDir, "config.yaml")
        localPath := filepath.Join(configDir, "config.local.yaml")

        defaultContent := `
auth:
  wechat:
    mini_program_secret: ""
`
        localContent := `
auth:
  wechat:
    mini_program_secret: "local-secret"
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
        opts.SensitiveKeys = []string{"auth.wechat.mini_program_secret"}

        cfg, _, err := Load[sensitiveOptionalConfig](opts)
        if err != nil {
                t.Fatalf("load: %v", err)
        }
        if cfg.Auth.Wechat.MiniProgramSecret != "local-secret" {
                t.Fatalf("mini program secret want local-secret got %q", cfg.Auth.Wechat.MiniProgramSecret)
        }
}

func TestSensitivePlaceholderRejectedFromEnv(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "configs")
	defaultPath := filepath.Join(configDir, "config.yaml")

	defaultContent := `
db:
  uri: ""
`
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(defaultPath, []byte(defaultContent), 0600); err != nil {
		t.Fatalf("write default: %v", err)
	}

	t.Setenv("APP__DB__URI", "******")

	opts := NewOptions()
	opts.DefaultConfigPath = defaultPath
	opts.RequiredKeys = []string{"db.uri"}
	_, _, err := Load[testConfig](opts)
	if err == nil {
		t.Fatalf("expected sensitive placeholder error")
	}
	if got := datax.CodeOf(err); got != datax.ErrCodeValidate {
		t.Fatalf("want validation error code got %d", got)
	}
}

func TestEmptySensitiveValueAllowedFromFile(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "configs")
	defaultPath := filepath.Join(configDir, "config.yaml")

	defaultContent := `
auth:
  wechat:
    mini_program_secret: ""
`
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(defaultPath, []byte(defaultContent), 0600); err != nil {
		t.Fatalf("write default: %v", err)
	}

	opts := NewOptions()
	opts.DefaultConfigPath = defaultPath
	opts.SensitiveKeys = []string{"auth.wechat.mini_program_secret"}

	cfg, _, err := Load[sensitiveOptionalConfig](opts)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Auth.Wechat.MiniProgramSecret != "" {
		t.Fatalf("mini program secret want empty got %q", cfg.Auth.Wechat.MiniProgramSecret)
	}
}

func TestSensitiveValueInFileCanBeOverriddenByEnv(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "configs")
	defaultPath := filepath.Join(configDir, "config.yaml")

	defaultContent := `
auth:
  wechat:
    mini_program_secret: "debug-secret"
`
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(defaultPath, []byte(defaultContent), 0600); err != nil {
		t.Fatalf("write default: %v", err)
	}

	t.Setenv("APP__AUTH__WECHAT__MINI_PROGRAM_SECRET", "env-secret")

	opts := NewOptions()
	opts.DefaultConfigPath = defaultPath
	opts.SensitiveKeys = []string{"auth.wechat.mini_program_secret"}

	cfg, _, err := Load[sensitiveOptionalConfig](opts)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Auth.Wechat.MiniProgramSecret != "env-secret" {
		t.Fatalf("mini program secret want env-secret got %q", cfg.Auth.Wechat.MiniProgramSecret)
	}
}

func TestEnvToMapIgnoresInvalidEnvNames(t *testing.T) {
	t.Setenv("APPTEST__HTTP__PORT", "8080")
	t.Setenv("APPTEST__db__uri", "sqlite://invalid")
	t.Setenv("APPTEST____BROKEN", "invalid")

	got := envToMap("APPTEST", "__")
	if port, ok := got["http"].(map[string]any)["port"]; !ok || port != "8080" {
		t.Fatalf("valid env missing: %v", got)
	}
	if _, ok := got["db"]; ok {
		t.Fatalf("lowercase env name should be ignored: %v", got)
	}
	if _, ok := got[""]; ok {
		t.Fatalf("empty env path node should be ignored: %v", got)
	}
}
