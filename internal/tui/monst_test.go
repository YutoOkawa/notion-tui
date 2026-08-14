package tui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jomei/notionapi"
	"github.com/ookawayuuto/notion-tui/internal/domain"
)

type mockMonstClient struct{
	updateMock func(ctx context.Context, monsterID notionapi.ObjectID, accountKey string, wakuwakuIDs []notionapi.ObjectID) error
}

func (m *mockMonstClient) FetchWakuwaku(ctx context.Context) ([]domain.Wakuwaku, map[notionapi.ObjectID]domain.Wakuwaku, error) {
	return nil, nil, nil
}
func (m *mockMonstClient) FetchMonsters(ctx context.Context, attribute string) ([]domain.Monster, error) {
	return nil, nil
}
func (m *mockMonstClient) UpdateMonsterRelations(ctx context.Context, monsterID notionapi.ObjectID, accountKey string, wakuwakuIDs []notionapi.ObjectID) error {
	if m.updateMock != nil {
		return m.updateMock(ctx, monsterID, accountKey, wakuwakuIDs)
	}
	return nil
}

func TestMonstEditWakuwaku(t *testing.T) {
	model := NewMonstModel(&mockMonstClient{})
	model.state = StateEditWakuwaku
	model.loading = false
	
	// モックデータの設定
	id := notionapi.ObjectID("waku-1")
	model.wakuwakuList = []domain.Wakuwaku{{ID: id, Name: "友撃"}}
	model.wakuwakuCursor = 0
	model.selectedWaku = make(map[notionapi.ObjectID]bool)
	model.monsters = []domain.Monster{{ID: "monst-1", Name: "Test Monst"}}
	model.monsterIndex = 0
	model.accounts = []string{"アカウントA"}
	model.accountIndex = 0

	// 1. Enterでチェックがトグルされるか（API保存はされない）
	updatedModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updatedModel.(MonstModel)
	if cmd != nil {
		t.Errorf("Enter key should not trigger save command")
	}
	if !model.selectedWaku[id] {
		t.Errorf("Expected Wakuwaku to be selected")
	}

	// もう一度Enterで解除されるか
	updatedModel, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updatedModel.(MonstModel)
	if model.selectedWaku[id] {
		t.Errorf("Expected Wakuwaku to be deselected")
	}

	// 2. 's' キーで保存処理が発火するか
	updatedModel, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	model = updatedModel.(MonstModel)
	if cmd == nil {
		t.Errorf("'s' key should trigger save command")
	}
}

func TestMonstExcludeMishoji(t *testing.T) {
	var savedIDs []notionapi.ObjectID
	mockClient := &mockMonstClient{
		updateMock: func(ctx context.Context, monsterID notionapi.ObjectID, accountKey string, wakuwakuIDs []notionapi.ObjectID) error {
			savedIDs = wakuwakuIDs
			return nil
		},
	}
	model := NewMonstModel(mockClient)
	model.state = StateEditWakuwaku
	model.loading = false
	
	id1 := notionapi.ObjectID("waku-1")
	idMishoji := notionapi.ObjectID("waku-mishoji")

	model.wakuwakuList = []domain.Wakuwaku{
		{ID: id1, Name: "友撃"},
		{ID: idMishoji, Name: "未所持"},
	}
	model.wakuwakuDict = map[notionapi.ObjectID]domain.Wakuwaku{
		id1: {ID: id1, Name: "友撃"},
		idMishoji: {ID: idMishoji, Name: "未所持"},
	}
	model.monsters = []domain.Monster{{ID: "monst-1", Name: "Test Monst"}}
	model.accounts = []string{"アカウントA"}
	
	model.selectedWaku = map[notionapi.ObjectID]bool{
		id1: true,
		idMishoji: true,
	}

	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if cmd == nil {
		t.Fatal("Expected command")
	}
	msg := cmd() // Executing the command returned
	_ = msg // Should be relationUpdatedMsg
	
	if len(savedIDs) != 1 || savedIDs[0] != id1 {
		t.Errorf("Expected only id1 to be saved, got %v", savedIDs)
	}
}
