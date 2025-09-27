package service

import (
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service/utils"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

type OrderService struct {
	store *repository.Store
}

// --- キャッシュ構造 ---
type cachedOrders struct {
	orders    []model.Order
	total     int
	expiresAt time.Time
}

var ordersCache struct {
	sync.RWMutex
	data map[string]cachedOrders
}

func init() {
	ordersCache.data = make(map[string]cachedOrders)
}

// キャッシュキー生成
func makeCacheKey(userID int, req model.ListRequest) string {
	raw := fmt.Sprintf("%d|%s|%s|%s|%s|%d|%d",
		userID,
		req.SortField,
		req.SortOrder,
		req.Search,
		req.Type,
		req.PageSize,
		req.Offset,
	)
	h := sha1.Sum([]byte(raw))
	return hex.EncodeToString(h[:])
}

func NewOrderService(store *repository.Store) *OrderService {
	return &OrderService{store: store}
}

// ユーザーの注文履歴を取得
func (s *OrderService) FetchOrders(ctx context.Context, userID int, req model.ListRequest) ([]model.Order, int, error) {
	key := makeCacheKey(userID, req)

	// --- キャッシュ確認 ---
	ordersCache.RLock()
	if cp, ok := ordersCache.data[key]; ok && time.Now().Before(cp.expiresAt) {
		// コピーを返す（外側で変更されないように）
		ordersCopy := make([]model.Order, len(cp.orders))
		copy(ordersCopy, cp.orders)
		total := cp.total
		ordersCache.RUnlock()
		return ordersCopy, total, nil
	}
	ordersCache.RUnlock()

	// --- DBアクセス ---
	var orders []model.Order
	var total int
	err := utils.WithTimeout(ctx, func(ctx context.Context) error {
		var fetchErr error
		orders, total, fetchErr = s.store.OrderRepo.ListOrders(ctx, userID, req)
		return fetchErr
	})
	if err != nil {
		return nil, 0, err
	}

	// --- キャッシュ保存 ---
	ordersCache.Lock()
	ordersCache.data[key] = cachedOrders{
		orders:    orders,
		total:     total,
		expiresAt: time.Now().Add(1 * time.Second), // キャッシュTTL
	}
	ordersCache.Unlock()

	return orders, total, nil
}
