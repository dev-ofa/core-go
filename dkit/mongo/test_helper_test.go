package mongo

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func testDatabase(t *testing.T) *mongo.Database {
	t.Helper()
	uri := os.Getenv("OFA_DKIT_MONGO_URI")
	if uri == "" {
		uri = os.Getenv("DKIT_MONGO_URI")
	}
	if uri == "" {
		t.Skip("set OFA_DKIT_MONGO_URI to run Mongo DKit integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cli, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("connect mongo: %v", err)
	}
	if err := cli.Ping(ctx, nil); err != nil {
		t.Fatalf("ping mongo: %v", err)
	}
	t.Cleanup(func() {
		_ = cli.Disconnect(context.Background())
	})

	dbName := os.Getenv("OFA_DKIT_MONGO_DB")
	if dbName == "" {
		dbName = "ofa-core-go-dkit-test"
	}
	return cli.Database(dbName)
}

func testPrefix(t *testing.T) string {
	t.Helper()
	replacer := strings.NewReplacer("/", "_", " ", "_", "-", "_")
	return "dkit_" + replacer.Replace(t.Name())
}

func dropCollections(t *testing.T, db *mongo.Database, names ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for _, name := range names {
		_ = db.Collection(name).Drop(ctx)
	}
}
