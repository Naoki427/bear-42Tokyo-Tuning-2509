package service

import (
	"context"
	"log"
	"sync"
	"time"
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service/utils"
	"strconv"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type RobotService struct {
	store *repository.Store
}

func NewRobotService(store *repository.Store) *RobotService {
	return &RobotService{store: store}
}

// キャッシュ用の構造体
type cachedPlan struct {
    plan      *model.DeliveryPlan
    orderIDs  []int64
    expiresAt time.Time
}

var deliveryPlanCache struct {
	sync.RWMutex
	data map[string]cachedPlan
}

func init() {
	deliveryPlanCache.data = make(map[string]cachedPlan)
}

func cacheKey(robotID string, capacity int) string {
	return robotID + ":" + strconv.Itoa(capacity)
}

func (s *RobotService) GenerateDeliveryPlan(ctx context.Context, robotID string, capacity int) (*model.DeliveryPlan, error) {
	tracer := otel.Tracer("app/custom")
	ctx, span := tracer.Start(ctx, "RobotService.GenerateDeliveryPlan")
	defer span.End()
	span.SetAttributes(
		attribute.String("robot.id", robotID),
		attribute.Int("robot.capacity", capacity),
	)

    key := cacheKey(robotID, capacity)

    // ---- キャッシュ確認 ----
    deliveryPlanCache.RLock()
    if cp, ok := deliveryPlanCache.data[key]; ok && time.Now().Before(cp.expiresAt) {
        deliveryPlanCache.RUnlock()

        statuses, err := s.store.OrderRepo.GetStatusesByIDs(ctx, cp.orderIDs)
        if err == nil {
            allShipping := true
            for _, st := range statuses {
                if st != "shipping" {
                    allShipping = false
                    break
                }
            }
            if allShipping {
                planCopy := *cp.plan
                return &planCopy, nil
            }
        }
    } else {
        deliveryPlanCache.RUnlock()
    }

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
	deliveryPlanCache.data[key] = cachedPlan{
		plan:      &plan,
		expiresAt: time.Now().Add(500 * time.Millisecond), // 0.5秒キャッシュ
	}
	deliveryPlanCache.Unlock()

	return &plan, nil
}



func (s *RobotService) UpdateOrderStatus(ctx context.Context, orderID int64, newStatus string) error {
	tracer := otel.Tracer("app/custom")
	ctx, span := tracer.Start(ctx, "RobotService.UpdateOrderStatus")
	defer span.End()
	span.SetAttributes(
		attribute.Int64("order.id", orderID),
		attribute.String("order.new_status", newStatus),
	)

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
	tracer := otel.Tracer("app/custom")
	ctx, span := tracer.Start(ctx, "selectOrdersForDelivery")
	defer span.End()
	span.SetAttributes(
		attribute.Int("orders.count", len(orders)),
		attribute.Int("robot.capacity", robotCapacity),
		attribute.String("robot.id", robotID),
	)

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
