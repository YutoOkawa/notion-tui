package domain

import (
	"time"

	"github.com/jomei/notionapi"
)

type MemoItem struct {
	ID        notionapi.ObjectID
	Title     string
	Category  string
	Status    string
	UpdatedAt time.Time
}
