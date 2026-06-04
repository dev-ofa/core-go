# Config

## 行为声明
- 不支持热更新
- 需要重启生效
- 内部基于 viper 进行配置解析

## 加载来源与优先级
1. 默认配置文件
2. 环境对应配置文件（ENV=dev -> config.dev.yaml）
3. 本地覆盖文件（config.local.yaml）
4. 环境变量

命令行参数是 `core-go` 的实现扩展，不属于标准配置来源。当前实现支持 `--group.key=value`，其优先级高于环境变量，仅建议用于本地调试、临时诊断和测试场景。

命令行参数不得用于传入密钥、密码、Token 等敏感配置；敏感配置必须来自环境变量或安全存储。

## 默认路径与命名
- 默认文件：configs/config.yaml
- 环境配置：configs/config.{env}.yaml
- 本地覆盖：configs/config.local.yaml
- 部署环境变量：ENV（可通过 DeployEnvKey 修改）
- 环境变量：APP__GROUP__KEY
- 命令行参数扩展：--group.key=value

`DefaultConfigPath` 表示默认基础配置文件路径。显式传入该路径时，当前实现会以该文件作为基础配置来源，并继续在该文件所在目录查找 `config.{env}.yaml` 和 `config.local.yaml` 参与最终配置计算。

环境变量名必须使用大写 ASCII 字母、数字与下划线，并使用 `__` 表示层级；不符合该规则的环境变量会被忽略。

## 示例

### 覆盖优先级
```bash
ENV=dev \
APP__HTTP__PORT=8080 \
APP__DB__URI=mongodb://user:password-from-env@host:27017/db \
your-app --http.port=9090
```

```go
type AppConfig struct {
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

### 环境配置文件
```bash
ENV=dev
```
默认读取 configs/config.dev.yaml，不存在则忽略。

### 本地覆盖文件
```bash
configs/config.local.yaml
```
存在时会参与最终配置计算，优先级高于环境配置文件、低于环境变量和命令行参数。

### 命令行参数扩展
```bash
your-app --http.port=9090
```
命令行参数参与最终配置计算，优先级高于环境变量。该能力不属于标准配置来源，不应作为标准部署配置方式，也不得用于传入敏感配置。启动日志会在 `config sources` 中记录 `flags`。

### 敏感配置来源校验
```go
opts := config.NewOptions()
opts.RequiredKeys = []string{"db.uri"}
opts.SensitiveKeys = []string{"db.uri", "db.password"}
cfg, _, err := config.Load[AppConfig](opts)
_ = cfg
_ = err
```

## 使用示例
```go
type AppConfig struct {
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

opts := config.NewOptions()
opts.RequiredKeys = []string{"db.uri"}
opts.SensitiveKeys = []string{"db.uri", "db.password"}
cfg, meta, err := config.Load[AppConfig](opts)
```
