package notion

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jomei/notionapi"
	"github.com/ookawayuuto/notion-tui/internal/domain"
)

type TaskClient struct {
	api        *notionapi.Client
	tasksDBID  notionapi.DatabaseID
}

func NewTaskClient(token, tasksID string) *TaskClient {
	return &TaskClient{
		api:       notionapi.NewClient(notionapi.Token(token)),
		tasksDBID: notionapi.DatabaseID(tasksID),
	}
}

// FetchPersonalTasks fetches tasks where "タスク種別" is "Personal"
func (c *TaskClient) FetchPersonalTasks(ctx context.Context) ([]domain.TaskItem, error) {
	query := &notionapi.DatabaseQueryRequest{
		Filter: notionapi.PropertyFilter{
			Property: "タスク種別",
			Select: &notionapi.SelectFilterCondition{
				Equals: "Personal",
			},
		},
		Sorts: []notionapi.SortObject{
			{
				Property:  "Status",
				Direction: notionapi.SortOrderASC,
			},
		},
	}
	resp, err := c.api.Database.Query(ctx, c.tasksDBID, query)
	if err != nil {
		return nil, fmt.Errorf("タスク取得エラー: %w", err)
	}

	var items []domain.TaskItem
	for _, page := range resp.Results {
		name := extractTitle(page, "Task name", "名前", "Name")
		status := ""
		if statusProp, ok := page.Properties["Status"].(*notionapi.StatusProperty); ok && statusProp.Status.Name != "" {
			status = statusProp.Status.Name
		}
		
		dueDate := ""
		if dateProp, ok := page.Properties["Due"].(*notionapi.DateProperty); ok && dateProp.Date != nil && dateProp.Date.Start != nil {
			dueDate = time.Time(*dateProp.Date.Start).Format("2006-01-02")
		}

		taskType := ""
		if typeProp, ok := page.Properties["タスク種別"].(*notionapi.SelectProperty); ok {
			taskType = typeProp.Select.Name
		}

		// Done と Archived のタスクはTUIの一覧から除外する
		if status == "Done" || status == "Archived" {
			continue
		}

		items = append(items, domain.TaskItem{
			ID:       page.ID,
			Name:     name,
			Status:   status,
			DueDate:  dueDate,
			TaskType: taskType,
		})
	}
	return items, nil
}

// UpdateTaskStatus updates the "Status" property of a task
func (c *TaskClient) UpdateTaskStatus(ctx context.Context, pageID notionapi.ObjectID, newStatus string) error {
	_, err := c.api.Page.Update(ctx, notionapi.PageID(pageID), &notionapi.PageUpdateRequest{
		Properties: notionapi.Properties{
			"Status": notionapi.StatusProperty{
				Status: notionapi.Option{Name: newStatus},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("ステータス更新エラー: %w", err)
	}
	return nil
}

// UpdateTaskDueDate updates the "Due" property of a task
func (c *TaskClient) UpdateTaskDueDate(ctx context.Context, pageID notionapi.ObjectID, dueDate string) error {
	var dateValue *notionapi.DateObject
	if dueDate != "" {
		parsedDate, err := time.Parse("2006-01-02", dueDate)
		if err != nil {
			return fmt.Errorf("日付のパースエラー(YYYY-MM-DD形式で入力してください): %w", err)
		}
		nd := notionapi.Date(parsedDate)
		dateValue = &notionapi.DateObject{Start: &nd}
	} else {
		// nullify the date
		dateValue = nil
	}

	_, err := c.api.Page.Update(ctx, notionapi.PageID(pageID), &notionapi.PageUpdateRequest{
		Properties: notionapi.Properties{
			"Due": notionapi.DateProperty{
				Date: dateValue,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("期限更新エラー: %w", err)
	}
	return nil
}

// AddPersonalTask adds a new Personal task
func (c *TaskClient) AddPersonalTask(ctx context.Context, name, dueDate, content string) (domain.TaskItem, error) {
	props := notionapi.Properties{
		"Task name": notionapi.TitleProperty{
			Title: []notionapi.RichText{{Text: &notionapi.Text{Content: name}}},
		},
		"タスク種別": notionapi.SelectProperty{
			Select: notionapi.Option{Name: "Personal"},
		},
		"Status": notionapi.StatusProperty{
			Status: notionapi.Option{Name: "Not Started"},
		},
	}

	if dueDate != "" {
		parsedDate, err := time.Parse("2006-01-02", dueDate)
		if err != nil {
			return domain.TaskItem{}, fmt.Errorf("日付のパースエラー: %w", err)
		}
		nd := notionapi.Date(parsedDate)
		props["Due"] = notionapi.DateProperty{Date: &notionapi.DateObject{Start: &nd}}
	}

	var children []notionapi.Block
	if content != "" {
		children = append(children, notionapi.ParagraphBlock{
			BasicBlock: notionapi.BasicBlock{
				Object: "block",
				Type:   notionapi.BlockTypeParagraph,
			},
			Paragraph: notionapi.Paragraph{
				RichText: []notionapi.RichText{
					{
						Text: &notionapi.Text{Content: content},
					},
				},
			},
		})
	}

	res, err := c.api.Page.Create(ctx, &notionapi.PageCreateRequest{
		Parent:     notionapi.Parent{Type: notionapi.ParentTypeDatabaseID, DatabaseID: c.tasksDBID},
		Properties: props,
		Children:   children,
	})
	if err != nil {
		return domain.TaskItem{}, fmt.Errorf("タスク追加エラー: %w", err)
	}

	return domain.TaskItem{
		ID:       res.ID,
		Name:     name,
		Status:   "Not Started",
		DueDate:  dueDate,
		TaskType: "Personal",
	}, nil
}

func (c *TaskClient) FetchTaskBody(ctx context.Context, pageID notionapi.ObjectID) (string, error) {
	resp, err := c.api.Block.GetChildren(ctx, notionapi.BlockID(pageID), nil)
	if err != nil {
		return "", fmt.Errorf("本文取得エラー: %w", err)
	}

	var sb strings.Builder
	for _, block := range resp.Results {
		switch b := block.(type) {
		case *notionapi.ParagraphBlock:
			sb.WriteString(richTextToPlainText(b.Paragraph.RichText) + "\n\n")
		case *notionapi.Heading1Block:
			sb.WriteString("# " + richTextToPlainText(b.Heading1.RichText) + "\n\n")
		case *notionapi.Heading2Block:
			sb.WriteString("## " + richTextToPlainText(b.Heading2.RichText) + "\n\n")
		case *notionapi.Heading3Block:
			sb.WriteString("### " + richTextToPlainText(b.Heading3.RichText) + "\n\n")
		case *notionapi.BulletedListItemBlock:
			sb.WriteString("- " + richTextToPlainText(b.BulletedListItem.RichText) + "\n")
		case *notionapi.QuoteBlock:
			sb.WriteString("> " + richTextToPlainText(b.Quote.RichText) + "\n\n")
		case *notionapi.CodeBlock:
			sb.WriteString("```" + b.Code.Language + "\n" + richTextToPlainText(b.Code.RichText) + "\n```\n\n")
		}
	}

	return strings.TrimSpace(sb.String()), nil
}

func (c *TaskClient) EditTaskBody(ctx context.Context, pageID notionapi.ObjectID, newMarkdown string) error {
	resp, err := c.api.Block.GetChildren(ctx, notionapi.BlockID(pageID), nil)
	if err != nil {
		return fmt.Errorf("既存ブロック取得エラー: %w", err)
	}

	var wg sync.WaitGroup
	for _, block := range resp.Results {
		wg.Add(1)
		go func(bID notionapi.BlockID) {
			defer wg.Done()
			c.api.Block.Delete(ctx, bID)
		}(block.GetID())
	}
	wg.Wait()

	newBlocks := parseMarkdownToBlocks(newMarkdown)
	if len(newBlocks) > 0 {
		req := &notionapi.AppendBlockChildrenRequest{
			Children: newBlocks,
		}
		_, err = c.api.Block.AppendChildren(ctx, notionapi.BlockID(pageID), req)
		if err != nil {
			return fmt.Errorf("本文更新(追加)エラー: %w", err)
		}
	}

	return nil
}
