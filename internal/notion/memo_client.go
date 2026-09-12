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

type MemoClient struct {
	api  *notionapi.Client
	dbID notionapi.DatabaseID
}

func NewMemoClient(token, dbID string) *MemoClient {
	return &MemoClient{
		api:  notionapi.NewClient(notionapi.Token(token)),
		dbID: notionapi.DatabaseID(dbID),
	}
}

func (c *MemoClient) FetchMemos(ctx context.Context) ([]domain.MemoItem, error) {
	query := &notionapi.DatabaseQueryRequest{
		Sorts: []notionapi.SortObject{
			{
				Timestamp: notionapi.TimestampLastEdited,
				Direction: notionapi.SortOrderDESC,
			},
		},
		PageSize: 50,
	}

	resp, err := c.api.Database.Query(ctx, c.dbID, query)
	if err != nil {
		return nil, fmt.Errorf("メモ取得エラー: %w", err)
	}

	var items []domain.MemoItem
	for _, page := range resp.Results {
		title := "No Title"
		if prop, ok := page.Properties["名前"].(*notionapi.TitleProperty); ok && len(prop.Title) > 0 {
			title = prop.Title[0].PlainText
		} else if prop, ok := page.Properties["Name"].(*notionapi.TitleProperty); ok && len(prop.Title) > 0 {
			title = prop.Title[0].PlainText
		}

		category := "-"
		if prop, ok := page.Properties["分類カテゴリ"].(*notionapi.SelectProperty); ok && prop.Select.Name != "" {
			category = prop.Select.Name
		}

		status := "-"
		if prop, ok := page.Properties["ステータス"].(*notionapi.SelectProperty); ok && prop.Select.Name != "" {
			status = prop.Select.Name
		} else if prop, ok := page.Properties["ステータス 1"].(*notionapi.StatusProperty); ok && prop.Status.Name != "" {
			status = prop.Status.Name
		}

		items = append(items, domain.MemoItem{
			ID:        page.ID,
			Title:     title,
			Category:  category,
			Status:    status,
			UpdatedAt: time.Time(page.LastEditedTime),
		})
	}
	return items, nil
}
func (c *MemoClient) AddMemo(ctx context.Context, title string, body string) error {
	blocks := parseMarkdownToBlocks(body)

	req := &notionapi.PageCreateRequest{
		Parent: notionapi.Parent{Type: notionapi.ParentTypeDatabaseID, DatabaseID: c.dbID},
		Properties: notionapi.Properties{
			"名前": notionapi.TitleProperty{Title: []notionapi.RichText{{Text: &notionapi.Text{Content: title}}}},
		},
		Children: blocks,
	}

	_, err := c.api.Page.Create(ctx, req)
	if err != nil {
		// Fallback to "Name" if "名前" does not exist
		req.Properties = notionapi.Properties{
			"Name": notionapi.TitleProperty{Title: []notionapi.RichText{{Text: &notionapi.Text{Content: title}}}},
		}
		_, err = c.api.Page.Create(ctx, req)
	}

	if err != nil {
		return fmt.Errorf("メモ追加エラー: %w", err)
	}
	return nil
}

func (c *MemoClient) UpdateMemoStatus(ctx context.Context, pageID notionapi.ObjectID, newStatus string) error {
	req := &notionapi.PageUpdateRequest{
		Properties: notionapi.Properties{
			"ステータス": notionapi.SelectProperty{Select: notionapi.Option{Name: newStatus}},
		},
	}
	_, err := c.api.Page.Update(ctx, notionapi.PageID(pageID), req)
	if err != nil {
		return fmt.Errorf("ステータス更新エラー: %w", err)
	}
	return nil
}

func (c *MemoClient) UpdateMemoCategory(ctx context.Context, pageID notionapi.ObjectID, newCategory string) error {
	req := &notionapi.PageUpdateRequest{
		Properties: notionapi.Properties{
			"分類カテゴリ": notionapi.SelectProperty{Select: notionapi.Option{Name: newCategory}},
		},
	}
	_, err := c.api.Page.Update(ctx, notionapi.PageID(pageID), req)
	if err != nil {
		return fmt.Errorf("カテゴリ更新エラー: %w", err)
	}
	return nil
}

func (c *MemoClient) FetchMemoBody(ctx context.Context, pageID notionapi.ObjectID) (string, error) {
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

func richTextToPlainText(rt []notionapi.RichText) string {
	var sb strings.Builder
	for _, r := range rt {
		sb.WriteString(r.PlainText)
	}
	return sb.String()
}

func (c *MemoClient) EditMemoBody(ctx context.Context, pageID notionapi.ObjectID, newMarkdown string) error {
	// Fetch existing blocks
	resp, err := c.api.Block.GetChildren(ctx, notionapi.BlockID(pageID), nil)
	if err != nil {
		return fmt.Errorf("既存ブロック取得エラー: %w", err)
	}
	
	// Delete existing blocks concurrently to reduce time lag
	var wg sync.WaitGroup
	for _, block := range resp.Results {
		wg.Add(1)
		go func(bID notionapi.BlockID) {
			defer wg.Done()
			c.api.Block.Delete(ctx, bID)
		}(block.GetID())
	}
	wg.Wait()
	
	// Append new blocks
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

func parseMarkdownToBlocks(text string) []notionapi.Block {
	if text == "" {
		return nil
	}
	var blocks []notionapi.Block
	lines := strings.Split(text, "\n")
	
	var inCodeBlock bool
	var codeContent []string
	var codeLang string
	var currentParagraph []string
	
	flushParagraph := func() {
		if len(currentParagraph) > 0 {
			p := strings.Join(currentParagraph, "\n")
			blocks = append(blocks, createParagraphBlock(p))
			currentParagraph = nil
		}
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		
		if inCodeBlock {
			if strings.HasPrefix(trimmed, "```") {
				blocks = append(blocks, createCodeBlock(strings.Join(codeContent, "\n"), codeLang))
				inCodeBlock = false
				codeContent = nil
			} else {
				codeContent = append(codeContent, line)
			}
			continue
		}
		
		if strings.HasPrefix(trimmed, "```") {
			flushParagraph()
			inCodeBlock = true
			codeLang = strings.TrimPrefix(trimmed, "```")
			if codeLang == "" {
				codeLang = "plain text"
			}
			continue
		}
		
		if strings.HasPrefix(trimmed, "# ") {
			flushParagraph()
			blocks = append(blocks, createHeadingBlock(1, strings.TrimPrefix(trimmed, "# ")))
			continue
		}
		if strings.HasPrefix(trimmed, "## ") {
			flushParagraph()
			blocks = append(blocks, createHeadingBlock(2, strings.TrimPrefix(trimmed, "## ")))
			continue
		}
		if strings.HasPrefix(trimmed, "### ") {
			flushParagraph()
			blocks = append(blocks, createHeadingBlock(3, strings.TrimPrefix(trimmed, "### ")))
			continue
		}
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			flushParagraph()
			txt := strings.TrimPrefix(trimmed, "- ")
			txt = strings.TrimPrefix(txt, "* ")
			blocks = append(blocks, createListBlock(txt))
			continue
		}
		if strings.HasPrefix(trimmed, "> ") {
			flushParagraph()
			blocks = append(blocks, createQuoteBlock(strings.TrimPrefix(trimmed, "> ")))
			continue
		}
		
		if trimmed == "" {
			flushParagraph()
			continue
		}
		
		currentParagraph = append(currentParagraph, line)
	}
	
	flushParagraph()
	
	if inCodeBlock {
		blocks = append(blocks, createCodeBlock(strings.Join(codeContent, "\n"), codeLang))
	}
	
	return blocks
}

func createParagraphBlock(text string) notionapi.Block {
	return notionapi.ParagraphBlock{
		BasicBlock: notionapi.BasicBlock{Object: "block", Type: notionapi.BlockTypeParagraph},
		Paragraph: notionapi.Paragraph{RichText: createRichText(text)},
	}
}

func createHeadingBlock(level int, text string) notionapi.Block {
	rt := createRichText(text)
	switch level {
	case 1:
		return notionapi.Heading1Block{BasicBlock: notionapi.BasicBlock{Object: "block", Type: notionapi.BlockTypeHeading1}, Heading1: notionapi.Heading{RichText: rt}}
	case 2:
		return notionapi.Heading2Block{BasicBlock: notionapi.BasicBlock{Object: "block", Type: notionapi.BlockTypeHeading2}, Heading2: notionapi.Heading{RichText: rt}}
	default:
		return notionapi.Heading3Block{BasicBlock: notionapi.BasicBlock{Object: "block", Type: notionapi.BlockTypeHeading3}, Heading3: notionapi.Heading{RichText: rt}}
	}
}

func createListBlock(text string) notionapi.Block {
	return notionapi.BulletedListItemBlock{
		BasicBlock: notionapi.BasicBlock{Object: "block", Type: notionapi.BlockTypeBulletedListItem},
		BulletedListItem: notionapi.ListItem{RichText: createRichText(text)},
	}
}

func createQuoteBlock(text string) notionapi.Block {
	return notionapi.QuoteBlock{
		BasicBlock: notionapi.BasicBlock{Object: "block", Type: notionapi.BlockTypeQuote},
		Quote: notionapi.Quote{RichText: createRichText(text)},
	}
}

func createCodeBlock(text, lang string) notionapi.Block {
	if lang == "" {
		lang = "plain text"
	}
	return notionapi.CodeBlock{
		BasicBlock: notionapi.BasicBlock{Object: "block", Type: notionapi.BlockTypeCode},
		Code: notionapi.Code{RichText: createRichText(text), Language: lang},
	}
}

func createRichText(text string) []notionapi.RichText {
	if text == "" {
		return []notionapi.RichText{{Type: "text", Text: &notionapi.Text{Content: ""}}}
	}
	var richTexts []notionapi.RichText
	runes := []rune(text)
	for len(runes) > 0 {
		chunkSize := 2000
		if len(runes) < chunkSize {
			chunkSize = len(runes)
		}
		chunk := string(runes[:chunkSize])
		richTexts = append(richTexts, notionapi.RichText{
			Type: "text",
			Text: &notionapi.Text{Content: chunk},
		})
		runes = runes[chunkSize:]
	}
	return richTexts
}
