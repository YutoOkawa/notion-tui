package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jomei/notionapi"
	"github.com/ookawayuuto/notion-tui/internal/domain"
)

type MorningClientInterface interface {
	FetchMorningReports(ctx context.Context) ([]domain.MorningReport, error)
	FetchReportBody(ctx context.Context, pageID notionapi.ObjectID) (string, error)
	UpdateReportStatus(ctx context.Context, pageID notionapi.ObjectID, newStatus string) error
}

type MorningState int

const (
	morningStateList MorningState = iota
	morningStateFilter
	morningStateDetail
)

type MorningModel struct {
	client MorningClientInterface

	allReports []domain.MorningReport
	reports    []domain.MorningReport

	cursor     int
	listOffset int

	// タグフィルタ（動的抽出）
	tags     []string
	tagIndex int

	state     MorningState
	filter    textinput.Model
	viewport  viewport.Model
	ready     bool
	loading   bool
	loadMsg   string
	err       error

	width  int
	height int

	// キャッシュ (PageID -> Markdown Content)
	contentCache map[notionapi.ObjectID]string
}

func NewMorningModel(client MorningClientInterface) *MorningModel {
	ti := textinput.New()
	ti.Placeholder = "キーワード(タイトル/サマリ/タグ)で絞り込み..."
	ti.CharLimit = 50
	ti.Width = 40

	vp := viewport.New(80, 20)

	return &MorningModel{
		client:       client,
		state:        morningStateList,
		filter:       ti,
		viewport:     vp,
		loading:      true,
		loadMsg:      "朝のキャッチアップレポートを取得中...",
		tags:         []string{"全て"},
		contentCache: make(map[notionapi.ObjectID]string),
	}
}

type morningReportsLoadedMsg struct {
	reports []domain.MorningReport
}

type morningBodyFetchedMsg struct {
	pageID  notionapi.ObjectID
	content string
}

type morningStatusUpdatedMsg struct {
	id     notionapi.ObjectID
	status string
}

type morningErrMsg struct{ err error }

func (m *MorningModel) Init() tea.Cmd {
	return tea.Batch(m.fetchReportsCmd(), textinput.Blink)
}

func (m *MorningModel) fetchReportsCmd() tea.Cmd {
	return func() tea.Msg {
		reports, err := m.client.FetchMorningReports(context.Background())
		if err != nil {
			return morningErrMsg{err}
		}
		return morningReportsLoadedMsg{reports: reports}
	}
}

func (m *MorningModel) fetchBodyCmd(pageID notionapi.ObjectID) tea.Cmd {
	return func() tea.Msg {
		content, err := m.client.FetchReportBody(context.Background(), pageID)
		if err != nil {
			return morningErrMsg{err}
		}
		return morningBodyFetchedMsg{pageID: pageID, content: content}
	}
}

func (m *MorningModel) updateStatusCmd(pageID notionapi.ObjectID, newStatus string) tea.Cmd {
	return func() tea.Msg {
		err := m.client.UpdateReportStatus(context.Background(), pageID, newStatus)
		if err != nil {
			return morningErrMsg{err}
		}
		return morningStatusUpdatedMsg{id: pageID, status: newStatus}
	}
}

func (m *MorningModel) extractTags() {
	tagMap := make(map[string]bool)
	m.tags = []string{"全て"}
	for _, r := range m.allReports {
		for _, tag := range r.Tags {
			if tag != "" && !tagMap[tag] {
				tagMap[tag] = true
				m.tags = append(m.tags, tag)
			}
		}
	}
}

func (m *MorningModel) filteredReports() []domain.MorningReport {
	query := strings.ToLower(strings.TrimSpace(m.filter.Value()))
	selectedTag := ""
	if m.tagIndex > 0 && m.tagIndex < len(m.tags) {
		selectedTag = m.tags[m.tagIndex]
	}

	var res []domain.MorningReport
	for _, r := range m.allReports {
		// タグ絞り込み
		if selectedTag != "" {
			hasTag := false
			for _, t := range r.Tags {
				if t == selectedTag {
					hasTag = true
					break
				}
			}
			if !hasTag {
				continue
			}
		}

		// テキスト検索
		if query != "" {
			matched := strings.Contains(strings.ToLower(r.Title), query) ||
				strings.Contains(strings.ToLower(r.Summary), query) ||
				strings.Contains(strings.ToLower(r.Date), query)
			if !matched {
				for _, t := range r.Tags {
					if strings.Contains(strings.ToLower(t), query) {
						matched = true
						break
					}
				}
			}
			if !matched {
				continue
			}
		}

		res = append(res, r)
	}
	return res
}

func (m *MorningModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		headerHeight := 4
		footerHeight := 3
		vpHeight := msg.Height - headerHeight - footerHeight
		if vpHeight < 5 {
			vpHeight = 5
		}
		m.viewport.Width = msg.Width - 4
		m.viewport.Height = vpHeight
		m.ready = true
		return m, nil

	case morningReportsLoadedMsg:
		m.allReports = msg.reports
		m.extractTags()
		m.reports = m.filteredReports()
		m.loading = false
		m.cursor = 0
		m.listOffset = 0
		return m, nil

	case morningBodyFetchedMsg:
		m.contentCache[msg.pageID] = msg.content
		m.loading = false
		m.state = morningStateDetail

		// 詳細ビューポートにコンテンツを設定
		selected := m.reports[m.cursor]
		styledContent := m.formatReportForViewport(selected, msg.content)
		m.viewport.SetContent(styledContent)
		m.viewport.GotoTop()
		return m, nil

	case morningStatusUpdatedMsg:
		m.loading = false
		for i, r := range m.allReports {
			if r.ID == msg.id {
				m.allReports[i].Status = msg.status
				break
			}
		}
		m.reports = m.filteredReports()
		return m, nil

	case morningErrMsg:
		m.err = msg.err
		m.loading = false
		return m, nil

	case tea.KeyMsg:
		if m.loading {
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			return m, nil
		}

		switch m.state {
		case morningStateList:
			switch msg.String() {
			case "ctrl+c", "q":
				return m, tea.Quit
			case "up", "k":
				if m.cursor > 0 {
					m.cursor--
					if m.cursor < m.listOffset {
						m.listOffset--
					}
				}
			case "down", "j":
				if m.cursor < len(m.reports)-1 {
					m.cursor++
					maxDisplay := m.maxListItems()
					if m.cursor >= m.listOffset+maxDisplay {
						m.listOffset++
					}
				}
			case "tab", "right", "l":
				if len(m.tags) > 0 {
					m.tagIndex = (m.tagIndex + 1) % len(m.tags)
					m.reports = m.filteredReports()
					m.cursor = 0
					m.listOffset = 0
				}
			case "shift+tab", "left", "h":
				if len(m.tags) > 0 {
					m.tagIndex = (m.tagIndex - 1 + len(m.tags)) % len(m.tags)
					m.reports = m.filteredReports()
					m.cursor = 0
					m.listOffset = 0
				}
			case "/":
				m.state = morningStateFilter
				m.filter.Focus()
				return m, textinput.Blink
			case "s":
				if len(m.reports) > 0 {
					r := m.reports[m.cursor]
					nextStatus := "完了"
					if r.Status == "完了" {
						nextStatus = "未着手"
					}
					m.loading = true
					m.loadMsg = fmt.Sprintf("ステータスを「%s」に更新中...", nextStatus)
					return m, m.updateStatusCmd(r.ID, nextStatus)
				}
			case "enter":
				if len(m.reports) > 0 {
					selected := m.reports[m.cursor]
					// キャッシュ済みか確認
					if content, ok := m.contentCache[selected.ID]; ok && content != "" {
						m.state = morningStateDetail
						styledContent := m.formatReportForViewport(selected, content)
						m.viewport.SetContent(styledContent)
						m.viewport.GotoTop()
						return m, nil
					}
					m.loading = true
					m.loadMsg = fmt.Sprintf("「%s」の本文を取得中...", selected.Title)
					return m, m.fetchBodyCmd(selected.ID)
				}
			}

		case morningStateFilter:
			switch msg.Type {
			case tea.KeyEsc, tea.KeyEnter:
				m.state = morningStateList
				m.filter.Blur()
				m.reports = m.filteredReports()
				m.cursor = 0
				m.listOffset = 0
			default:
				m.filter, cmd = m.filter.Update(msg)
				m.reports = m.filteredReports()
				m.cursor = 0
				m.listOffset = 0
				return m, cmd
			}

		case morningStateDetail:
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc", "q", "backspace":
				m.state = morningStateList
				return m, nil
			case "s":
				if len(m.reports) > 0 {
					r := m.reports[m.cursor]
					nextStatus := "完了"
					if r.Status == "完了" {
						nextStatus = "未着手"
					}
					m.loading = true
					m.loadMsg = fmt.Sprintf("ステータスを「%s」に更新中...", nextStatus)
					return m, m.updateStatusCmd(r.ID, nextStatus)
				}
			default:
				m.viewport, cmd = m.viewport.Update(msg)
				cmds = append(cmds, cmd)
				return m, tea.Batch(cmds...)
			}
		}
	}

	return m, nil
}

func (m *MorningModel) maxListItems() int {
	if m.height > 20 {
		return m.height - 15
	}
	return 8
}

func (m *MorningModel) formatReportForViewport(report domain.MorningReport, markdown string) string {
	var sb strings.Builder

	borderStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	headerTitleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).MarginBottom(1)
	metaStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	summaryBoxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("63")).
		Padding(0, 1).
		MarginBottom(1)

	// タイトルとメタデータ
	sb.WriteString(headerTitleStyle.Render(report.Title) + "\n")
	sb.WriteString(metaStyle.Render(fmt.Sprintf("📅 日付: %s  │  🏷️ タグ: %s  │  📌 ステータス: %s",
		report.Date, strings.Join(report.Tags, ", "), report.Status)) + "\n")

	if report.Summary != "" {
		summaryText := lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Render("💡 サマリ: " + report.Summary)
		sb.WriteString(summaryBoxStyle.Render(summaryText) + "\n")
	}

	sb.WriteString(borderStyle.Render(strings.Repeat("─", 60)) + "\n\n")

	// 本文の整形（見出しや箇条書きのスタイリング）
	lines := strings.Split(markdown, "\n")
	h1Style := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	h2Style := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("43"))
	h3Style := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	bulletStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("220"))

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			sb.WriteString(h1Style.Render(trimmed) + "\n")
		} else if strings.HasPrefix(trimmed, "## ") {
			sb.WriteString(h2Style.Render(trimmed) + "\n")
		} else if strings.HasPrefix(trimmed, "### ") {
			sb.WriteString(h3Style.Render(trimmed) + "\n")
		} else if strings.HasPrefix(trimmed, "- ") {
			sb.WriteString(bulletStyle.Render("• ") + strings.TrimPrefix(trimmed, "- ") + "\n")
		} else if strings.HasPrefix(trimmed, "• ") {
			sb.WriteString(bulletStyle.Render("• ") + strings.TrimPrefix(trimmed, "• ") + "\n")
		} else {
			sb.WriteString(line + "\n")
		}
	}

	return sb.String()
}

func (m *MorningModel) View() string {
	if m.err != nil {
		return fmt.Sprintf("\n  ❌ エラー: %v\n\n  (q で終了)\n", m.err)
	}
	if m.loading {
		return fmt.Sprintf("\n  🔄 %s\n", m.loadMsg)
	}

	switch m.state {
	case morningStateDetail:
		return m.viewDetail()
	default:
		return m.viewList()
	}
}

func (m *MorningModel) viewList() string {
	var sb strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	sb.WriteString(titleStyle.Render("☀️ Morning Catchup (朝のキャッチアップ一覧)") + "\n\n")

	// タグバー
	var tagStrs []string
	activeTagStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Underline(true)
	inactiveTagStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	for i, t := range m.tags {
		if i == m.tagIndex {
			tagStrs = append(tagStrs, activeTagStyle.Render("["+t+"]"))
		} else {
			tagStrs = append(tagStrs, inactiveTagStyle.Render(t))
		}
	}
	sb.WriteString(strings.Join(tagStrs, "  ") + "\n\n")

	// 検索バー
	if m.state == morningStateFilter || m.filter.Value() != "" {
		sb.WriteString(m.filter.View() + "\n\n")
	}

	if len(m.reports) == 0 {
		sb.WriteString("  該当するレポートがありません。\n\n")
	} else {
		maxItems := m.maxListItems()
		displayCount := maxItems
		if len(m.reports) < displayCount {
			displayCount = len(m.reports)
		}

		selectedStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
		dateStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
		tagBadgeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("39"))

		for i := 0; i < displayCount; i++ {
			idx := m.listOffset + i
			if idx >= len(m.reports) {
				break
			}
			report := m.reports[idx]

			cursor := "  "
			if idx == m.cursor {
				cursor = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Render("> ")
			}

			// ステータス表示
			statusBadge := "[未読]"
			statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
			if report.Status == "完了" {
				statusBadge = "[確認済]"
				statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
			} else if report.Status == "進行中" {
				statusBadge = "[読込中]"
				statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
			}
			statusStr := statusStyle.Render(statusBadge)

			dateStr := dateStyle.Render(report.Date)
			if report.Date == "" {
				dateStr = dateStyle.Render(report.CreatedAt.Local().Format("2006-01-02"))
			}

			tagStr := ""
			if len(report.Tags) > 0 {
				tagStr = tagBadgeStyle.Render(fmt.Sprintf("[%s]", strings.Join(report.Tags, ", ")))
			}

			titleStr := report.Title
			if idx == m.cursor {
				titleStr = selectedStyle.Render(titleStr)
			}

			line := fmt.Sprintf("%s%s %s %s  %s", cursor, statusStr, dateStr, titleStr, tagStr)
			sb.WriteString(line + "\n")
		}
	}

	// 選択中アイテムのプレビュー枠（サマリ）
	if len(m.reports) > 0 && m.cursor < len(m.reports) {
		selected := m.reports[m.cursor]
		sb.WriteString("\n")
		previewBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("63")).
			Padding(0, 1).
			Width(75)

		previewContent := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("43")).Render("💡 本日の一言サマリ:") + "\n"
		if selected.Summary != "" {
			previewContent += selected.Summary
		} else {
			previewContent += lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("(サマリなし)")
		}
		sb.WriteString(previewBox.Render(previewContent) + "\n")
	}

	sb.WriteString("\n")
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	if m.state == morningStateFilter {
		sb.WriteString(helpStyle.Render("文字を入力して絞り込み | Enter / Esc: 完了"))
	} else {
		sb.WriteString(helpStyle.Render("↑/↓: 選択 │ Enter: 詳細閲覧 │ Tab/←/→: タグ切替 │ s: 確認済切替 │ /: 検索 │ q: 終了"))
	}

	return sb.String()
}

func (m *MorningModel) viewDetail() string {
	var sb strings.Builder

	selected := m.reports[m.cursor]

	// ヘッダーバー
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	statusStr := "[未読]"
	if selected.Status == "完了" {
		statusStr = "[確認済]"
	}
	sb.WriteString(fmt.Sprintf("%s  %s (進捗: %3.f%%)\n",
		headerStyle.Render("📖 "+selected.Title),
		statusStr,
		m.viewport.ScrollPercent()*100))

	// ビューポート
	sb.WriteString(m.viewport.View() + "\n")

	// フッター
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	sb.WriteString(helpStyle.Render("j/k/↑/↓: スクロール │ d/u: 半画面 │ g/G: 先頭/末尾 │ s: 確認済切替 │ Esc / q: 一覧へ戻る"))

	return sb.String()
}
