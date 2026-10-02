package notion

import (
	"context"
	"fmt"
	"time"

	"github.com/jomei/notionapi"
	"github.com/ookawayuuto/notion-tui/internal/domain"
)

const CheckboxPropertyName = "ステータス"
const StockPropertyName = "在庫数"
const ExpirationPropertyName = "賞味期限"
const SubscriptionPropertyName = "定期便"
const SubCyclePropertyName = "配送サイクル"
const NextDeliveryPropertyName = "次回配送日"

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
		expDate := extractDateProperty(page, ExpirationPropertyName)

		isSub := false
		if subProp, ok := page.Properties[SubscriptionPropertyName].(*notionapi.CheckboxProperty); ok {
			isSub = subProp.Checkbox
		}
		subCycle := ""
		if cycleProp, ok := page.Properties[SubCyclePropertyName].(*notionapi.SelectProperty); ok && cycleProp.Select.Name != "" {
			subCycle = cycleProp.Select.Name
		}
		nextDelivery := extractDateProperty(page, NextDeliveryPropertyName)

		items = append(items, domain.InventoryItem{
			ID:               page.ID,
			Name:             name,
			Stock:            stock,
			Categories:       categories,
			ExpirationDate:   expDate,
			IsSubscription:   isSub,
			SubCycle:         subCycle,
			NextDeliveryDate: nextDelivery,
		})
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

func (c *Client) AddInventoryItem(ctx context.Context, name string, stock int, categories []string) (domain.InventoryItem, error) {
	props := notionapi.Properties{
		"名前":              notionapi.TitleProperty{Title: []notionapi.RichText{{Text: &notionapi.Text{Content: name}}}},
		StockPropertyName: notionapi.NumberProperty{Number: float64(stock)},
	}
	if len(categories) > 0 {
		var options []notionapi.Option
		for _, cat := range categories {
			options = append(options, notionapi.Option{Name: cat})
		}
		props["分類"] = notionapi.MultiSelectProperty{MultiSelect: options}
	}

	res, err := c.api.Page.Create(ctx, &notionapi.PageCreateRequest{
		Parent:     notionapi.Parent{Type: notionapi.ParentTypeDatabaseID, DatabaseID: c.invDBID},
		Properties: props,
	})
	if err != nil {
		props["Name"] = props["名前"]
		delete(props, "名前")
		res, err = c.api.Page.Create(ctx, &notionapi.PageCreateRequest{
			Parent:     notionapi.Parent{Type: notionapi.ParentTypeDatabaseID, DatabaseID: c.invDBID},
			Properties: props,
		})
	}
	if err != nil {
		return domain.InventoryItem{}, fmt.Errorf("在庫追加エラー: %w", err)
	}

	return domain.InventoryItem{
		ID:         res.ID,
		Name:       name,
		Stock:      stock,
		Categories: categories,
	}, nil
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

func (c *Client) UpdateExpirationDate(ctx context.Context, pageID notionapi.ObjectID, expirationDate string) error {
	var dateValue *notionapi.DateObject
	if expirationDate != "" {
		parsedDate, err := time.Parse("2006-01-02", expirationDate)
		if err != nil {
			return fmt.Errorf("日付のパースエラー(YYYY-MM-DD形式で入力してください): %w", err)
		}
		nd := notionapi.Date(parsedDate)
		dateValue = &notionapi.DateObject{Start: &nd}
	} else {
		dateValue = nil
	}

	_, err := c.api.Page.Update(ctx, notionapi.PageID(pageID), &notionapi.PageUpdateRequest{
		Properties: notionapi.Properties{
			ExpirationPropertyName: notionapi.DateProperty{
				Date: dateValue,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("賞味期限更新エラー: %w", err)
	}
	return nil
}

func (c *Client) UpdateNextDeliveryDate(ctx context.Context, pageID notionapi.ObjectID, nextDeliveryDate string) error {
	var dateValue *notionapi.DateObject
	if nextDeliveryDate != "" {
		parsedDate, err := time.Parse("2006-01-02", nextDeliveryDate)
		if err != nil {
			return fmt.Errorf("日付のパースエラー(YYYY-MM-DD形式で入力してください): %w", err)
		}
		nd := notionapi.Date(parsedDate)
		dateValue = &notionapi.DateObject{Start: &nd}
	} else {
		dateValue = nil
	}

	_, err := c.api.Page.Update(ctx, notionapi.PageID(pageID), &notionapi.PageUpdateRequest{
		Properties: notionapi.Properties{
			NextDeliveryPropertyName: notionapi.DateProperty{
				Date: dateValue,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("次回配送日更新エラー: %w", err)
	}
	return nil
}

func (c *Client) UpdateSubscription(ctx context.Context, pageID notionapi.ObjectID, isSub bool, cycle string, nextDeliveryDate string) error {
	var dateValue *notionapi.DateObject
	if nextDeliveryDate != "" {
		parsedDate, err := time.Parse("2006-01-02", nextDeliveryDate)
		if err != nil {
			return fmt.Errorf("日付のパースエラー(YYYY-MM-DD形式で入力してください): %w", err)
		}
		nd := notionapi.Date(parsedDate)
		dateValue = &notionapi.DateObject{Start: &nd}
	} else {
		dateValue = nil
	}

	var selectOption *notionapi.Option
	if cycle != "" {
		selectOption = &notionapi.Option{Name: cycle}
	}

	var cycleProp notionapi.Property
	if selectOption != nil {
		cycleProp = notionapi.SelectProperty{Select: *selectOption}
	} else {
		cycleProp = notionapi.SelectProperty{}
	}

	_, err := c.api.Page.Update(ctx, notionapi.PageID(pageID), &notionapi.PageUpdateRequest{
		Properties: notionapi.Properties{
			SubscriptionPropertyName: notionapi.CheckboxProperty{Checkbox: isSub},
			SubCyclePropertyName:     cycleProp,
			NextDeliveryPropertyName: notionapi.DateProperty{Date: dateValue},
		},
	})
	if err != nil {
		return fmt.Errorf("定期便情報更新エラー: %w", err)
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

func extractDateProperty(page notionapi.Page, key string) string {
	if dateProp, ok := page.Properties[key].(*notionapi.DateProperty); ok && dateProp.Date != nil && dateProp.Date.Start != nil {
		return time.Time(*dateProp.Date.Start).Format("2006-01-02")
	}
	return ""
}

func extractExpirationDate(page notionapi.Page, key string) string {
	return extractDateProperty(page, key)
}
