package notion

import (
	"context"
	"fmt"

	"github.com/jomei/notionapi"
	"github.com/ookawayuuto/notion-tui/internal/domain"
)

type ReadClient struct {
	api       *notionapi.Client
	tasksDB   notionapi.DatabaseID
	projectID notionapi.ObjectID
}

func NewReadClient(token, tasksDB, projectID string) *ReadClient {
	return &ReadClient{
		api:       notionapi.NewClient(notionapi.Token(token)),
		tasksDB:   notionapi.DatabaseID(tasksDB),
		projectID: notionapi.ObjectID(projectID),
	}
}

func (c *ReadClient) FetchBooks(ctx context.Context) ([]domain.Book, error) {
	query := &notionapi.DatabaseQueryRequest{
		Filter: notionapi.PropertyFilter{
			Property: "Project",
			Relation: &notionapi.RelationFilterCondition{
				Contains: c.projectID.String(),
			},
		},
		PageSize: 100,
	}

	var books []domain.Book
	hasMore := true
	cursor := notionapi.Cursor("")

	for hasMore {
		if cursor != "" {
			query.StartCursor = cursor
		} else {
			query.StartCursor = ""
		}

		res, err := c.api.Database.Query(ctx, c.tasksDB, query)
		if err != nil {
			return nil, fmt.Errorf("読書タスク取得エラー: %w", err)
		}

		for _, page := range res.Results {
			title := extractTitle(page, "Task name")
			status := "Not started"
			if prop, ok := page.Properties["Status"].(*notionapi.StatusProperty); ok && prop.Status.Name != "" {
				status = prop.Status.Name
			}

			var readPages, totalPages int
			if prop, ok := page.Properties["読んだページ数"].(*notionapi.NumberProperty); ok {
				readPages = int(prop.Number)
			}
			if prop, ok := page.Properties["総ページ数"].(*notionapi.NumberProperty); ok {
				totalPages = int(prop.Number)
			}

			books = append(books, domain.Book{
				ID:         notionapi.ObjectID(page.ID),
				Title:      title,
				Status:     status,
				ReadPages:  readPages,
				TotalPages: totalPages,
			})
		}

		hasMore = res.HasMore
		if hasMore {
			cursor = res.NextCursor
		}
	}

	return books, nil
}

func (c *ReadClient) UpdateBookProgress(ctx context.Context, id notionapi.ObjectID, readPages int, newStatus string) error {
	props := notionapi.Properties{
		"読んだページ数": notionapi.NumberProperty{Number: float64(readPages)},
	}
	if newStatus != "" {
		props["Status"] = notionapi.StatusProperty{
			Status: notionapi.Option{Name: newStatus},
		}
	}

	_, err := c.api.Page.Update(ctx, notionapi.PageID(id), &notionapi.PageUpdateRequest{
		Properties: props,
	})
	if err != nil {
		return fmt.Errorf("進捗更新エラー: %w", err)
	}
	return nil
}

func (c *ReadClient) CreateBook(ctx context.Context, title string, totalPages int) error {
	props := notionapi.Properties{
		"Task name": notionapi.TitleProperty{
			Title: []notionapi.RichText{
				{Text: &notionapi.Text{Content: title}},
			},
		},
		"Status": notionapi.StatusProperty{
			Status: notionapi.Option{Name: "Not Started"},
		},
		"総ページ数": notionapi.NumberProperty{
			Number: float64(totalPages),
		},
		"読んだページ数": notionapi.NumberProperty{
			Number: 0,
		},
		"Project": notionapi.RelationProperty{
			Relation: []notionapi.Relation{
				{ID: notionapi.PageID(c.projectID.String())},
			},
		},
		"タスク種別": notionapi.SelectProperty{
			Select: notionapi.Option{Name: "Study"},
		},
	}

	_, err := c.api.Page.Create(ctx, &notionapi.PageCreateRequest{
		Parent: notionapi.Parent{
			DatabaseID: c.tasksDB,
			Type:       notionapi.ParentTypeDatabaseID,
		},
		Properties: props,
	})
	if err != nil {
		return fmt.Errorf("読書タスク作成エラー: %w", err)
	}
	return nil
}
