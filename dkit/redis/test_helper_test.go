package redis

import (
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

func testRedisClient(t *testing.T) (*miniredis.Miniredis, *goredis.Client) {
	t.Helper()
	srv, err := miniredis.Run()
	if err != nil {
		t.Fatalf("run miniredis: %v", err)
	}
	cli := goredis.NewClient(&goredis.Options{Addr: srv.Addr()})
	t.Cleanup(func() {
		_ = cli.Close()
		srv.Close()
	})
	return srv, cli
}

func testPrefix(t *testing.T) string {
	t.Helper()
	replacer := strings.NewReplacer("/", "_", " ", "_", "-", "_")
	return "dkit_" + replacer.Replace(t.Name())
}

func waitForCondition(t *testing.T, timeout time.Duration, fn func() bool, desc string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", desc)
}
