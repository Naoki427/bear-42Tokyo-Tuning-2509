// webapp/backend/internal/repository/order.go

package repository

import (
	"backend/internal/model"
	"context"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
)

type OrderRepository struct {
	db DBTX
}

func NewOrderRepository(db DBTX) *OrderRepository {
	return &OrderRepository{db: db}
}

// CreateOrders は複数の注文を一括で作成する（バルクインサート）
func (r *OrderRepository) CreateOrders(ctx context.Context, userID int, productIDs []int) ([]string, error) {
	if len(productIDs) == 0 {
		return []string{}, nil
	}

	query := "INSERT INTO orders (user_id, product_id, shipped_status, created_at) VALUES "
	var args []interface{}
	var placeholders []string

	for _, pID := range productIDs {
		placeholders = append(placeholders, "(?, ?, 'shipping', NOW())")
		args = append(args, userID, pID)
	}

	query += strings.Join(placeholders, ",")

	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to bulk insert orders: %w", err)
	}

	lastID, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	rowsAffected := int64(len(productIDs))

	insertedIDs := make([]string, rowsAffected)
	for i := 0; i < int(rowsAffected); i++ {
		insertedIDs[i] = fmt.Sprintf("%d", int(lastID)-int(rowsAffected)+1+i)
	}
	return insertedIDs, nil
}


// 注文を作成し、生成された注文IDを返す
func (r *OrderRepository) Create(ctx context.Context, order *model.Order) (string, error) {
	query := `INSERT INTO orders (user_id, product_id, shipped_status, created_at) VALUES (?, ?, 'shipping', NOW())`
	result, err := r.db.ExecContext(ctx, query, order.UserID, order.ProductID)
	if err != nil {
		return "", err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d", id), nil
}

// 複数の注文IDのステータスを一括で更新
// 主に配送ロボットが注文を引き受けた際に一括更新をするために使用
func (r *OrderRepository) UpdateStatuses(ctx context.Context, orderIDs []int64, newStatus string) error {
	if len(orderIDs) == 0 {
		return nil
	}
	query, args, err := sqlx.In("UPDATE orders SET shipped_status = ? WHERE order_id IN (?)", newStatus, orderIDs)
	if err != nil {
		return err
	}
	query = r.db.Rebind(query)
	_, err = r.db.ExecContext(ctx, query, args...)
	return err
}

// 配送中(shipped_status:shipping)の注文一覧を取得
func (r *OrderRepository) GetShippingOrders(ctx context.Context) ([]model.Order, error) {
	var orders []model.Order
	query := `
        SELECT
            o.order_id,
            p.weight,
            p.value
        FROM orders o
        JOIN products p ON o.product_id = p.product_id
        WHERE o.shipped_status = 'shipping'
        ORDER BY p.value DESC, o.order_id ASC
    `
	err := r.db.SelectContext(ctx, &orders, query)
	return orders, err
}

//キャッシュのorderのstatusを取得するため
func (r *OrderRepository) GetStatusesByIDs(ctx context.Context, orderIDs []int64) ([]string, error) {
    if len(orderIDs) == 0 {
        return []string{}, nil
    }
    query, args, err := sqlx.In("SELECT shipped_status FROM orders WHERE order_id IN (?)", orderIDs)
    if err != nil {
        return nil, err
    }
    query = r.db.Rebind(query)

    var statuses []string
    if err := r.db.SelectContext(ctx, &statuses, query, args...); err != nil {
        return nil, err
    }
    return statuses, nil
}


// 注文履歴一覧を取得
func (r *OrderRepository) ListOrders(ctx context.Context, userID int, req model.ListRequest) ([]model.Order, int, error) {
    // ソート列を決定
    sortField := "o.order_id"
    // ... (ソート列決定ロジックは変更なし) ...
    switch req.SortField {
    case "product_name":
        sortField = "p.name"
    case "created_at":
        sortField = "o.created_at"
    case "shipped_status":
        sortField = "o.shipped_status"
    case "arrived_at":
        sortField = "o.arrived_at"
    }
    sortOrder := "ASC"
    if strings.ToUpper(req.SortOrder) == "DESC" {
        sortOrder = "DESC"
    }

    // 検索条件
    searchCond := ""
    var args []interface{}
    args = append(args, userID)

    if req.Search != "" {
        if req.Type == "prefix" {
            searchCond = "AND p.name LIKE ?"
            args = append(args, req.Search+"%")
        } else {
            searchCond = "AND p.name LIKE ?"
            args = append(args, "%"+req.Search+"%")
        }
    }

    // データ取得と件数取得を同時に行う1つのクエリ
    // 最初の 'args' の後に LIMIT と OFFSET 用のパラメータが続くため、
    // queryのパラメータ数に注意してargsスライスを作成する。
    
    // NOTE: SELECT句に COUNT(*) OVER() を追加し、total_countとして取得する
    query := fmt.Sprintf(`
        SELECT
            o.order_id,
            o.product_id,
            o.shipped_status,
            o.created_at,
            o.arrived_at,
            p.name AS product_name,
            COUNT(*) OVER() AS total_count  -- ★ 1クエリ化のキーポイント
        FROM orders o
        JOIN products p ON o.product_id = p.product_id
        WHERE o.user_id = ? %s
        ORDER BY %s %s
        LIMIT ? OFFSET ?
    `, searchCond, sortField, sortOrder)

    // LIMITとOFFSETのためのパラメータを追加
    queryArgs := append(args, req.PageSize, req.Offset)

    var ordersWithTotalCount []struct {
        model.Order
        TotalCount int `db:"total_count"`
    }
    
    if err := r.db.SelectContext(ctx, &ordersWithTotalCount, query, queryArgs...); err != nil {
        return nil, 0, err
    }

    // 結果の処理
    if len(ordersWithTotalCount) == 0 {
        return nil, 0, nil
    }

    // total_countは最初の要素から取得
    total := ordersWithTotalCount[0].TotalCount
    
    // model.Orderのスライスに変換
    orders := make([]model.Order, len(ordersWithTotalCount))
    for i, item := range ordersWithTotalCount {
        orders[i] = item.Order
    }

    return orders, total, nil
}