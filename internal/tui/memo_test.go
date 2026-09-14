package tui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jomei/notionapi"
	"github.com/ookawayuuto/notion-tui/internal/domain"
)

type mockMemoClient struct {
	memos           []domain.MemoItem
	updatedID       notionapi.ObjectID
	updatedStatus   string
	updatedCategory string
}

func (m *mockMemoClient) FetchMemos(ctx context.Context) ([]domain.MemoItem, error) {
	return m.memos, nil
}

func (m *mockMemoClient) UpdateMemoStatus(ctx context.Context, pageID notionapi.ObjectID, newStatus string) error {
	m.updatedID = pageID
	m.updatedStatus = newStatus
	return nil
}

func (m *mockMemoClient) UpdateMemoCategory(ctx context.Context, pageID notionapi.ObjectID, newCategory string) error {
	m.updatedID = pageID
	m.updatedCategory = newCategory
	return nil
}

func (m *mockMemoClient) FetchMemoBody(ctx context.Context, pageID notionapi.ObjectID) (string, error) {
	return "Memo Body", nil
}

func (m *mockMemoClient) EditMemoBody(ctx context.Context, pageID notionapi.ObjectID, newMarkdown string) error {
	return nil
}

func TestMemoModel_TabsAndFiltering(t *testing.T) {
	mockClient := &mockMemoClient{
		memos: []domain.MemoItem{
			{ID: "memo-1", Title: "アイデア1", Status: "インボックス", Category: "アイデア"},
			{ID: "memo-2", Title: "完了メモ", Status: "完了", Category: "タスク"},
			{ID: "memo-3", Title: "調査メモ", Status: "整理・検討中", Category: "技術"},
		},
	}

	model := NewMemoModel(mockClient)
	updated, _ := model.Update(memoLoadedMsg{memos: mockClient.memos})
	m := updated.(*MemoModel)

	// 1. 初期状態は Active タブ（"完了" を除く2件が表示されるべき）
	filtered := m.filteredMemos()
	if len(filtered) != 2 {
		t.Fatalf("Activeタブでは完了を除く2件が表示されるべき: got %d", len(filtered))
	}
	if filtered[0].ID != "memo-1" || filtered[1].ID != "memo-3" {
		t.Errorf("ActiveタブのメモIDが一致しません: got %v", filtered)
	}

	// 2. Tab キーで Completed タブへ切り替え（1件）
	tabKey := tea.KeyMsg{Type: tea.KeyTab}
	updated, _ = m.Update(tabKey)
	m = updated.(*MemoModel)

	if m.tabIndex != tabCompleted {
		t.Fatalf("Completedタブになるべき: got %d", m.tabIndex)
	}
	filtered = m.filteredMemos()
	if len(filtered) != 1 || filtered[0].ID != "memo-2" {
		t.Fatalf("Completedタブでは完了メモ1件が表示されるべき: got %v", filtered)
	}

	// 3. Tab キーで All タブへ切り替え（3件）
	updated, _ = m.Update(tabKey)
	m = updated.(*MemoModel)
	if m.tabIndex != tabAll {
		t.Fatalf("Allタブになるべき: got %d", m.tabIndex)
	}
	filtered = m.filteredMemos()
	if len(filtered) != 3 {
		t.Fatalf("Allタブでは全3件が表示されるべき: got %d", len(filtered))
	}

	// 4. 検索フィルタの絞り込み検証
	slashKey := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}
	updated, _ = m.Update(slashKey)
	m = updated.(*MemoModel)
	if m.state != memoStateFilter {
		t.Fatalf("状態が memoStateFilter になるべき: got %v", m.state)
	}

	m.filter.SetValue("技術")
	filtered = m.filteredMemos()
	if len(filtered) != 1 || filtered[0].ID != "memo-3" {
		t.Errorf("フィルター検索結果が一致しません: got %v", filtered)
	}
}

func TestMemoModel_StatusAndCategoryUpdate(t *testing.T) {
	mockClient := &mockMemoClient{
		memos: []domain.MemoItem{
			{ID: "memo-1", Title: "メモ1", Status: "インボックス", Category: "一般"},
		},
	}

	model := NewMemoModel(mockClient)
	updated, _ := model.Update(memoLoadedMsg{memos: mockClient.memos})
	m := updated.(*MemoModel)

	// 's' キーでステータス変更（インボックス -> 整理・検討中）
	sKey := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}}
	updated, cmd := m.Update(sKey)
	m = updated.(*MemoModel)
	if cmd == nil {
		t.Fatal("ステータス変更用の Cmd が発行されるべき")
	}
	msg := cmd()
	if mockClient.updatedStatus != "整理・検討中" {
		t.Errorf("次のステータスは '整理・検討中' であるべき: got %s", mockClient.updatedStatus)
	}

	// statusUpdatedMsg を処理して Model 内のステータスが更新されること
	updated, _ = m.Update(msg)
	m = updated.(*MemoModel)
	if m.memos[0].Status != "整理・検討中" {
		t.Errorf("モデル内のステータスが更新されるべき: got %s", m.memos[0].Status)
	}

	// 'p' キーでカテゴリを "ナレッジベース" へ変更
	pKey := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}}
	updated, cmd = m.Update(pKey)
	m = updated.(*MemoModel)
	if cmd == nil {
		t.Fatal("カテゴリ変更用の Cmd が発行されるべき")
	}
	msg = cmd()
	if mockClient.updatedCategory != "ナレッジベース" {
		t.Errorf("カテゴリが 'ナレッジベース' になるべき: got %s", mockClient.updatedCategory)
	}
}
