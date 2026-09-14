package domain

import (
	"time"

	"github.com/jomei/notionapi"
)

type RecipeItem struct {
	ID         notionapi.ObjectID `json:"id"`
	Title      string             `json:"title"`
	URL        string             `json:"url"`
	Categories []string           `json:"categories"`
	Genre      string             `json:"genre"`
	Difficulty string             `json:"difficulty"`
	Cooked     bool               `json:"cooked"`
	Memo       string             `json:"memo"`
	CreatedAt  time.Time          `json:"created_at"`
}

// IsUnlabeled checks if the recipe has no categories or no genre assigned
func (r RecipeItem) IsUnlabeled() bool {
	return len(r.Categories) == 0 || r.Genre == ""
}
