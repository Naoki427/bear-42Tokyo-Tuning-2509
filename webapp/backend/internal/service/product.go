
package service

import (
	"context"
	"log" // logパッケージをインポート

	"backend/internal/model"
	"backend/internal/repository"
)

type ProductService struct {
	store *repository.Store
}

func NewProductService(store *repository.Store) *ProductService {
	return &ProductService{store: store}
}

// ★ この CreateOrders 関数をまるごと書き換える
func (s *ProductService) CreateOrders(ctx context.Context, userID int, items []model.RequestItem) ([]string, error) {
	
	var productIDsToOrder []int
	for _, item := range items {
		if item.Quantity > 0 {
			for i := 0; i < item.Quantity; i++ {
				productIDsToOrder = append(productIDsToOrder, item.ProductID)
			}
		}
	}
	
	if len(productIDsToOrder) == 0 {
		return []string{}, nil
	}

	// repositoryに新設したバルクインサート用の関数を呼び出す
	insertedOrderIDs, err := s.store.OrderRepo.CreateOrders(ctx, userID, productIDsToOrder)
	if err != nil {
		return nil, err
	}

	log.Printf("Created %d orders for user %d", len(insertedOrderIDs), userID)
	return insertedOrderIDs, nil
}

func (s *ProductService) FetchProducts(ctx context.Context, userID int, req model.ListRequest) ([]model.Product, int, error) {
	products, total, err := s.store.ProductRepo.ListProducts(ctx, userID, req)
	return products, total, err
}