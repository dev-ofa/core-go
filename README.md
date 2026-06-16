# core-go

## Modules
- `config`: configuration loading, redacted summaries and hashes, strict validation
- `pass`: trace, request, operator, tenant, and app context propagation
- `trace/logging`: unified logging interfaces carrying trace and request context
- `httpx`: HTTP client with trace propagation, timeout budgets, bounded retries, and pluggable service discovery
- `model`: shared audit fields and context-driven audit injection
- `dkit`: distributed primitive abstractions, snowflake IDs, and distributed mutex helpers

## Quick Start

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
var backend dkit.Atomic // Injected by a Mongo/MySQL/Redis backend adapter.

kit, err := dkit.NewDefaultKitWithContext(context.Background(), backend)
if err != nil {
	return err
}
// A provider-owned default instance can also be registered during application startup
// for reuse across shared call sites.
dkit.SetDefaultKit(kit)
defer dkit.ResetDefaultKit()

id := dkit.DefaultKit().GetSnowflakeID()
// dkit uses a fixed Sonyflake epoch so newly generated decimal IDs start in the
// 19-digit range immediately and avoid a future 18/19-digit boundary change.
// model.SnowflakeID is a string in Go/API paths and stored numerically in the
// database. If the project can choose its own ID format, prefer fixed-width string IDs.

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

`dkitmongo` refers to `github.com/dev-ofa/core-go/dkit/mongo`.

Mongo adapter integration tests are skipped by default. Run them with `OFA_DKIT_MONGO_URI` set:

```bash
OFA_DKIT_MONGO_URI='mongodb://localhost:27017' go test ./dkit/mongo
```
