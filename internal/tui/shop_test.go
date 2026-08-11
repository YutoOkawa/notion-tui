package tui

import (
	"context"
	"testing"

	"github.com/charmbracelet/bubbletea"
	"github.com/jomei/notionapi"
	"github.com/ookawayuuto/notion-tui/internal/domain"
)

// モッククライアント
type MockClient struct{}

func (m *MockClient) FetchData(ctx context.Context) ([]domain.InventoryItem, []domain.ShoppingItem, error) {
	return []domain.InventoryItem{
		{ID: "1", Name: "Test Item", Stock: 1},
	}, nil, nil
}
func (m *MockClient) AddShoppingItem(ctx context.Context, name string) (domain.ShoppingItem, error) {
	return domain.ShoppingItem{ID: "2", Name: name}, nil
}
func (m *MockClient) CheckShoppingItem(ctx context.Context, pageID notionapi.ObjectID) error {
	return nil
}
func (m *MockClient) UpdateStock(ctx context.Context, pageID notionapi.ObjectID, newStock int) error {
	return nil
}

func TestModelUpdate(t *testing.T) {
	client := &MockClient{}
	model := NewShopModel(client)

	// 初期状態はローディング中
	if !model.loading {
		t.Error("Expected initial model to be loading")
	}

	// dataLoadedMsg のテスト（非同期データフェッチのコールバック）
	msg := dataLoadedMsg{
		items:    []domain.InventoryItem{{ID: "1", Name: "Test Item", Stock: 1}},
		shopping: []domain.ShoppingItem{},
	}
	updatedModel, _ := model.Update(msg)
	newModel := updatedModel.(ShopModel)

	if newModel.loading {
		t.Error("Model should not be loading after dataLoadedMsg")
	}
	if len(newModel.items) != 1 {
		t.Errorf("Expected 1 item, got %d", len(newModel.items))
	}

	// タブキーによるペイン切り替えテスト
	tabMsg := tea.KeyMsg{Type: tea.KeyTab}
	switchedModel, _ := newModel.Update(tabMsg)
	if switchedModel.(ShopModel).activePane != 1 {
		t.Error("Expected active pane to switch to 1 (Right Pane)")
	}
}
