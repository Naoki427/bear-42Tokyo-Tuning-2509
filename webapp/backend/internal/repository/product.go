package repository

import (
	"backend/internal/model"
	"context"
)

type ProductRepository struct {
	db DBTX
}

func NewProductRepository(db DBTX) *ProductRepository {
	return &ProductRepository{db: db}
}

// 商品一覧を全件取得し、アプリケーション側でページング処理を行う
func (r *ProductRepository) ListProducts(ctx context.Context, userID int, req model.ListRequest) ([]model.Product, int, error) {
	var products []model.Product
	baseQuery := `
		SELECT product_id, name, value, weight, image, description
		FROM products
	`
	args := []interface{}{}

	if req.Search != "" {
		baseQuery += " WHERE (name LIKE ? OR description LIKE ?)"
		searchPattern := "%" + req.Search + "%"
		args = append(args, searchPattern, searchPattern)
	}

	baseQuery += " ORDER BY " + req.SortField + " " + req.SortOrder + " , product_id ASC"

	err := r.db.SelectContext(ctx, &products, baseQuery, args...)
	if err != nil {
		return nil, 0, err
	}

	total := len(products)
	start := req.Offset
	end := req.Offset + req.PageSize
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}
	pagedProducts := products[start:end]

	return pagedProducts, total, nil
}

// package repository

// import (
// 	"backend/internal/model"
// 	"context"
// 	"sort"
// 	"strings"
// 	"sync"
// )

// type ProductRepository struct {
// 	db DBTX
// }

// // --- プリロード用キャッシュ ---
// var (
// 	productCache     []model.Product
// 	productCacheLock sync.RWMutex
// )

// func NewProductRepository(db DBTX) *ProductRepository {
// 	return &ProductRepository{db: db}
// }

// // プリロード: アプリ起動時や定期的に呼んで全件ロード
// func (r *ProductRepository) PreloadProducts(ctx context.Context) error {
// 	var products []model.Product
// 	query := `
// 		SELECT product_id, name, value, weight, image, description
// 		FROM products
// 	`
// 	if err := r.db.SelectContext(ctx, &products, query); err != nil {
// 		return err
// 	}

// 	productCacheLock.Lock()
// 	productCache = products
// 	productCacheLock.Unlock()
// 	return nil
// }

// func (r *ProductRepository) ListProducts(ctx context.Context, userID int, req model.ListRequest) ([]model.Product, int, error) {
// 	productCacheLock.RLock()
	
// 	// キャッシュが空の場合は初回ロード
// 	if len(productCache) == 0 {
// 		productCacheLock.RUnlock()
// 		if err := r.PreloadProducts(ctx); err != nil {
// 			return nil, 0, err
// 		}
// 		productCacheLock.RLock()
// 	}
// 	defer productCacheLock.RUnlock()

// 	// --- フィルタ（検索条件）---
// 	filtered := make([]model.Product, 0)
// 	if req.Search != "" {
// 		searchLower := strings.ToLower(req.Search)
// 		for _, p := range productCache {
// 			if strings.Contains(strings.ToLower(p.Name), searchLower) ||
// 				strings.Contains(strings.ToLower(p.Description), searchLower) {
// 				filtered = append(filtered, p)
// 			}
// 		}
// 	} else {
// 		filtered = append(filtered, productCache...)
// 	}

// 	// --- ソート ---
// 	switch req.SortField {
// 	case "name":
// 		sort.Slice(filtered, func(i, j int) bool {
// 			if strings.ToUpper(req.SortOrder) == "DESC" {
// 				return filtered[i].Name > filtered[j].Name
// 			}
// 			return filtered[i].Name < filtered[j].Name
// 		})
// 	case "value":
// 		sort.Slice(filtered, func(i, j int) bool {
// 			if strings.ToUpper(req.SortOrder) == "DESC" {
// 				return filtered[i].Value > filtered[j].Value
// 			}
// 			return filtered[i].Value < filtered[j].Value
// 		})
// 	case "weight":
// 		sort.Slice(filtered, func(i, j int) bool {
// 			if strings.ToUpper(req.SortOrder) == "DESC" {
// 				return filtered[i].Weight > filtered[j].Weight
// 			}
// 			return filtered[i].Weight < filtered[j].Weight
// 		})
// 	default: // product_id
// 		sort.Slice(filtered, func(i, j int) bool {
// 			if strings.ToUpper(req.SortOrder) == "DESC" {
// 				return filtered[i].ProductID > filtered[j].ProductID
// 			}
// 			return filtered[i].ProductID < filtered[j].ProductID
// 		})
// 	}

// 	// --- ページング ---
// 	total := len(filtered)
// 	start := req.Offset
// 	end := req.Offset + req.PageSize
// 	if start > total {
// 		start = total
// 	}
// 	if end > total {
// 		end = total
// 	}
// 	pagedProducts := filtered[start:end]

// 	return pagedProducts, total, nil
// }
