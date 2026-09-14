package tui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jomei/notionapi"
	"github.com/ookawayuuto/notion-tui/internal/domain"
)

type mockReadClient struct {
	books            []domain.Book
	updatedID        notionapi.ObjectID
	updatedReadPages int
	updatedStatus    string
	createdTitle     string
	createdPages     int
}

func (m *mockReadClient) FetchBooks(ctx context.Context) ([]domain.Book, error) {
	return m.books, nil
}

func (m *mockReadClient) UpdateBookProgress(ctx context.Context, id notionapi.ObjectID, readPages int, newStatus string) error {
	m.updatedID = id
	m.updatedReadPages = readPages
	m.updatedStatus = newStatus
	return nil
}

func (m *mockReadClient) CreateBook(ctx context.Context, title string, totalPages int) error {
	m.createdTitle = title
	m.createdPages = totalPages
	return nil
}

func TestReadModel_LoadAndFilter(t *testing.T) {
	mockClient := &mockReadClient{
		books: []domain.Book{
			{ID: "book-1", Title: "Go言語入門", Status: "In Progress", ReadPages: 50, TotalPages: 200},
			{ID: "book-2", Title: "リーダブルコード", Status: "Done", ReadPages: 250, TotalPages: 250},
			{ID: "book-3", Title: "デザインパターン", Status: "Not Started", ReadPages: 0, TotalPages: 300},
		},
	}

	model := NewReadModel(mockClient)
	if !model.loading {
		t.Error("初期状態は loading であるべき")
	}

	// データロード
	msg := booksLoadedMsg{books: mockClient.books}
	updated, _ := model.Update(msg)
	m := updated.(ReadModel)

	if m.loading {
		t.Error("データ読み込み後は loading が解除されるべき")
	}

	// 初期表示は "In Progress" が優先選択される仕様
	if len(m.books) != 1 || m.books[0].ID != "book-1" {
		t.Fatalf("初回は In Progress の書籍（1件）が表示されるべき: got %v", m.books)
	}

	// '/' キーで次のステータス（Done）へフィルタ切り替え
	slashKey := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}
	updated, _ = m.Update(slashKey)
	m = updated.(ReadModel)

	if len(m.books) != 1 || m.books[0].ID != "book-2" {
		t.Errorf("フィルター後は Done の書籍（1件）が表示されるべき: got %v", m.books)
	}
}

func TestReadModel_PageIncrementAndStatusAutoUpdate(t *testing.T) {
	mockClient := &mockReadClient{
		books: []domain.Book{
			{ID: "book-1", Title: "テスト駆動開発", Status: "In Progress", ReadPages: 99, TotalPages: 100},
		},
	}

	model := NewReadModel(mockClient)
	updated, _ := model.Update(booksLoadedMsg{books: mockClient.books})
	m := updated.(ReadModel)

	// 'l' キーでページインクリメント (99 -> 100)
	lKey := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}}
	updated, _ = m.Update(lKey)
	m = updated.(ReadModel)

	if m.books[0].ReadPages != 100 {
		t.Errorf("読了ページ数が100になるべき: got %d", m.books[0].ReadPages)
	}

	// 総ページ数（100）を超えないことの検証
	updated, _ = m.Update(lKey)
	m = updated.(ReadModel)
	if m.books[0].ReadPages != 100 {
		t.Errorf("総ページ数を超えて増加しないべき: got %d", m.books[0].ReadPages)
	}

	// 's' キーで保存時に、100/100 ページのため自動で Status が "Done" に更新されることの検証
	sKey := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}}
	updated, cmd := m.Update(sKey)
	if cmd == nil {
		t.Fatal("保存用の tea.Cmd が発行されるべき")
	}
	cmd() // コマンドを実行してモックに記録させる

	if mockClient.updatedReadPages != 100 {
		t.Errorf("保存されたページ数が一致しません: got %d, want 100", mockClient.updatedReadPages)
	}
	if mockClient.updatedStatus != "Done" {
		t.Errorf("全ページ読了時は自動的に 'Done' になるべき: got '%s'", mockClient.updatedStatus)
	}
}

func TestReadModel_CreateBook(t *testing.T) {
	mockClient := &mockReadClient{}
	model := NewReadModel(mockClient)
	updated, _ := model.Update(booksLoadedMsg{books: []domain.Book{}})
	m := updated.(ReadModel)

	// 'a' キーでタイトル入力モードへ
	aKey := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	updated, _ = m.Update(aKey)
	m = updated.(ReadModel)

	if m.mode != ModeInputTitle {
		t.Fatalf("モードが ModeInputTitle になるべき: got %v", m.mode)
	}

	// タイトル入力 -> Enter でページ数入力モードへ
	m.textInput.SetValue("リファクタリング第2版")
	enterKey := tea.KeyMsg{Type: tea.KeyEnter}
	updated, _ = m.Update(enterKey)
	m = updated.(ReadModel)

	if m.mode != ModeInputPages {
		t.Fatalf("モードが ModeInputPages になるべき: got %v", m.mode)
	}

	// ページ数入力 -> Enter で作成 Cmd 発行
	m.textInput.SetValue("450")
	updated, cmd := m.Update(enterKey)
	m = updated.(ReadModel)

	if m.mode != ModeList {
		t.Errorf("作成後は ModeList に戻るべき: got %v", m.mode)
	}
	if cmd == nil {
		t.Fatal("書籍作成用の tea.Cmd が発行されるべき")
	}
	cmd() // 実行

	if mockClient.createdTitle != "リファクタリング第2版" {
		t.Errorf("作成タイトルが一致しません: got %s", mockClient.createdTitle)
	}
	if mockClient.createdPages != 450 {
		t.Errorf("作成ページ数が一致しません: got %d, want 450", mockClient.createdPages)
	}
}
