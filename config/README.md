# Config

## 行为声明
- 不支持热更新
- 需要重启生效
- 内部基于 viper 进行配置解析

## 加载来源与优先级
1. 默认配置文件
2. 环境对应配置文件（ENV=dev -> config.dev.yaml）
3. 环境变量
4. 命令行参数

## 默认路径与命名
- 默认文件：configs/config.default.yaml
- 环境配置：configs/config.{env}.yaml
- 部署环境变量：ENV（可通过 DeployEnvKey 修改）
- 环境变量：APP.GROUP.KEY
- 命令行参数：--group.key=value

## 示例

### 覆盖优先级
```bash
ENV=dev \
APP.HTTP.PORT=8080 \
APP.DB.URI=mongodb://user:******@host:27017/db \
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
