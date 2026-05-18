# core-go

## 模块概览
- config：配置加载、脱敏摘要与哈希、严格校验
- pass：上下文传递 trace、request、operator、tenant、app 信息
- trace/logging：带 trace/request 的统一日志接口
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

cfg, meta, err := config.Load[AppConfig](config.Options{
	RequiredKeys:  []string{"db.uri"},
	SensitiveKeys: []string{"db.uri"},
})
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

id, err := kit.NextIDString(context.Background())
if err != nil {
	return err
}

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
