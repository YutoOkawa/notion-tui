package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jomei/notionapi"
	"github.com/ookawayuuto/notion-tui/internal/domain"
)

type mockTechClient struct {
	items []domain.TechItem
}

func (m *mockTechClient) FetchTechItems(ctx context.Context) ([]domain.TechItem, error) {
	return m.items, nil
}

func (m *mockTechClient) UpdateTechItem(ctx context.Context, pageID notionapi.ObjectID, genre string, tags []string, status string, memo string) error {
	for i, item := range m.items {
		if item.ID == pageID {
			if genre != "" {
				m.items[i].Genre = genre
			}
			if tags != nil {
				m.items[i].Tags = tags
			}
			if status != "" {
				m.items[i].Status = status
			}
			if memo != "" {
				m.items[i].Memo = memo
			}
			break
		}
	}
	return nil
}

func (m *mockTechClient) ToggleStatus(ctx context.Context, pageID notionapi.ObjectID, newStatus string) error {
	return m.UpdateTechItem(ctx, pageID, "", nil, newStatus, "")
}

func TestTechModel_LifecycleAndTabs(t *testing.T) {
	mockData := []domain.TechItem{
		{
			ID:        "page-1",
			Title:     "未分類記事",
			URL:       "https://example.com/unlabeled",
			Status:    "未読",
			Genre:     "",
			Tags:      nil,
			CreatedAt: time.Now(),
		},
		{
			ID:        "page-2",
			Title:     "読了記事",
			URL:       "https://example.com/read",
			Status:    "読了",
			Genre:     "Backend",
			Tags:      []string{"Go"},
			CreatedAt: time.Now(),
		},
	}

	client := &mockTechClient{items: mockData}
	model := NewTechModel(client)

	// 1. Initial State
	if !model.loading {
		t.Error("Expected initial model to be loading")
	}

	// 2. Data Loaded
	msg := techItemsLoadedMsg{items: mockData}
	updated, _ := model.Update(msg)
	m := updated.(TechModel)

	if m.loading {
		t.Error("Expected model not to be loading after items loaded")
	}

	// Default tab: TechTabUnlabeled -> only page-1
	if len(m.items) != 1 {
		t.Fatalf("Expected 1 item in unlabeled tab, got %d", len(m.items))
	}
	if m.items[0].ID != "page-1" {
		t.Errorf("Expected page-1, got %s", m.items[0].ID)
	}

	// 3. Switch to TechTabAll (Tab key)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(TechModel)
	if m.tab != TechTabAll {
		t.Errorf("Expected TechTabAll, got %d", m.tab)
	}
	if len(m.items) != 2 {
		t.Fatalf("Expected 2 items in all tab, got %d", len(m.items))
	}

	// 4. Switch to TechTabUnread
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(TechModel)
	if m.tab != TechTabUnread {
		t.Errorf("Expected TechTabUnread, got %d", m.tab)
	}
	if len(m.items) != 1 || m.items[0].ID != "page-1" {
		t.Errorf("Expected page-1 in unread tab, got %v", m.items)
	}

	// 5. Fast Status Toggle with 's' key on page-1
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	m = updated.(TechModel)
	if cmd == nil {
		t.Error("Expected command to be returned for toggle status")
	}
	// In-memory update immediately flips to "読了"
	if m.allTechItems[0].Status != "読了" {
		t.Errorf("Expected status to be updated to 読了, got %s", m.allTechItems[0].Status)
	}

	// 6. View smoke test
	viewOutput := m.View()
	if !strings.Contains(viewOutput, "TechBlog Stack") {
		t.Errorf("Expected view to contain TechBlog Stack header")
	}
}
