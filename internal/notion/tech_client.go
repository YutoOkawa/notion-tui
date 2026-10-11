package notion

import (
	"context"
	"fmt"

	"github.com/jomei/notionapi"
	"github.com/ookawayuuto/notion-tui/internal/domain"
)

type TechClient struct {
	api    *notionapi.Client
	techDB notionapi.DatabaseID
}

func NewTechClient(token, techDBID string) *TechClient {
	return &TechClient{
		api:    notionapi.NewClient(notionapi.Token(token)),
		techDB: notionapi.DatabaseID(techDBID),
	}
}

// FetchTechItems retrieves all tech blog stack records from Notion DB ordered by created_time desc
func (c *TechClient) FetchTechItems(ctx context.Context) ([]domain.TechItem, error) {
	hasMore := true
	cursor := notionapi.Cursor("")
	var items []domain.TechItem

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

		res, err := c.api.Database.Query(ctx, c.techDB, req)
		if err != nil {
			return nil, fmt.Errorf("TechBlog Stack DB取得エラー: %w", err)
		}

		for _, page := range res.Results {
			title := extractTitle(page, "名前", "title", "Title", "Name")
			url := extractURL(page, "URL", "url", "リンク")
			genre := extractSelect(page, "Genre")
			if genre == "" {
				genre = extractSelect(page, "ジャンル")
			}
			tags := extractMultiSelect(page, "Tags")
			if len(tags) == 0 {
				tags = extractMultiSelect(page, "タグ")
			}
			status := extractSelect(page, "Status")
			if status == "" {
				status = extractSelect(page, "ステータス")
			}
			memo := extractRichText(page, "Memo")
			if memo == "" {
				memo = extractRichText(page, "メモ")
			}

			items = append(items, domain.TechItem{
				ID:        notionapi.ObjectID(page.ID),
				Title:     title,
				URL:       url,
				Status:    status,
				Genre:     genre,
				Tags:      tags,
				Memo:      memo,
				CreatedAt: page.CreatedTime,
			})
		}

		hasMore = res.HasMore
		cursor = res.NextCursor
	}

	return items, nil
}

// BuildTechPageUpdateRequest creates PageUpdateRequest purely for testability
func BuildTechPageUpdateRequest(genre string, tags []string, status string, memo string) *notionapi.PageUpdateRequest {
	props := notionapi.Properties{}

	if genre != "" {
		props["Genre"] = notionapi.SelectProperty{
			Select: notionapi.Option{Name: genre},
		}
	}

	if tags != nil {
		options := make([]notionapi.Option, len(tags))
		for i, t := range tags {
			options[i] = notionapi.Option{Name: t}
		}
		props["Tags"] = notionapi.MultiSelectProperty{
			MultiSelect: options,
		}
	}

	if status != "" {
		props["Status"] = notionapi.SelectProperty{
			Select: notionapi.Option{Name: status},
		}
	}

	if memo != "" {
		props["Memo"] = notionapi.RichTextProperty{
			RichText: []notionapi.RichText{
				{
					Text: &notionapi.Text{Content: memo},
				},
			},
		}
	}

	if len(props) == 0 {
		return nil
	}

	return &notionapi.PageUpdateRequest{
		Properties: props,
	}
}

// UpdateTechItem updates properties of a tech blog item
func (c *TechClient) UpdateTechItem(ctx context.Context, pageID notionapi.ObjectID, genre string, tags []string, status string, memo string) error {
	req := BuildTechPageUpdateRequest(genre, tags, status, memo)
	if req == nil {
		return nil
	}

	_, err := c.api.Page.Update(ctx, notionapi.PageID(pageID), req)
	if err != nil {
		return fmt.Errorf("TechBlog更新エラー: %w", err)
	}
	return nil
}

// ToggleStatus updates the status (e.g. "未読" <-> "読了")
func (c *TechClient) ToggleStatus(ctx context.Context, pageID notionapi.ObjectID, newStatus string) error {
	return c.UpdateTechItem(ctx, pageID, "", nil, newStatus, "")
}
