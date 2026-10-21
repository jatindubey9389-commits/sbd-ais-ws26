package model

import "time"

type Order struct {
	DrinkID   uint64    `json:"drink_id"` // foreign key
	Amount    uint64    `json:"amount"`
	CreatedAt time.Time `json:"created_at"`
}
