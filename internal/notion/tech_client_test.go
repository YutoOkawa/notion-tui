package notion_test

import (
	"testing"

	"github.com/jomei/notionapi"
	"github.com/ookawayuuto/notion-tui/internal/notion"
)

func TestBuildTechPageUpdateRequest(t *testing.T) {
	tests := []struct {
		name         string
		genre        string
		tags         []string
		status       string
		memo         string
		expectNil    bool
		checkGenre   string
		checkTagsLen int
		checkStatus  string
		checkMemo    string
	}{
		{
			name:      "All empty returns nil",
			genre:     "",
			tags:      nil,
			status:    "",
			memo:      "",
			expectNil: true,
		},
		{
			name:        "Status only",
			genre:       "",
			tags:        nil,
			status:      "読了",
			memo:        "",
			expectNil:   false,
			checkStatus: "読了",
		},
		{
			name:         "Genre and Tags",
			genre:        "Backend",
			tags:         []string{"Go", "Docker"},
			status:       "",
			memo:         "",
			expectNil:    false,
			checkGenre:   "Backend",
			checkTagsLen: 2,
		},
		{
			name:         "All fields populated",
			genre:        "AI/ML",
			tags:         []string{"LLM"},
			status:       "読了",
			memo:         "素晴らしい記事だった",
			expectNil:    false,
			checkGenre:   "AI/ML",
			checkTagsLen: 1,
			checkStatus:  "読了",
			checkMemo:    "素晴らしい記事だった",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := notion.BuildTechPageUpdateRequest(tt.genre, tt.tags, tt.status, tt.memo)
			if tt.expectNil {
				if req != nil {
					t.Fatalf("expected nil request, got %v", req)
				}
				return
			}

			if req == nil {
				t.Fatalf("expected non-nil request, got nil")
			}

			if tt.checkGenre != "" {
				genreProp, ok := req.Properties["Genre"].(notionapi.SelectProperty)
				if !ok || genreProp.Select.Name != tt.checkGenre {
					t.Errorf("Genre expected %s, got %v", tt.checkGenre, genreProp)
				}
			}

			if tt.checkTagsLen > 0 {
				tagsProp, ok := req.Properties["Tags"].(notionapi.MultiSelectProperty)
				if !ok || len(tagsProp.MultiSelect) != tt.checkTagsLen {
					t.Errorf("Tags len expected %d, got %v", tt.checkTagsLen, tagsProp)
				}
			}

			if tt.checkStatus != "" {
				statusProp, ok := req.Properties["Status"].(notionapi.SelectProperty)
				if !ok || statusProp.Select.Name != tt.checkStatus {
					t.Errorf("Status expected %s, got %v", tt.checkStatus, statusProp)
				}
			}

			if tt.checkMemo != "" {
				memoProp, ok := req.Properties["Memo"].(notionapi.RichTextProperty)
				if !ok || len(memoProp.RichText) == 0 || memoProp.RichText[0].Text.Content != tt.checkMemo {
					t.Errorf("Memo expected %s, got %v", tt.checkMemo, memoProp)
				}
			}
		})
	}
}
