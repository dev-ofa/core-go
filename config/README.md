# Config

## Behavior
- Hot reload is not supported
- Process restart is required for config changes to take effect
- Config parsing is implemented on top of `viper`

## Sources and Precedence
1. Default config file
2. Environment-specific config file (`APP__ENV=dev` -> `config.dev.yaml`)
3. Local override file (`config.local.yaml`)
4. Environment variables

Command-line flags are a `core-go` implementation extension and are not part of the standard config sources. The current implementation supports `--group.key=value`, with precedence above environment variables. Use it only for local debugging, temporary diagnostics, and tests.

Command-line flags must not be used for secrets, passwords, tokens, or other sensitive config. In shared environments, sensitive config must come from environment variables or secure storage. Local development may use `config.local.yaml` for sensitive values.

## Default Paths and Naming
- Default file: `configs/config.yaml`
- Environment file: `configs/config.{env}.yaml`
- Local override: `configs/config.local.yaml`
- Deployment environment variable: `EnvPrefix + EnvSeparator + DeployEnvKey`, `APP__ENV` by default
- Environment variable pattern: `APP__GROUP__KEY`
- Command-line flag extension: `--group.key=value`

`DefaultConfigPath` is the path to the base config file. When passed explicitly, the current implementation uses that file as the base source and continues loading `config.{env}.yaml` and `config.local.yaml` from the same directory when present.

Environment variable names must use uppercase ASCII letters, digits, and underscores, with `__` as the hierarchy separator. Variables that do not follow this rule are ignored.

## Examples

### Override Precedence
```bash
APP__ENV=dev \
APP__HTTP__PORT=8080 \
APP__DB__URI=mongodb://user:${DB_PASSWORD}@host:27017/db \
your-app --http.port=9090
```

```go
type AppConfig struct {
	App struct {
		Env string `yaml:"env"`
	} `yaml:"app"`
	HTTP struct {
		Port int `yaml:"port"`
	} `yaml:"http"`
	DB struct {
		URI string `yaml:"uri"`
	} `yaml:"db"`
}

opts := config.NewOptions()
opts.RequiredKeys = []string{"db.uri"}
opts.SensitiveKeys = []string{"db.uri"}
cfg, meta, err := config.Load[AppConfig](opts)
_ = cfg
_ = meta
_ = err
```

### Environment-Specific Config File
```bash
APP__ENV=dev
```
By default, `configs/config.dev.yaml` is loaded when present.

The selected deployment environment also participates in final config overrides. With the defaults, `APP__ENV=dev` maps to `app.env=dev`. Customizing `EnvPrefix`, `EnvSeparator`, or `DeployEnvKey` changes both the deployment environment variable name and the final config path, e.g. `SERVICE__PROFILE=dev` maps to `service.profile=dev`.

### Local Override File
```bash
configs/config.local.yaml
```
When present, it participates in the final config result. Its precedence is higher than the environment-specific file and lower than environment variables and command-line flags.

### Command-Line Flag Extension
```bash
your-app --http.port=9090
```
Command-line flags participate in the final config result with precedence above environment variables. This capability is not a standard config source, should not be used as a normal deployment mechanism, and must not be used for sensitive config. Startup logs record this source as `flags` in `config sources`.

### Sensitive Config Source Validation
```go
opts := config.NewOptions()
opts.RequiredKeys = []string{"db.uri"}
opts.SensitiveKeys = []string{"db.uri", "db.password"}
cfg, _, err := config.Load[AppConfig](opts)
_ = cfg
_ = err
```

When a sensitive config key has a concrete value, that value must come from an environment variable or `config.local.yaml`. Sensitive values from the base config file or environment-specific config file are rejected. Empty strings are still treated as unset so the base config can leave them blank and let the runtime environment or local override provide the actual value.

## Usage Example
```go
type AppConfig struct {
	App struct {
		Name string `yaml:"name"`
		Env  string `yaml:"env"`
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

opts := config.NewOptions()
opts.RequiredKeys = []string{"db.uri"}
opts.SensitiveKeys = []string{"db.uri", "db.password"}
cfg, meta, err := config.Load[AppConfig](opts)
```
