package notion

import (
	"context"
	"fmt"

	"github.com/jomei/notionapi"
)

type KBClient struct {
	api    *notionapi.Client
	kbDBID notionapi.DatabaseID
}

func NewKBClient(token, kbID string) *KBClient {
	return &KBClient{
		api:    notionapi.NewClient(notionapi.Token(token)),
		kbDBID: notionapi.DatabaseID(kbID),
	}
}

// FetchCategories fetches the options for the "分類カテゴリ" property from the database schema
func (c *KBClient) FetchCategories(ctx context.Context) ([]string, error) {
	db, err := c.api.Database.Get(ctx, c.kbDBID)
	if err != nil {
		return nil, fmt.Errorf("ナレッジベースDBの取得エラー: %w", err)
	}

	var categories []string
	if prop, ok := db.Properties["分類カテゴリ"].(*notionapi.SelectPropertyConfig); ok {
		for _, opt := range prop.Select.Options {
			categories = append(categories, opt.Name)
		}
	}
	return categories, nil
}

// AddDraftItem creates a new page in the Knowledge Base with the "下書き" label
func (c *KBClient) AddDraftItem(ctx context.Context, title string, category string) (string, error) {
	props := notionapi.Properties{
		"名前": notionapi.TitleProperty{
			Title: []notionapi.RichText{
				{Text: &notionapi.Text{Content: title}},
			},
		},
		"選択": notionapi.SelectProperty{
			Select: notionapi.Option{Name: "下書き"},
		},
	}

	if category != "" {
		props["分類カテゴリ"] = notionapi.SelectProperty{
			Select: notionapi.Option{Name: category},
		}
	}

	res, err := c.api.Page.Create(ctx, &notionapi.PageCreateRequest{
		Parent:     notionapi.Parent{Type: notionapi.ParentTypeDatabaseID, DatabaseID: c.kbDBID},
		Properties: props,
	})

	if err != nil {
		return "", fmt.Errorf("ページ作成エラー: %w", err)
	}

	return res.URL, nil
}
