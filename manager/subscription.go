package manager

import (
	"context"
	"github.com/go-redis/redis/v8"
	"sync"
)

type SubscriptionManager struct {
	ctx context.Context
	pubsub *redis.PubSub
	mu sync.Mutex
	subscriptions map[string]int
}

func NewSubscriptionManager(ctx context.Context, pubsub *redis.PubSub) *SubscriptionManager {
	return &SubscriptionManager{
		ctx: ctx,
		pubsub: pubsub,
		subscriptions: map[string]int{},
	}
}

func (s *SubscriptionManager) increment(key string, count int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.subscriptions[key] += count

	if s.subscriptions[key] == 1 {
		return s.pubsub.Subscribe(s.ctx, key)
	} else if s.subscriptions[key] == 0 {
		return s.pubsub.Unsubscribe(s.ctx, key)
	}

	return nil
}

func (s *SubscriptionManager) Add(key string) error {
	return s.increment(key, 1)
}

func (s *SubscriptionManager) Remove(key string) error {
	return s.increment(key, -1)
}
