package tui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jomei/notionapi"
	"github.com/ookawayuuto/notion-tui/internal/domain"
)

type mockMorningClient struct {
	reports []domain.MorningReport
	body    string
}

func (m *mockMorningClient) FetchMorningReports(ctx context.Context) ([]domain.MorningReport, error) {
	return m.reports, nil
}

func (m *mockMorningClient) FetchReportBody(ctx context.Context, pageID notionapi.ObjectID) (string, error) {
	return m.body, nil
}

func (m *mockMorningClient) UpdateReportStatus(ctx context.Context, pageID notionapi.ObjectID, newStatus string) error {
	for i, r := range m.reports {
		if r.ID == pageID {
			m.reports[i].Status = newStatus
		}
	}
	return nil
}

func TestMorningModel_LoadAndFilter(t *testing.T) {
	mockClient := &mockMorningClient{
		reports: []domain.MorningReport{
			{
				ID:      "page-1",
				Title:   "2026-09-14 朝のキャッチアップ",
				Date:    "2026-09-14",
				Summary: "OpenAIとGoogleの最新動向",
				Tags:    []string{"AI Agent", "Google AI"},
				Status:  "未着手",
			},
			{
				ID:      "page-2",
				Title:   "2026-09-13 朝のキャッチアップ",
				Date:    "2026-09-13",
				Summary: "セキュリティインシデントまとめ",
				Tags:    []string{"セキュリティ"},
				Status:  "完了",
			},
		},
		body: "# レポート本文\n- 詳細1\n- 詳細2",
	}

	model := NewMorningModel(mockClient)
	if !model.loading {
		t.Error("初期状態はローディング中であるべき")
	}

	// データロード
	msg := morningReportsLoadedMsg{reports: mockClient.reports}
	updated, _ := model.Update(msg)
	m := updated.(*MorningModel)

	if m.loading {
		t.Error("データ読み込み後はローディングが解除されるべき")
	}
	if len(m.reports) != 2 {
		t.Fatalf("レポート件数が一致しません: got %d, want 2", len(m.reports))
	}
	// タグが「全て」「AI Agent」「Google AI」「セキュリティ」の4つ抽出されるべき
	if len(m.tags) != 4 {
		t.Errorf("タグ抽出数が一致しません: got %d, want 4", len(m.tags))
	}

	// タブキーによるタグ絞り込み
	tabMsg := tea.KeyMsg{Type: tea.KeyTab}
	updated, _ = m.Update(tabMsg)
	m = updated.(*MorningModel)
	if m.tagIndex != 1 {
		t.Errorf("タグインデックスが1になるべき: got %d", m.tagIndex)
	}
	// タグ "AI Agent" のレポートのみ（1件）
	if len(m.reports) != 1 || m.reports[0].ID != "page-1" {
		t.Errorf("タグ絞り込み結果が不正です: got %v", m.reports)
	}

	// Enterキーで詳細モードへ遷移
	enterMsg := tea.KeyMsg{Type: tea.KeyEnter}
	updated, cmd := m.Update(enterMsg)
	m = updated.(*MorningModel)
	if cmd == nil {
		t.Error("本文取得用のCmdが返されるべき")
	}

	// 本文取得完了メッセージ
	bodyMsg := morningBodyFetchedMsg{pageID: "page-1", content: mockClient.body}
	updated, _ = m.Update(bodyMsg)
	m = updated.(*MorningModel)
	if m.state != morningStateDetail {
		t.Errorf("詳細表示モード(morningStateDetail)になるべき: got %v", m.state)
	}

	// Escキーで一覧に戻る
	escMsg := tea.KeyMsg{Type: tea.KeyEsc}
	updated, _ = m.Update(escMsg)
	m = updated.(*MorningModel)
	if m.state != morningStateList {
		t.Errorf("一覧表示モード(morningStateList)に戻るべき: got %v", m.state)
	}
}

func TestMorningModel_StatusToggle(t *testing.T) {
	mockClient := &mockMorningClient{
		reports: []domain.MorningReport{
			{
				ID:     "page-1",
				Title:  "2026-09-14 レポート",
				Status: "未着手",
			},
		},
	}

	model := NewMorningModel(mockClient)
	updated, _ := model.Update(morningReportsLoadedMsg{reports: mockClient.reports})
	m := updated.(*MorningModel)

	// 's' キー押下でステータス更新Cmdが発行される
	sKeyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}}
	updated, cmd := m.Update(sKeyMsg)
	m = updated.(*MorningModel)
	if cmd == nil {
		t.Fatal("ステータス更新用のCmdが返されるべき")
	}

	// 更新完了
	statusMsg := morningStatusUpdatedMsg{id: "page-1", status: "完了"}
	updated, _ = m.Update(statusMsg)
	m = updated.(*MorningModel)
	if m.allReports[0].Status != "完了" {
		t.Errorf("ステータスが完了になるべき: got %s", m.allReports[0].Status)
	}
}
