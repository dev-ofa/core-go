# core-go

## 模块概览
- config：配置加载、脱敏摘要与哈希、严格校验
- pass：上下文传递 trace、request、operator、tenant、app 信息
- trace/logging：带 trace/request 的统一日志接口
- httpx：带 trace 透传、超时预算、有限重试与可插拔服务发现的 HTTP client
- model：通用审计字段与上下文注入
- dkit：分布式原语核心抽象、雪花 ID、分布式锁执行辅助

## 快速使用

### config
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

### pass
```go
ctx := context.Background()
ctx = pass.CtxSetTraceID(ctx, "trace-1")
ctx = pass.CtxSetRequestID(ctx, "req-1")
ctx = pass.CtxSetOperator(ctx, "user-1")
ctx = pass.CtxSetTenantID(ctx, "tenant-1")
ctx = pass.CtxSetAppID(ctx, "app-1")

traceID, _ := pass.CtxGetTraceID(ctx)
reqID, _ := pass.CtxGetRequestID(ctx)
_ = traceID
_ = reqID
```

### trace/logging
```go
ctx := context.Background()
ctx = pass.CtxSetTraceID(ctx, "trace-1")
ctx = pass.CtxSetRequestID(ctx, "req-1")

logging.CtxInfof(ctx, "hello %s", "world")
logging.Infof("system %s", "ready")
```

### httpx
```go
type UserResp struct {
	Name string `json:"name"`
}

ctx := context.Background()
ctx = pass.CtxSetTraceID(ctx, "8f14e45fceea167a5a36dedd4bea2543")
ctx = pass.CtxSetOperator(ctx, "user-1")
ctx = pass.CtxSetTenantID(ctx, "tenant-1")
ctx = pass.CtxSetAppID(ctx, "app-1")

var resp UserResp
err := httpx.Get("http://inventory.prod/api/v1/users/me",
	httpx.Context(ctx),
	httpx.JsonResp(&resp),
	httpx.RespWrapper(httpx.NewCommonWrapper()),
	httpx.Service(httpx.ServiceOptions{
		EnableDiscovery: true,
		Namespace:       "prod",
	}),
).Do()
_ = resp
_ = err
```

### model
```go
type Order struct {
	model.CreateAudit
	model.UpdateAudit
	model.DeleteAudit
}

ctx := context.Background()
ctx = pass.CtxSetOperator(ctx, "user-1")
ctx = pass.CtxSetTenantID(ctx, "tenant-1")
ctx = pass.CtxSetAppID(ctx, "app-1")

o := Order{}
_ = model.CtxCreateAudit(ctx, &o)
_ = model.CtxUpdateAudit(ctx, &o)
_, _ = model.CtxDeleteAudit(ctx, &o)
```

### dkit
```go
var backend dkit.Atomic // 由 Mongo/MySQL/Redis 等后端适配实现注入。

kit, err := dkit.NewDefaultKitWithContext(context.Background(), backend)
if err != nil {
	return err
}
// 应用启动时也可以注册 provider-owned 默认实例，供大量基础设施调用点复用。
dkit.SetDefaultKit(kit)
defer dkit.ResetDefaultKit()

id := dkit.DefaultKit().GetSnowflakeID()
// dkit 使用固定 Sonyflake epoch，让新生成的十进制 ID 上线即进入 19 位区间，避免未来跨 18/19 位边界。
// model.SnowflakeID 在 Go/API 链路中是字符串类型，在数据库中按数值存储；若项目可自由选择 ID 格式，优先考虑固定宽度字符串 ID。

err = kit.MutexCtxDo(context.Background(), "daily-worker", func(ctx context.Context) error {
	// do work while holding the distributed mutex
	return nil
})
_ = id
_ = err
```

#### dkit/mongo
```go
client, err := mongo.Connect(options.Client().ApplyURI("mongodb://localhost:27017"))
if err != nil {
	return err
}

backend, err := dkitmongo.NewMongoAtomic(
	dkitmongo.Database(client.Database("app")),
	dkitmongo.CollectionPrefix("dkit"),
	dkitmongo.TTL(30*time.Second),
)
if err != nil {
	return err
}
defer backend.Close()

kit, err := dkit.NewDefaultKitWithContext(context.Background(), backend)
if err != nil {
	return err
}
_ = kit
```

其中 `dkitmongo` 是 `github.com/dev-ofa/core-go/dkit/mongo`。

Mongo adapter 的集成测试默认跳过；设置 `OFA_DKIT_MONGO_URI` 后可运行：

```bash
OFA_DKIT_MONGO_URI='mongodb://localhost:27017' go test ./dkit/mongo
```
