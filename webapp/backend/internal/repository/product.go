package repository

import (
	"backend/internal/model"
	"context"
	"strings"
	"fmt"
)

type ProductRepository struct {
	db DBTX
}

func NewProductRepository(db DBTX) *ProductRepository {
	return &ProductRepository{db: db}
}

func (r *ProductRepository) ListProducts(
	ctx context.Context, userID int, req model.ListRequest,
) ([]model.Product, int, error) {
	var (
		products        []model.Product
		searchWhereSQL  string   // ← 検索だけ（count 用）
		searchArgs      []any    // ← 検索だけ（count 用）
		dataWhereSQL    string   // ← 検索 + カーソル（データ取得用）
		dataArgs        []any    // ← 検索 + カーソル（データ取得用）
	)

	// ---- 検索句を組み立て（count 用の where/args はこれだけを使う）----
	if req.Search != "" {
		var pat string
		if req.Type == "prefix" {
			pat = req.Search + "%"
		} else {
			pat = "%" + req.Search + "%" // 既定は部分一致（テスト期待どおり）
		}
		searchWhereSQL = "WHERE (name LIKE ? OR description LIKE ?)"
		searchArgs = append(searchArgs, pat, pat)
	}
	// data 用の where は最初は検索だけをコピー
	dataWhereSQL = searchWhereSQL
	dataArgs = append(dataArgs, searchArgs...)

	// ---- ソートホワイトリスト ----
	sortField := "product_id"
	switch req.SortField {
	case "value", "weight", "name", "product_id":
		sortField = req.SortField
	}
	sortOrder := "ASC"
	if strings.ToUpper(req.SortOrder) == "DESC" {
		sortOrder = "DESC"
	}

	// ---- カラム別に最適化（カーソル条件は dataWhereSQL にのみ足す！）----
	switch sortField {
	case "value":
		if req.AfterValue != nil && req.AfterID != nil {
			op := ">"
			if sortOrder == "DESC" { op = "<" }
			if dataWhereSQL == "" {
				dataWhereSQL = "WHERE (value, product_id) " + op + " (?, ?)"
			} else {
				dataWhereSQL += " AND (value, product_id) " + op + " (?, ?)"
			}
			dataArgs = append(dataArgs, *req.AfterValue, *req.AfterID)

			q := fmt.Sprintf(`
				SELECT product_id, name, value, weight, image, description
				FROM products
				%s
				ORDER BY value %s, product_id %s
				LIMIT ?`, dataWhereSQL, sortOrder, sortOrder)
			args := append(dataArgs, req.PageSize)
			if err := r.db.SelectContext(ctx, &products, q, args...); err != nil {
				return nil, 0, err
			}
		} else {
			q := fmt.Sprintf(`
				SELECT product_id, name, value, weight, image, description
				FROM products
				%s
				ORDER BY value %s, product_id %s
				LIMIT ? OFFSET ?`, dataWhereSQL, sortOrder, sortOrder)
			args := append(dataArgs, req.PageSize, req.Offset)
			if err := r.db.SelectContext(ctx, &products, q, args...); err != nil {
				return nil, 0, err
			}
		}

	case "weight":
		if req.AfterWeight != nil && req.AfterID != nil {
			op := ">"
			if sortOrder == "DESC" { op = "<" }
			if dataWhereSQL == "" {
				dataWhereSQL = "WHERE (weight, product_id) " + op + " (?, ?)"
			} else {
				dataWhereSQL += " AND (weight, product_id) " + op + " (?, ?)"
			}
			dataArgs = append(dataArgs, *req.AfterWeight, *req.AfterID)

			q := fmt.Sprintf(`
				SELECT product_id, name, value, weight, image, description
				FROM products
				%s
				ORDER BY weight %s, product_id %s
				LIMIT ?`, dataWhereSQL, sortOrder, sortOrder)
			args := append(dataArgs, req.PageSize)
			if err := r.db.SelectContext(ctx, &products, q, args...); err != nil {
				return nil, 0, err
			}
		} else {
			q := fmt.Sprintf(`
				SELECT product_id, name, value, weight, image, description
				FROM products
				%s
				ORDER BY weight %s, product_id %s
				LIMIT ? OFFSET ?`, dataWhereSQL, sortOrder, sortOrder)
			args := append(dataArgs, req.PageSize, req.Offset)
			if err := r.db.SelectContext(ctx, &products, q, args...); err != nil {
				return nil, 0, err
			}
		}

	case "name":
		q := fmt.Sprintf(`
			SELECT product_id, name, value, weight, image, description
			FROM products
			%s
			ORDER BY name %s, product_id %s
			LIMIT ? OFFSET ?`, dataWhereSQL, sortOrder, sortOrder)
		args := append(dataArgs, req.PageSize, req.Offset)
		if err := r.db.SelectContext(ctx, &products, q, args...); err != nil {
			return nil, 0, err
		}

	default: // product_id
		if req.AfterID != nil {
			op := ">"
			if sortOrder == "DESC" { op = "<" }
			if dataWhereSQL == "" {
				dataWhereSQL = "WHERE product_id " + op + " ?"
			} else {
				dataWhereSQL += " AND product_id " + op + " ?"
			}
			dataArgs = append(dataArgs, *req.AfterID)

			q := fmt.Sprintf(`
				SELECT product_id, name, value, weight, image, description
				FROM products
				%s
				ORDER BY product_id %s
				LIMIT ?`, dataWhereSQL, sortOrder)
			args := append(dataArgs, req.PageSize)
			if err := r.db.SelectContext(ctx, &products, q, args...); err != nil {
				return nil, 0, err
			}
		} else {
			q := fmt.Sprintf(`
				SELECT product_id, name, value, weight, image, description
				FROM products
				%s
				ORDER BY product_id %s
				LIMIT ? OFFSET ?`, dataWhereSQL, sortOrder)
			args := append(dataArgs, req.PageSize, req.Offset)
			if err := r.db.SelectContext(ctx, &products, q, args...); err != nil {
				return nil, 0, err
			}
		}
	}

	// ---- 総件数：検索だけ（カーソル条件は一切入れない）----
	var total int
	countSQL := "SELECT COUNT(*) FROM products"
	if searchWhereSQL != "" {
		countSQL += " " + searchWhereSQL
	}
	if err := r.db.GetContext(ctx, &total, countSQL, searchArgs...); err != nil {
		return nil, 0, err
	}

	return products, total, nil
}
