package notion

import (
	"context"
	"fmt"

	"github.com/jomei/notionapi"
	"github.com/ookawayuuto/notion-tui/internal/domain"
)

const CheckboxPropertyName = "ステータス"
const StockPropertyName = "在庫数"

type Client struct {
	api      *notionapi.Client
	invDBID  notionapi.DatabaseID
	shopDBID notionapi.DatabaseID
}

func NewClient(token, invID, shopID string) *Client {
	return &Client{
		api:      notionapi.NewClient(notionapi.Token(token)),
		invDBID:  notionapi.DatabaseID(invID),
		shopDBID: notionapi.DatabaseID(shopID),
	}
}

func (c *Client) FetchData(ctx context.Context) ([]domain.InventoryItem, []domain.ShoppingItem, error) {
	invResp, err := c.api.Database.Query(ctx, c.invDBID, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("在庫DBエラー: %w", err)
	}
	var items []domain.InventoryItem
	for _, page := range invResp.Results {
		name := extractTitle(page, "名前", "Name")
		stock := 0
		if numProp, ok := page.Properties[StockPropertyName].(*notionapi.NumberProperty); ok {
			stock = int(numProp.Number)
		}
		var categories []string
		if prop, ok := page.Properties["分類"].(*notionapi.MultiSelectProperty); ok {
			for _, opt := range prop.MultiSelect {
				categories = append(categories, opt.Name)
			}
		}
		items = append(items, domain.InventoryItem{ID: page.ID, Name: name, Stock: stock, Categories: categories})
	}

	shopQuery := &notionapi.DatabaseQueryRequest{
		Filter: notionapi.PropertyFilter{
			Property: CheckboxPropertyName,
			Checkbox: &notionapi.CheckboxFilterCondition{DoesNotEqual: true},
		},
	}
	shopResp, err := c.api.Database.Query(ctx, c.shopDBID, shopQuery)
	if err != nil {
		return nil, nil, fmt.Errorf("買い物DBエラー: %w", err)
	}

	var shopping []domain.ShoppingItem
	for _, page := range shopResp.Results {
		name := extractTitle(page, "名前", "Name")
		shopping = append(shopping, domain.ShoppingItem{ID: page.ID, Name: name})
	}

	return items, shopping, nil
}

func (c *Client) AddShoppingItem(ctx context.Context, name string) (domain.ShoppingItem, error) {
	res, err := c.api.Page.Create(ctx, &notionapi.PageCreateRequest{
		Parent: notionapi.Parent{Type: notionapi.ParentTypeDatabaseID, DatabaseID: c.shopDBID},
		Properties: notionapi.Properties{
			"名前": notionapi.TitleProperty{Title: []notionapi.RichText{{Text: &notionapi.Text{Content: name}}}},
		},
	})
	if err != nil {
		res, err = c.api.Page.Create(ctx, &notionapi.PageCreateRequest{
			Parent: notionapi.Parent{Type: notionapi.ParentTypeDatabaseID, DatabaseID: c.shopDBID},
			Properties: notionapi.Properties{
				"Name": notionapi.TitleProperty{Title: []notionapi.RichText{{Text: &notionapi.Text{Content: name}}}},
			},
		})
	}
	if err != nil {
		return domain.ShoppingItem{}, fmt.Errorf("追加エラー: %w", err)
	}
	return domain.ShoppingItem{ID: res.ID, Name: name}, nil
}

func (c *Client) CheckShoppingItem(ctx context.Context, pageID notionapi.ObjectID) error {
	_, err := c.api.Page.Update(ctx, notionapi.PageID(pageID), &notionapi.PageUpdateRequest{
		Properties: notionapi.Properties{
			CheckboxPropertyName: notionapi.CheckboxProperty{Checkbox: true},
		},
	})
	if err != nil {
		return fmt.Errorf("更新エラー: %w", err)
	}
	return nil
}

func (c *Client) UpdateStock(ctx context.Context, pageID notionapi.ObjectID, newStock int) error {
	_, err := c.api.Page.Update(ctx, notionapi.PageID(pageID), &notionapi.PageUpdateRequest{
		Properties: notionapi.Properties{
			StockPropertyName: notionapi.NumberProperty{Number: float64(newStock)},
		},
	})
	if err != nil {
		return fmt.Errorf("在庫更新エラー: %w", err)
	}
	return nil
}

func extractTitle(page notionapi.Page, keys ...string) string {
	for _, key := range keys {
		if titleProp, ok := page.Properties[key].(*notionapi.TitleProperty); ok && len(titleProp.Title) > 0 {
			return titleProp.Title[0].PlainText
		}
	}
	return "No Title"
}
