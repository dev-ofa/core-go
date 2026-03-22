# core-go

## 模块概览
- config：配置加载、脱敏摘要与哈希、严格校验
- pass：上下文传递 trace、request、operator、tenant、app 信息
- trace/logging：带 trace/request 的统一日志接口
- model：通用审计字段与上下文注入

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
