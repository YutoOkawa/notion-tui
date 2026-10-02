package notion

import (
	"testing"
	"time"

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

func TestExtractExpirationDate(t *testing.T) {
	sampleTime := notionapi.Date(time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC))

	tests := []struct {
		name     string
		page     notionapi.Page
		key      string
		expected string
	}{
		{
			name: "valid date property",
			page: notionapi.Page{
				Properties: notionapi.Properties{
					"賞味期限": &notionapi.DateProperty{
						Date: &notionapi.DateObject{
							Start: &sampleTime,
						},
					},
				},
			},
			key:      "賞味期限",
			expected: "2026-09-30",
		},
		{
			name: "nil date object",
			page: notionapi.Page{
				Properties: notionapi.Properties{
					"賞味期限": &notionapi.DateProperty{
						Date: nil,
					},
				},
			},
			key:      "賞味期限",
			expected: "",
		},
		{
			name: "nil start date",
			page: notionapi.Page{
				Properties: notionapi.Properties{
					"賞味期限": &notionapi.DateProperty{
						Date: &notionapi.DateObject{
							Start: nil,
						},
					},
				},
			},
			key:      "賞味期限",
			expected: "",
		},
		{
			name: "missing property key",
			page: notionapi.Page{
				Properties: notionapi.Properties{},
			},
			key:      "賞味期限",
			expected: "",
		},
		{
			name: "different property type",
			page: notionapi.Page{
				Properties: notionapi.Properties{
					"賞味期限": &notionapi.NumberProperty{Number: 123},
				},
			},
			key:      "賞味期限",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractExpirationDate(tt.page, tt.key)
			if got != tt.expected {
				t.Errorf("extractExpirationDate() = %v, want %v", got, tt.expected)
			}
			gotDate := extractDateProperty(tt.page, tt.key)
			if gotDate != tt.expected {
				t.Errorf("extractDateProperty() = %v, want %v", gotDate, tt.expected)
			}
		})
	}
}

