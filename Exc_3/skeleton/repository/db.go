package repository

import (
	"time"

	"ordersystem/model"
)

type DatabaseHandler struct {
	// drinks represent all available drinks
	drinks []model.Drink
	// orders serves as order history
	orders []model.Order
}

func NewDatabaseHandler() *DatabaseHandler {
	drinks := []model.Drink{
		{ID: 1, Name: "Espresso", Price: 2.50, Description: "Short, strong, keeps you upright."},
		{ID: 2, Name: "Cappuccino", Price: 3.20, Description: "Espresso with milk foam. Fancy mornings only."},
		{ID: 3, Name: "Club-Mate", Price: 2.80, Description: "The hacker fuel. Caffeine with attitude."},
		{ID: 4, Name: "Beer", Price: 4.00, Description: "After-lecture special. Not for 9am."},
	}

	orders := []model.Order{
		{DrinkID: 1, Amount: 2, CreatedAt: time.Now().Add(-2 * time.Hour)},
		{DrinkID: 3, Amount: 1, CreatedAt: time.Now().Add(-1 * time.Hour)},
		{DrinkID: 1, Amount: 1, CreatedAt: time.Now().Add(-30 * time.Minute)},
	}

	return &DatabaseHandler{
		drinks: drinks,
		orders: orders,
	}
}

func (db *DatabaseHandler) GetDrinks() []model.Drink {
	return db.drinks
}

func (db *DatabaseHandler) GetOrders() []model.Order {
	return db.orders
}

func (db *DatabaseHandler) GetTotalledOrders() map[uint64]uint64 {
	totalledOrders := make(map[uint64]uint64)
	for _, o := range db.orders {
		totalledOrders[o.DrinkID] += o.Amount
	}
	return totalledOrders
}

func (db *DatabaseHandler) AddOrder(order *model.Order) {
	db.orders = append(db.orders, *order)
}
