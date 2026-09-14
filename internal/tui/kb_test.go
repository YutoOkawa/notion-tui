package tui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

type mockKBClient struct {
	categories  []string
	createdItem struct {
		title    string
		category string
	}
}

func (m *mockKBClient) FetchCategories(ctx context.Context) ([]string, error) {
	return m.categories, nil
}

func (m *mockKBClient) AddDraftItem(ctx context.Context, title string, category string) (string, error) {
	m.createdItem.title = title
	m.createdItem.category = category
	return "https://notion.so/test-page-url", nil
}

func TestKBModel_StateTransitions(t *testing.T) {
	mockClient := &mockKBClient{
		categories: []string{"技術", "アイデア"},
	}

	model := NewKBModel(mockClient, mockClient.categories)
	if model.state != kbStateInput {
		t.Fatalf("初期状態は kbStateInput であるべき: got %v", model.state)
	}

	// 1. 空入力での Enter は状態遷移しない
	model.textInput.SetValue("")
	enterKey := tea.KeyMsg{Type: tea.KeyEnter}
	updated, cmd := model.Update(enterKey)
	m := updated.(*KBModel)
	if m.state != kbStateInput || cmd != nil {
		t.Error("空入力時は状態遷移せず Cmd も発行されないべき")
	}

	// 2. 有効なタイトル入力での Enter -> カテゴリ選択へ遷移
	m.textInput.SetValue("AIエージェントの動向調査")
	updated, _ = m.Update(enterKey)
	m = updated.(*KBModel)

	if m.state != kbStateCategory {
		t.Fatalf("カテゴリ選択モード (kbStateCategory) に遷移するべき: got %v", m.state)
	}
	if m.title != "AIエージェントの動向調査" {
		t.Errorf("入力タイトルが保持されていません: got %s", m.title)
	}

	// 3. カテゴリ選択状態で Enter -> 作成中 (kbStateCreating) へ遷移し Cmd 発行
	updated, cmd = m.Update(enterKey)
	m = updated.(*KBModel)

	if m.state != kbStateCreating {
		t.Fatalf("作成中モード (kbStateCreating) に遷移するべき: got %v", m.state)
	}
	if cmd == nil {
		t.Fatal("下書き作成用の Cmd が発行されるべき")
	}

	// コマンドを実行して結果メッセージを検証
	msg := cmd()
	cMsg, ok := msg.(createMsg)
	if !ok {
		t.Fatalf("成功時は createMsg が返るべき: got %T", msg)
	}
	if cMsg.url != "https://notion.so/test-page-url" {
		t.Errorf("作成された URL が一致しません: got %s", cMsg.url)
	}

	// 4. createMsg を受信して完了状態へ遷移
	updated, quitCmd := m.Update(cMsg)
	m = updated.(*KBModel)

	if m.state != kbStateDone {
		t.Errorf("完了モード (kbStateDone) になるべき: got %v", m.state)
	}
	if quitCmd == nil {
		t.Error("完了時は tea.Quit が返るべき")
	}
}

func TestKBModel_DirectCreateWithoutCategories(t *testing.T) {
	mockClient := &mockKBClient{}
	// カテゴリが空の場合
	model := NewKBModel(mockClient, nil)

	model.textInput.SetValue("直接作成テーマ")
	enterKey := tea.KeyMsg{Type: tea.KeyEnter}
	updated, cmd := model.Update(enterKey)
	m := updated.(*KBModel)

	// カテゴリ選択をスキップして直接作成モードへ
	if m.state != kbStateCreating {
		t.Fatalf("カテゴリがない場合は直接 kbStateCreating へ遷移するべき: got %v", m.state)
	}
	if cmd == nil {
		t.Fatal("作成用の Cmd が発行されるべき")
	}
}
