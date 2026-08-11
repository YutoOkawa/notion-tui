package notion

import (
	"testing"
	"github.com/jomei/notionapi"
)

func TestExtractTitle(t *testing.T) {
	page := notionapi.Page{
		Properties: notionapi.Properties{
			"名前": &notionapi.TitleProperty{
				Title: []notionapi.RichText{
					{PlainText: "Test Title"},
				},
			},
		},
	}
	
	title := extractTitle(page, "名前", "Name")
	if title != "Test Title" {
		t.Errorf("Expected 'Test Title', got '%s'", title)
	}

	// 該当キーがない場合のフォールバックテスト
	pageNoTitle := notionapi.Page{Properties: notionapi.Properties{}}
	titleEmpty := extractTitle(pageNoTitle, "名前")
	if titleEmpty != "No Title" {
		t.Errorf("Expected 'No Title', got '%s'", titleEmpty)
	}
}
