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

func TestNormalizeSearchText(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"ルシファー", "ルシファー"},
		{"るしふぁー", "ルシファー"},
		{"マナ", "マナ"},
		{"まな", "マナ"},
		{"NEO", "neo"},
		{"ＮＥＯ", "neo"},
		{"Neo: Reverse", "neo: reverse"},
		{"エクスカリバー", "エクスカリバー"},
		{"えくすかりばー", "エクスカリバー"},
	}

	for _, tt := range tests {
		got := normalizeSearchText(tt.input)
		if got != tt.expected {
			t.Errorf("normalizeSearchText(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestMonstSearch_FilterByName(t *testing.T) {
	model := NewMonstModel(&mockMonstClient{})
	model.loading = false
	model.state = StateBrowse
	model.allMonsters = []domain.Monster{
		{ID: "1", Name: "ルシファー", Attribute: "闇", Priority: "S"},
		{ID: "2", Name: "マナ", Attribute: "火", Priority: "S"},
		{ID: "3", Name: "NEO", Attribute: "光", Priority: "A"},
		{ID: "4", Name: "エクスカリバー", Attribute: "火", Priority: "A"},
	}
	model.applyFilter()

	if len(model.monsters) != 4 {
		t.Fatalf("Expected 4 monsters initially, got %d", len(model.monsters))
	}

	// 1. '/' キーを押して検索モードに移行
	updatedModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	model = updatedModel.(MonstModel)
	if model.state != StateSearch {
		t.Fatalf("Expected state to be StateSearch, got %v", model.state)
	}
	if cmd == nil {
		t.Errorf("Expected blink command when entering search mode")
	}

	// 2. 文字入力: ひらがな「るし」を入力してリアルタイム絞り込みされるか
	for _, r := range "るし" {
		updatedModel, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		model = updatedModel.(MonstModel)
	}
	if len(model.monsters) != 1 || model.monsters[0].Name != "ルシファー" {
		t.Errorf("Expected 1 monster (ルシファー), got %v", model.monsters)
	}

	// 3. Enter キーで StateBrowse に戻り、検索結果が維持されるか
	updatedModel, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updatedModel.(MonstModel)
	if model.state != StateBrowse {
		t.Errorf("Expected state to be StateBrowse after Enter, got %v", model.state)
	}
	if len(model.monsters) != 1 || model.monsters[0].Name != "ルシファー" {
		t.Errorf("Expected search results to be preserved after Enter, got %v", model.monsters)
	}

	// 4. 英字検索: '/' で再度検索モードに入り、"neo" と入力
	model.search.SetValue("") // 一旦クリア
	model.applyFilter()
	updatedModel, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	model = updatedModel.(MonstModel)
	for _, r := range "neo" {
		updatedModel, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		model = updatedModel.(MonstModel)
	}
	if len(model.monsters) != 1 || model.monsters[0].Name != "NEO" {
		t.Errorf("Expected 1 monster (NEO) for 'neo', got %v", model.monsters)
	}
}

func TestMonstSearch_EscClear(t *testing.T) {
	model := NewMonstModel(&mockMonstClient{})
	model.loading = false
	model.state = StateBrowse
	model.allMonsters = []domain.Monster{
		{ID: "1", Name: "ルシファー", Attribute: "闇"},
		{ID: "2", Name: "マナ", Attribute: "火"},
	}
	model.search.SetValue("マナ")
	model.applyFilter()

	if len(model.monsters) != 1 {
		t.Fatalf("Expected 1 monster with filter 'マナ', got %d", len(model.monsters))
	}

	// StateBrowse で Esc を押すと検索クエリがクリアされる
	updatedModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updatedModel.(MonstModel)

	if model.search.Value() != "" {
		t.Errorf("Expected search value to be empty, got %q", model.search.Value())
	}
	if len(model.monsters) != 2 {
		t.Errorf("Expected all 2 monsters after Esc, got %d", len(model.monsters))
	}
}
