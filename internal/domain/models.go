package domain

import "github.com/jomei/notionapi"

type InventoryItem struct {
	ID    notionapi.ObjectID
	Name  string
	Stock int
}

type ShoppingItem struct {
	ID   notionapi.ObjectID
	Name string
}
