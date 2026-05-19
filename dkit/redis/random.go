package redis

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/dev-ofa/core-go/dkit"
	goredis "github.com/redis/go-redis/v9"
)

const randomLeaseTTL = time.Minute

// RandomNumberImpl allocates temporary unique numbers from Redis.
type RandomNumberImpl struct {
	randMu sync.Mutex
	r      *rand.Rand

	redisCli  goredis.UniversalClient
	keyPrefix string
}

// NewRandomNumberImpl creates a RandomNumberImpl.
func NewRandomNumberImpl(cli goredis.UniversalClient, keyPrefix string) *RandomNumberImpl {
	return &RandomNumberImpl{
		redisCli:  cli,
		keyPrefix: keyPrefix,
	}
}

// Init initializes the random number allocator state.
func (impl *RandomNumberImpl) Init() error {
	impl.r = rand.New(rand.NewSource(time.Now().UnixNano()))
	return nil
}

// GetUniqueRandomNumber returns a unique number in [0, max).
func (impl *RandomNumberImpl) GetUniqueRandomNumber(ctx context.Context, max int) (int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if max <= 0 {
		return 0, fmt.Errorf("%w: max must be positive", dkit.ErrInvalidOption)
	}
	if impl.redisCli == nil {
		return 0, fmt.Errorf("%w: redis client is nil", dkit.ErrInvalidOption)
	}
	if impl.r == nil {
		impl.r = rand.New(rand.NewSource(time.Now().UnixNano()))
	}

	for _, num := range impl.nextPerm(max) {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}
		ok, err := impl.redisCli.SetNX(ctx, buildRandomKey(impl.keyPrefix, num), "1", randomLeaseTTL).Result()
		if err != nil {
			return 0, fmt.Errorf("claim random number failed: %w", err)
		}
		if ok {
			return num, nil
		}
	}
	return 0, dkit.ErrNoAvailableNumber
}

func (impl *RandomNumberImpl) nextPerm(max int) []int {
	impl.randMu.Lock()
	defer impl.randMu.Unlock()
	return impl.r.Perm(max)
}
