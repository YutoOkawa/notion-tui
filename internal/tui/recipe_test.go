package tui

import (
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jomei/notionapi"
	"github.com/ookawayuuto/notion-tui/internal/domain"
)

type mockRecipeClient struct {
	recipes []domain.RecipeItem
	updated map[notionapi.ObjectID]domain.RecipeItem
}

func (m *mockRecipeClient) FetchRecipes(ctx context.Context) ([]domain.RecipeItem, error) {
	return m.recipes, nil
}

func (m *mockRecipeClient) UpdateRecipe(ctx context.Context, pageID notionapi.ObjectID, categories []string, genre string, difficulty string, cooked *bool) error {
	for i, r := range m.recipes {
		if r.ID == pageID {
			if categories != nil {
				m.recipes[i].Categories = categories
			}
			if genre != "" {
				m.recipes[i].Genre = genre
			}
			if difficulty != "" {
				m.recipes[i].Difficulty = difficulty
			}
			if cooked != nil {
				m.recipes[i].Cooked = *cooked
			}
			break
		}
	}
	return nil
}

func (m *mockRecipeClient) ToggleCooked(ctx context.Context, pageID notionapi.ObjectID, cooked bool) error {
	return m.UpdateRecipe(ctx, pageID, nil, "", "", &cooked)
}

func TestRecipeModel(t *testing.T) {
	mockData := []domain.RecipeItem{
		{
			ID:         "page-1",
			Title:      "テスト唐揚げ",
			URL:        "https://example.com/karaage",
			Categories: []string{},
			Genre:      "",
			Difficulty: "",
			Cooked:     false,
			CreatedAt:  time.Now(),
		},
		{
			ID:         "page-2",
			Title:      "テストカレー",
			URL:        "https://example.com/curry",
			Categories: []string{"肉", "野菜"},
			Genre:      "洋食",
			Difficulty: "★",
			Cooked:     true,
			CreatedAt:  time.Now(),
		},
	}

	client := &mockRecipeClient{recipes: mockData, updated: make(map[notionapi.ObjectID]domain.RecipeItem)}
	model := NewRecipeModel(client)

	// 1. 初期状態チェック
	if !model.loading {
		t.Error("Expected initial model to be loading")
	}

	// 2. データ読み込みメッセージ処理
	msg := recipesLoadedMsg{recipes: mockData}
	updated, _ := model.Update(msg)
	m := updated.(RecipeModel)

	if m.loading {
		t.Error("Expected model not to be loading after recipesLoadedMsg")
	}

	// デフォルトは TabUnlabeled（未分類タブ）なので、page-1 のみ表示されているはず
	if len(m.recipes) != 1 {
		t.Fatalf("Expected 1 unlabeled recipe, got %d", len(m.recipes))
	}
	if m.recipes[0].ID != "page-1" {
		t.Errorf("Expected recipe ID page-1, got %s", m.recipes[0].ID)
	}

	// 3. Tab キーで「すべて」タブへ切り替え
	tabKey := tea.KeyMsg{Type: tea.KeyTab}
	updated, _ = m.Update(tabKey)
	m = updated.(RecipeModel)

	if m.tab != TabAll {
		t.Fatalf("Expected tab to be TabAll (1), got %d", m.tab)
	}
	if len(m.recipes) != 2 {
		t.Fatalf("Expected 2 recipes in TabAll, got %d", len(m.recipes))
	}

	// 4. 'c' キーで「つくってみた」をトグル
	cKey := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}}
	updated, cmd := m.Update(cKey)
	m = updated.(RecipeModel)
	if cmd == nil {
		t.Error("Expected cmd from 'c' key toggle")
	}
	if !m.recipes[0].Cooked {
		t.Errorf("Expected recipe 0 to be marked as cooked")
	}

	// 5. 'e' キーでラベル編集モードへ遷移
	eKey := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}}
	updated, _ = m.Update(eKey)
	m = updated.(RecipeModel)

	if m.mode != ViewModeEditGenre {
		t.Errorf("Expected mode ViewModeEditGenre, got %d", m.mode)
	}

	// View がクラッシュしないか検証
	viewStr := m.View()
	if viewStr == "" {
		t.Error("View output should not be empty")
	}
}
