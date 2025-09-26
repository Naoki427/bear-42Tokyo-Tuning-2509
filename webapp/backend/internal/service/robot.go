package service

import (
	"context"
	"log"
	"sync"
	"time"
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service/utils"
)

type RobotService struct {
	store *repository.Store
}

// キャッシュ用の構造体
var deliveryPlanCache struct {
	sync.RWMutex
	plan      *model.DeliveryPlan
	expiresAt time.Time
}

func NewRobotService(store *repository.Store) *RobotService {
	return &RobotService{store: store}
}

func (s *RobotService) GenerateDeliveryPlan(ctx context.Context, robotID string, capacity int) (*model.DeliveryPlan, error) {
	// ---- キャッシュ確認 ----
	deliveryPlanCache.RLock()
	if deliveryPlanCache.plan != nil && time.Now().Before(deliveryPlanCache.expiresAt) {
		planCopy := *deliveryPlanCache.plan
		deliveryPlanCache.RUnlock()
		return &planCopy, nil
	}
	deliveryPlanCache.RUnlock()

	// ---- DBアクセスして生成 ----
	var plan model.DeliveryPlan
	err := utils.WithTimeout(ctx, func(ctx context.Context) error {
		return s.store.ExecTx(ctx, func(txStore *repository.Store) error {
			orders, err := txStore.OrderRepo.GetShippingOrders(ctx)
			if err != nil {
				return err
			}
			plan, err = selectOrdersForDelivery(ctx, orders, robotID, capacity)
			if err != nil {
				return err
			}
			if len(plan.Orders) > 0 {
				orderIDs := make([]int64, len(plan.Orders))
				for i, order := range plan.Orders {
					orderIDs[i] = order.OrderID
				}

				if err := txStore.OrderRepo.UpdateStatuses(ctx, orderIDs, "delivering"); err != nil {
					return err
				}
				log.Printf("Updated status to 'delivering' for %d orders", len(orderIDs))
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}

	// ---- キャッシュ保存 ----
	deliveryPlanCache.Lock()
	deliveryPlanCache.plan = &plan
	deliveryPlanCache.expiresAt = time.Now().Add(500 * time.Millisecond) // 0.5秒キャッシュ
	deliveryPlanCache.Unlock()

	return &plan, nil
}


func (s *RobotService) UpdateOrderStatus(ctx context.Context, orderID int64, newStatus string) error {
	return utils.WithTimeout(ctx, func(ctx context.Context) error {
		return s.store.OrderRepo.UpdateStatuses(ctx, []int64{orderID}, newStatus)
	})
}

func selectOrdersForDelivery(
	ctx context.Context,
	orders []model.Order,
	robotID string,
	robotCapacity int,
) (model.DeliveryPlan, error) {
	n := len(orders)
	// dp[i][w] = i 個目まで見て容量 w のときの最大価値
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, robotCapacity+1)
	}

	// DPテーブル構築
	for i := 1; i <= n; i++ {
		wi := orders[i-1].Weight
		vi := orders[i-1].Value
		for w := 0; w <= robotCapacity; w++ {
			select {
			case <-ctx.Done():
				return model.DeliveryPlan{}, ctx.Err()
			default:
			}
			// 品物を入れない場合
			dp[i][w] = dp[i-1][w]
			// 入れる場合
			if w >= wi && dp[i-1][w-wi]+vi > dp[i][w] {
				dp[i][w] = dp[i-1][w-wi] + vi
			}
		}
	}

	bestValue := dp[n][robotCapacity]

	// 復元
	w := robotCapacity
	var bestSet []model.Order
	for i := n; i >= 1; i-- {
		if dp[i][w] != dp[i-1][w] { // i番目を入れた場合
			bestSet = append(bestSet, orders[i-1])
			w -= orders[i-1].Weight
		}
	}

	// 合計重さ計算
	totalWeight := 0
	for _, o := range bestSet {
		totalWeight += o.Weight
	}

	// bestSet が nil のときに空スライスへ
	if bestSet == nil {
		bestSet = []model.Order{}
	}

	return model.DeliveryPlan{
		RobotID:     robotID,
		TotalWeight: totalWeight,
		TotalValue:  bestValue,
		Orders:      bestSet,
	}, nil
}
