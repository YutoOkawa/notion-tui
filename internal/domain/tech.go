package domain

import (
	"time"

	"github.com/jomei/notionapi"
)

type TechItem struct {
	ID        notionapi.ObjectID `json:"id"`
	Title     string             `json:"title"`
	URL       string             `json:"url"`
	Status    string             `json:"status"` // 未読, 読書中, 読了, 要復習
	Genre     string             `json:"genre"`  // Frontend, Backend, Infra/Cloud, AI/ML, etc.
	Tags      []string           `json:"tags"`   // Go, TypeScript, AWS, etc.
	Memo      string             `json:"memo"`
	CreatedAt time.Time          `json:"created_at"`
}

// IsUnlabeled checks if the tech blog item has no genre or no tags assigned
func (t TechItem) IsUnlabeled() bool {
	return t.Genre == "" || len(t.Tags) == 0
}

// IsUnread checks if the item is unread (or empty status)
func (t TechItem) IsUnread() bool {
	return t.Status == "未読" || t.Status == ""
}

// IsRead checks if the item is read
func (t TechItem) IsRead() bool {
	return t.Status == "読了"
}
