package notion

import (
	"context"
	"fmt"

	"github.com/jomei/notionapi"
	"github.com/ookawayuuto/notion-tui/internal/domain"
)

type RecipeClient struct {
	api      *notionapi.Client
	recipeDB notionapi.DatabaseID
}

func NewRecipeClient(token, recipeDBID string) *RecipeClient {
	return &RecipeClient{
		api:      notionapi.NewClient(notionapi.Token(token)),
		recipeDB: notionapi.DatabaseID(recipeDBID),
	}
}

// FetchRecipes retrieves all recipe records from Notion DB ordered by created_time desc
func (c *RecipeClient) FetchRecipes(ctx context.Context) ([]domain.RecipeItem, error) {
	hasMore := true
	cursor := notionapi.Cursor("")
	var recipes []domain.RecipeItem

	for hasMore {
		req := &notionapi.DatabaseQueryRequest{
			StartCursor: cursor,
			PageSize:    100,
			Sorts: []notionapi.SortObject{
				{
					Timestamp: notionapi.TimestampCreated,
					Direction: notionapi.SortOrderDESC,
				},
			},
		}
		if cursor == "" {
			req.StartCursor = ""
		}

		res, err := c.api.Database.Query(ctx, c.recipeDB, req)
		if err != nil {
			return nil, fmt.Errorf("レシピDB取得エラー: %w", err)
		}

		for _, page := range res.Results {
			title := extractTitle(page, "名前", "title", "Title", "Name")
			url := extractURL(page, "リンク", "URL", "url")
			categories := extractMultiSelect(page, "カテゴリー")
			genre := extractSelect(page, "ジャンル")
			difficulty := extractSelect(page, "難易度")
			cooked := extractCheckbox(page, "つくってみた")
			memo := extractRichText(page, "メモ")

			recipes = append(recipes, domain.RecipeItem{
				ID:         notionapi.ObjectID(page.ID),
				Title:      title,
				URL:        url,
				Categories: categories,
				Genre:      genre,
				Difficulty: difficulty,
				Cooked:     cooked,
				Memo:       memo,
				CreatedAt:  page.CreatedTime,
			})
		}

		hasMore = res.HasMore
		cursor = res.NextCursor
	}

	return recipes, nil
}

// UpdateRecipe updates properties of a recipe
func (c *RecipeClient) UpdateRecipe(ctx context.Context, pageID notionapi.ObjectID, categories []string, genre string, difficulty string, cooked *bool) error {
	props := notionapi.Properties{}

	if categories != nil {
		options := make([]notionapi.Option, len(categories))
		for i, cat := range categories {
			options[i] = notionapi.Option{Name: cat}
		}
		props["カテゴリー"] = notionapi.MultiSelectProperty{
			MultiSelect: options,
		}
	}

	if genre != "" {
		props["ジャンル"] = notionapi.SelectProperty{
			Select: notionapi.Option{Name: genre},
		}
	}

	if difficulty != "" {
		props["難易度"] = notionapi.SelectProperty{
			Select: notionapi.Option{Name: difficulty},
		}
	}

	if cooked != nil {
		props["つくってみた"] = notionapi.CheckboxProperty{
			Checkbox: *cooked,
		}
	}

	if len(props) == 0 {
		return nil
	}

	_, err := c.api.Page.Update(ctx, notionapi.PageID(pageID), &notionapi.PageUpdateRequest{
		Properties: props,
	})
	if err != nil {
		return fmt.Errorf("レシピ更新エラー: %w", err)
	}
	return nil
}

// ToggleCooked updates the "つくってみた" status
func (c *RecipeClient) ToggleCooked(ctx context.Context, pageID notionapi.ObjectID, cooked bool) error {
	return c.UpdateRecipe(ctx, pageID, nil, "", "", &cooked)
}

func extractURL(page notionapi.Page, keys ...string) string {
	for _, key := range keys {
		if urlProp, ok := page.Properties[key].(*notionapi.URLProperty); ok && urlProp.URL != "" {
			return urlProp.URL
		}
	}
	return ""
}

func extractRichText(page notionapi.Page, keys ...string) string {
	for _, key := range keys {
		if textProp, ok := page.Properties[key].(*notionapi.RichTextProperty); ok {
			return richTextToPlainText(textProp.RichText)
		}
	}
	return ""
}
