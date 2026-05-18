package mongo

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/dev-ofa/core-go/dkit"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// RandomNumberImpl allocates temporary unique numbers from MongoDB.
type RandomNumberImpl struct {
	randMu sync.Mutex
	r      *rand.Rand
	cls    *mongo.Collection
}

// NewRandomNumberImpl creates a RandomNumberImpl.
func NewRandomNumberImpl(cls *mongo.Collection) *RandomNumberImpl {
	return &RandomNumberImpl{
		cls: cls,
	}
}

// Init initializes collection indexes.
func (impl *RandomNumberImpl) Init() error {
	impl.r = rand.New(rand.NewSource(time.Now().UnixNano()))
	return ensureTTLIndex(context.Background(), impl.cls)
}

// GetUniqueRandomNumber returns a unique number in [0, max).
func (impl *RandomNumberImpl) GetUniqueRandomNumber(ctx context.Context, max int) (int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if max <= 0 {
		return 0, fmt.Errorf("%w: max must be positive", dkit.ErrInvalidOption)
	}
	if impl.cls == nil {
		return 0, fmt.Errorf("%w: mongo collection is nil", dkit.ErrInvalidOption)
	}
	if impl.r == nil {
		impl.r = rand.New(rand.NewSource(time.Now().UnixNano()))
	}

	expires := time.Now().Add(time.Minute)
	for {
		num := impl.nextInt(max)
		findRet := RandInfo{}
		err := impl.cls.FindOne(ctx, bson.M{"_id": num}).Decode(&findRet)
		if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
			return 0, fmt.Errorf("find random number failed: %w", err)
		}

		if errors.Is(err, mongo.ErrNoDocuments) {
			if _, err := impl.cls.InsertOne(ctx, &RandInfo{ID: num, Expires: expires}); err != nil {
				if mongo.IsDuplicateKeyError(err) {
					continue
				}
				return 0, fmt.Errorf("insert random number failed: %w", err)
			}
			return num, nil
		}

		if findRet.Expires.Before(time.Now()) {
			ret, err := impl.cls.UpdateOne(ctx,
				bson.M{"_id": num, "expires": findRet.Expires},
				bson.M{"$set": bson.M{"expires": expires}},
			)
			if err != nil {
				return 0, fmt.Errorf("update random number failed: %w", err)
			}
			if ret.ModifiedCount > 0 {
				return num, nil
			}
		}
	}
}

func (impl *RandomNumberImpl) nextInt(max int) int {
	impl.randMu.Lock()
	defer impl.randMu.Unlock()
	return impl.r.Intn(max)
}
