package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jomei/notionapi"
	"github.com/ookawayuuto/notion-tui/internal/domain"
)

type MemoClientInterface interface {
	FetchMemos(ctx context.Context) ([]domain.MemoItem, error)
	UpdateMemoStatus(ctx context.Context, pageID notionapi.ObjectID, newStatus string) error
	UpdateMemoCategory(ctx context.Context, pageID notionapi.ObjectID, newCategory string) error
	FetchMemoBody(ctx context.Context, pageID notionapi.ObjectID) (string, error)
	EditMemoBody(ctx context.Context, pageID notionapi.ObjectID, newMarkdown string) error
}

type MemoState int

const (
	memoStateBrowse MemoState = iota
	memoStateFilter
)

const (
	tabActive = iota
	tabCompleted
	tabAll
)

type MemoModel struct {
	client  MemoClientInterface
	memos   []domain.MemoItem
	cursor  int
	tabIndex int
	state   MemoState
	filter  textinput.Model
	loading bool
	err     error
}

func NewMemoModel(client MemoClientInterface) *MemoModel {
	ti := textinput.New()
	ti.Placeholder = "キーワード(タイトル/カテゴリ)で絞り込み..."
	ti.CharLimit = 50
	ti.Width = 40

	return &MemoModel{
		client:  client,
		state:   memoStateBrowse,
		filter:  ti,
		loading: true,
	}
}

type memoLoadedMsg struct {
	memos []domain.MemoItem
}
type memoErrMsg struct{ err error }
type memoActionDoneMsg struct{}

func (m *MemoModel) Init() tea.Cmd {
	return tea.Batch(m.fetchMemosCmd(), textinput.Blink)
}

func (m *MemoModel) fetchMemosCmd() tea.Cmd {
	return func() tea.Msg {
		memos, err := m.client.FetchMemos(context.Background())
		if err != nil {
			return memoErrMsg{err}
		}
		return memoLoadedMsg{memos: memos}
	}
}

func getNextMemoStatus(current string) string {
	switch current {
	case "インボックス":
		return "整理・検討中"
	case "整理・検討中":
		return "完了"
	case "完了":
		return "インボックス"
	default:
		return "インボックス"
	}
}

type statusUpdatedMsg struct {
	id     notionapi.ObjectID
	status string
}

func (m *MemoModel) updateStatusCmd(id notionapi.ObjectID, status string) tea.Cmd {
	return func() tea.Msg {
		if err := m.client.UpdateMemoStatus(context.Background(), id, status); err != nil {
			return memoErrMsg{err}
		}
		return statusUpdatedMsg{id: id, status: status}
	}
}

type categoryUpdatedMsg struct {
	id       notionapi.ObjectID
	category string
}

func (m *MemoModel) updateCategoryCmd(id notionapi.ObjectID, category string) tea.Cmd {
	return func() tea.Msg {
		if err := m.client.UpdateMemoCategory(context.Background(), id, category); err != nil {
			return memoErrMsg{err}
		}
		return categoryUpdatedMsg{id: id, category: category}
	}
}

type bodyFetchedMsg struct {
	pageID  notionapi.ObjectID
	content string
}

func (m *MemoModel) fetchBodyCmd(pageID notionapi.ObjectID) tea.Cmd {
	return func() tea.Msg {
		content, err := m.client.FetchMemoBody(context.Background(), pageID)
		if err != nil {
			return memoErrMsg{err}
		}
		return bodyFetchedMsg{pageID: pageID, content: content}
	}
}

type updateBodyMsg struct {
	pageID  notionapi.ObjectID
	content string
}

type bodyUpdatedMsg struct {
	pageID notionapi.ObjectID
}

func (m *MemoModel) updateBodyCmd(pageID notionapi.ObjectID, content string) tea.Cmd {
	return func() tea.Msg {
		err := m.client.EditMemoBody(context.Background(), pageID, content)
		if err != nil {
			return memoErrMsg{err}
		}
		return bodyUpdatedMsg{pageID: pageID}
	}
}

func (m *MemoModel) openEditorCmd(pageID notionapi.ObjectID, content string) tea.Cmd {
	tmpFile, err := os.CreateTemp("", "ntui-edit-*.md")
	if err != nil {
		return func() tea.Msg { return memoErrMsg{err} }
	}

	tmpFile.WriteString(content)
	tmpFile.Close()

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vim"
	}

	c := exec.Command(editor, tmpFile.Name())
	return tea.ExecProcess(c, func(err error) tea.Msg {
		if err != nil {
			os.Remove(tmpFile.Name())
			return memoErrMsg{err}
		}

		newContentBytes, err := os.ReadFile(tmpFile.Name())
		os.Remove(tmpFile.Name())

		if err != nil {
			return memoErrMsg{err}
		}

		newContent := strings.TrimSpace(string(newContentBytes))
		if newContent == strings.TrimSpace(content) {
			return nil // No change, do nothing
		}

		return updateBodyMsg{pageID: pageID, content: newContent}
	})
}

func (m *MemoModel) filteredMemos() []domain.MemoItem {
	query := strings.ToLower(strings.TrimSpace(m.filter.Value()))
	
	var res []domain.MemoItem
	for _, memo := range m.memos {
		// Filter by Tab
		if m.tabIndex == tabActive && memo.Status == "完了" {
			continue
		}
		if m.tabIndex == tabCompleted && memo.Status != "完了" {
			continue
		}

		// Filter by Search Query
		if query != "" {
			if !strings.Contains(strings.ToLower(memo.Title), query) && !strings.Contains(strings.ToLower(memo.Category), query) {
				continue
			}
		}
		res = append(res, memo)
	}
	return res
}

func (m *MemoModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case memoLoadedMsg:
		m.memos = msg.memos
		m.loading = false
		m.cursor = 0
		return m, nil
	case statusUpdatedMsg:
		for i, memo := range m.memos {
			if memo.ID == msg.id {
				m.memos[i].Status = msg.status
				break
			}
		}
		m.loading = false
		return m, nil
	case categoryUpdatedMsg:
		for i, memo := range m.memos {
			if memo.ID == msg.id {
				m.memos[i].Category = msg.category
				break
			}
		}
		m.loading = false
		return m, nil
	case bodyUpdatedMsg:
		// Could update LastEditedTime if desired, but we just dismiss loading
		m.loading = false
		return m, nil
	case bodyFetchedMsg:
		// Not displaying loading anymore since editor is opening
		m.loading = false
		return m, m.openEditorCmd(msg.pageID, msg.content)
	case updateBodyMsg:
		m.loading = true
		return m, m.updateBodyCmd(msg.pageID, msg.content)
	case memoErrMsg:
		m.err = msg.err
		return m, nil
	case tea.KeyMsg:
		if m.state == memoStateBrowse {
			switch msg.String() {
			case "ctrl+c", "q":
				return m, tea.Quit
			case "up", "k":
				if m.cursor > 0 {
					m.cursor--
				}
			case "down", "j":
				filtered := m.filteredMemos()
				if m.cursor < len(filtered)-1 {
					m.cursor++
				}
			case "tab", "right", "l":
				m.tabIndex = (m.tabIndex + 1) % 3
				m.cursor = 0
			case "shift+tab", "left", "h":
				m.tabIndex = (m.tabIndex - 1 + 3) % 3
				m.cursor = 0
			case "s":
				filtered := m.filteredMemos()
				if len(filtered) > 0 {
					memo := filtered[m.cursor]
					nextStatus := getNextMemoStatus(memo.Status)
					m.loading = true
					return m, m.updateStatusCmd(memo.ID, nextStatus)
				}
			case "p":
				filtered := m.filteredMemos()
				if len(filtered) > 0 {
					memo := filtered[m.cursor]
					m.loading = true
					return m, m.updateCategoryCmd(memo.ID, "ナレッジベース")
				}
			case "enter":
				filtered := m.filteredMemos()
				if len(filtered) > 0 {
					memo := filtered[m.cursor]
					m.loading = true
					return m, m.fetchBodyCmd(memo.ID)
				}
			case "/":
				m.state = memoStateFilter
				m.filter.Focus()
				return m, textinput.Blink
			}
		} else if m.state == memoStateFilter {
			switch msg.Type {
			case tea.KeyEsc, tea.KeyEnter:
				m.state = memoStateBrowse
				m.filter.Blur()
				// フィルタ変更時はカーソルリセット
				m.cursor = 0
			default:
				m.filter, cmd = m.filter.Update(msg)
				m.cursor = 0 
				return m, cmd
			}
		}
	}
	return m, nil
}

func (m *MemoModel) View() string {
	if m.err != nil {
		return fmt.Sprintf("エラー: %v\n\n(q: 終了)", m.err)
	}
	if m.loading {
		return "Now Loading... (メモを取得中)\n"
	}

	var sb strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	sb.WriteString(titleStyle.Render("=== アイデア & 思考ログ ===") + "\n\n")

	tabs := []string{"未処理", "完了", "すべて"}
	var tabStrs []string
	activeTabStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Underline(true)
	inactiveTabStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	for i, t := range tabs {
		if i == m.tabIndex {
			tabStrs = append(tabStrs, activeTabStyle.Render(t))
		} else {
			tabStrs = append(tabStrs, inactiveTabStyle.Render(t))
		}
	}
	sb.WriteString(strings.Join(tabStrs, " │ ") + "\n\n")

	if m.state == memoStateFilter || m.filter.Value() != "" {
		sb.WriteString(m.filter.View() + "\n\n")
	}

	filtered := m.filteredMemos()

	if len(filtered) == 0 {
		sb.WriteString("表示できるメモがありません。\n")
	}

	catStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	dateStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

	displayCount := 20
	if len(filtered) < displayCount {
		displayCount = len(filtered)
	}

	startIdx := 0
	if m.cursor >= displayCount {
		startIdx = m.cursor - displayCount + 1
	}

	for i := 0; i < displayCount; i++ {
		idx := startIdx + i
		if idx >= len(filtered) {
			break
		}
		memo := filtered[idx]

		cursor := "  "
		if idx == m.cursor {
			cursor = "> "
		}

		catFormatted := ""
		if memo.Category != "-" && memo.Category != "" {
			catFormatted = catStyle.Render(fmt.Sprintf("[%s] ", memo.Category))
		}

		statusStr := ""
		if memo.Status != "-" && memo.Status != "" {
			statusStr = statusStyle.Render(fmt.Sprintf("(%s)", memo.Status))
		}

		dateStr := dateStyle.Render(memo.UpdatedAt.Local().Format("01/02 15:04"))

		nameStr := memo.Title
		if idx == m.cursor {
			nameStr = lipgloss.NewStyle().Bold(true).Render(nameStr)
			cursor = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Render("> ")
		}

		line := fmt.Sprintf("%s %s%s %s  %s", cursor, catFormatted, nameStr, statusStr, dateStr)
		sb.WriteString(line + "\n")
	}

	sb.WriteString("\n")
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	if m.state == memoStateBrowse {
		sb.WriteString(helpStyle.Render("up/down: 移動 | tab: ビュー切替 | enter: 閲覧・追記 | s: ステータス | p: ナレッジ化 | /: 絞り込み | q: 終了"))
	} else if m.state == memoStateFilter {
		sb.WriteString(helpStyle.Render("文字を入力して絞り込み | enter/esc: 完了"))
	}

	return sb.String()
}
