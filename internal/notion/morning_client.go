package notion

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jomei/notionapi"
	"github.com/ookawayuuto/notion-tui/internal/domain"
)

type MorningClient struct {
	api  *notionapi.Client
	dbID notionapi.DatabaseID
}

func NewMorningClient(token, dbID string) *MorningClient {
	return &MorningClient{
		api:  notionapi.NewClient(notionapi.Token(token)),
		dbID: notionapi.DatabaseID(dbID),
	}
}

// FetchMorningReports は朝のキャッチアップレポート一覧を取得します（日付降順）。
func (c *MorningClient) FetchMorningReports(ctx context.Context) ([]domain.MorningReport, error) {
	query := &notionapi.DatabaseQueryRequest{
		Sorts: []notionapi.SortObject{
			{
				Property:  "日付",
				Direction: notionapi.SortOrderDESC,
			},
			{
				Timestamp: notionapi.TimestampCreated,
				Direction: notionapi.SortOrderDESC,
			},
		},
		PageSize: 100,
	}

	resp, err := c.api.Database.Query(ctx, c.dbID, query)
	if err != nil {
		// 日付プロパティでのソートが失敗した場合、作成日時ソートで再試行
		fallbackQuery := &notionapi.DatabaseQueryRequest{
			Sorts: []notionapi.SortObject{
				{
					Timestamp: notionapi.TimestampCreated,
					Direction: notionapi.SortOrderDESC,
				},
			},
			PageSize: 100,
		}
		resp, err = c.api.Database.Query(ctx, c.dbID, fallbackQuery)
		if err != nil {
			return nil, fmt.Errorf("朝キャッチアップDB取得エラー: %w", err)
		}
	}

	var items []domain.MorningReport
	for _, page := range resp.Results {
		title := extractTitle(page, "名前", "Name", "Title", "タイトル")

		dateStr := ""
		if prop, ok := page.Properties["日付"].(*notionapi.DateProperty); ok && prop.Date != nil && prop.Date.Start != nil {
			dateStr = time.Time(*prop.Date.Start).Format("2006-01-02")
		}

		summary := ""
		if prop, ok := page.Properties["一言サマリ"].(*notionapi.RichTextProperty); ok && len(prop.RichText) > 0 {
			summary = richTextToPlainText(prop.RichText)
		} else if prop, ok := page.Properties["サマリ"].(*notionapi.RichTextProperty); ok && len(prop.RichText) > 0 {
			summary = richTextToPlainText(prop.RichText)
		}

		var tags []string
		if prop, ok := page.Properties["マルチセレクト"].(*notionapi.MultiSelectProperty); ok {
			for _, opt := range prop.MultiSelect {
				tags = append(tags, opt.Name)
			}
		} else if prop, ok := page.Properties["タグ"].(*notionapi.MultiSelectProperty); ok {
			for _, opt := range prop.MultiSelect {
				tags = append(tags, opt.Name)
			}
		}

		status := "未着手"
		if prop, ok := page.Properties["ステータス"].(*notionapi.StatusProperty); ok && prop.Status.Name != "" {
			status = prop.Status.Name
		} else if prop, ok := page.Properties["ステータス"].(*notionapi.SelectProperty); ok && prop.Select.Name != "" {
			status = prop.Select.Name
		}

		items = append(items, domain.MorningReport{
			ID:        page.ID,
			Title:     title,
			Date:      dateStr,
			Summary:   summary,
			Tags:      tags,
			Status:    status,
			URL:       page.URL,
			CreatedAt: time.Time(page.CreatedTime),
			UpdatedAt: time.Time(page.LastEditedTime),
		})
	}

	return items, nil
}

// FetchReportBody はレポートのページ本文を取得し、Markdown文字列に変換して返します。
func (c *MorningClient) FetchReportBody(ctx context.Context, pageID notionapi.ObjectID) (string, error) {
	markdown, err := c.fetchBlocksRecursively(ctx, notionapi.BlockID(pageID), 0)
	if err != nil {
		return "", fmt.Errorf("レポート本文取得エラー: %w", err)
	}
	return strings.TrimSpace(markdown), nil
}

// UpdateReportStatus はレポートのステータス（未着手、完了等）を更新します。
func (c *MorningClient) UpdateReportStatus(ctx context.Context, pageID notionapi.ObjectID, newStatus string) error {
	// StatusPropertyまたはSelectPropertyを試す
	req := &notionapi.PageUpdateRequest{
		Properties: notionapi.Properties{
			"ステータス": notionapi.StatusProperty{Status: notionapi.Option{Name: newStatus}},
		},
	}
	_, err := c.api.Page.Update(ctx, notionapi.PageID(pageID), req)
	if err != nil {
		// SelectPropertyとして再試行
		req2 := &notionapi.PageUpdateRequest{
			Properties: notionapi.Properties{
				"ステータス": notionapi.SelectProperty{Select: notionapi.Option{Name: newStatus}},
			},
		}
		_, err2 := c.api.Page.Update(ctx, notionapi.PageID(pageID), req2)
		if err2 != nil {
			return fmt.Errorf("ステータス更新エラー: %w", err)
		}
	}
	return nil
}

func (c *MorningClient) fetchBlocksRecursively(ctx context.Context, blockID notionapi.BlockID, depth int) (string, error) {
	if depth > 4 {
		return "", nil // 深すぎる再帰を防止
	}

	var allBlocks []notionapi.Block
	var cursor notionapi.Cursor

	for {
		var pagination *notionapi.Pagination
		if cursor != "" {
			pagination = &notionapi.Pagination{StartCursor: cursor, PageSize: 100}
		}
		resp, err := c.api.Block.GetChildren(ctx, blockID, pagination)
		if err != nil {
			return "", err
		}
		allBlocks = append(allBlocks, resp.Results...)
		if !resp.HasMore || resp.NextCursor == "" {
			break
		}
		cursor = notionapi.Cursor(resp.NextCursor)
	}

	var sb strings.Builder
	indent := strings.Repeat("  ", depth)

	for _, block := range allBlocks {
		hasChildren := false
		currID := block.GetID()

		switch b := block.(type) {
		case *notionapi.ParagraphBlock:
			hasChildren = b.HasChildren
			text := richTextToMarkdown(b.Paragraph.RichText)
			if text != "" {
				sb.WriteString(indent + text + "\n\n")
			}
		case *notionapi.Heading1Block:
			hasChildren = b.HasChildren
			sb.WriteString("\n" + indent + "# " + richTextToMarkdown(b.Heading1.RichText) + "\n\n")
		case *notionapi.Heading2Block:
			hasChildren = b.HasChildren
			sb.WriteString("\n" + indent + "## " + richTextToMarkdown(b.Heading2.RichText) + "\n\n")
		case *notionapi.Heading3Block:
			hasChildren = b.HasChildren
			sb.WriteString("\n" + indent + "### " + richTextToMarkdown(b.Heading3.RichText) + "\n\n")
		case *notionapi.BulletedListItemBlock:
			hasChildren = b.HasChildren
			sb.WriteString(indent + "- " + richTextToMarkdown(b.BulletedListItem.RichText) + "\n")
		case *notionapi.NumberedListItemBlock:
			hasChildren = b.HasChildren
			sb.WriteString(indent + "1. " + richTextToMarkdown(b.NumberedListItem.RichText) + "\n")
		case *notionapi.QuoteBlock:
			hasChildren = b.HasChildren
			sb.WriteString(indent + "> " + richTextToMarkdown(b.Quote.RichText) + "\n\n")
		case *notionapi.CodeBlock:
			hasChildren = b.HasChildren
			sb.WriteString(indent + "```" + b.Code.Language + "\n" + richTextToPlainText(b.Code.RichText) + "\n" + indent + "```\n\n")
		case *notionapi.DividerBlock:
			sb.WriteString(indent + "---\n\n")
		case *notionapi.CalloutBlock:
			hasChildren = b.HasChildren
			icon := "💡"
			if b.Callout.Icon != nil && b.Callout.Icon.Emoji != nil {
				icon = string(*b.Callout.Icon.Emoji)
			}
			sb.WriteString(indent + "> " + icon + " " + richTextToMarkdown(b.Callout.RichText) + "\n\n")
		case *notionapi.ToDoBlock:
			hasChildren = b.HasChildren
			check := "[ ]"
			if b.ToDo.Checked {
				check = "[x]"
			}
			sb.WriteString(indent + check + " " + richTextToMarkdown(b.ToDo.RichText) + "\n")
		}

		if hasChildren {
			childrenMD, err := c.fetchBlocksRecursively(ctx, currID, depth+1)
			if err == nil && childrenMD != "" {
				sb.WriteString(childrenMD)
			}
		}
	}

	return sb.String(), nil
}

func richTextToMarkdown(rts []notionapi.RichText) string {
	var sb strings.Builder
	for _, rt := range rts {
		text := rt.PlainText
		if text == "" {
			continue
		}
		if rt.Annotations != nil {
			if rt.Annotations.Code {
				text = "`" + text + "`"
			}
			if rt.Annotations.Bold {
				text = "**" + text + "**"
			}
			if rt.Annotations.Italic {
				text = "*" + text + "*"
			}
			if rt.Annotations.Strikethrough {
				text = "~~" + text + "~~"
			}
		}
		sb.WriteString(text)
	}
	return sb.String()
}
