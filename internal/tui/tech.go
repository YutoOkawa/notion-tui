package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jomei/notionapi"
	"github.com/ookawayuuto/notion-tui/internal/domain"
)

type TechClientInterface interface {
	FetchTechItems(ctx context.Context) ([]domain.TechItem, error)
	UpdateTechItem(ctx context.Context, pageID notionapi.ObjectID, genre string, tags []string, status string, memo string) error
	ToggleStatus(ctx context.Context, pageID notionapi.ObjectID, newStatus string) error
}

type TechTab int

const (
	TechTabUnlabeled TechTab = iota
	TechTabAll
	TechTabUnread
	TechTabRead
)

type TechViewMode int

const (
	TechViewModeBrowse TechViewMode = iota
	TechViewModeSearch
	TechViewModeEditGenre
	TechViewModeEditTags
	TechViewModeEditStatus
)

var (
	DefaultGenres = []string{
		"Frontend", "Backend", "Infra/Cloud", "AI/ML", "DevOps/SRE", "Architecture", "Management", "Other",
	}
	DefaultTags = []string{
		"Go", "TypeScript", "Next.js", "AWS", "Docker", "LLM", "Python", "Rust", "Kubernetes", "Database", "Security",
	}
	MasterStatuses = []string{
		"未読", "読書中", "読了", "要復習",
	}
)

type TechModel struct {
	client       TechClientInterface
	allTechItems []domain.TechItem
	items        []domain.TechItem

	genres []string
	tags   []string

	tab        TechTab
	cursor     int
	listOffset int

	mode   TechViewMode
	search textinput.Model

	// Label/Status editing state
	editItemIdx   int
	selectedGenre string
	genreCursor   int
	selectedTags  map[string]bool
	tagCursor     int
	statusCursor  int

	loading   bool
	statusMsg string
	err       error
}

type techItemsLoadedMsg struct {
	items []domain.TechItem
}

type techItemUpdatedMsg struct {
	id     notionapi.ObjectID
	genre  string
	tags   []string
	status string
}

type techErrMsg struct {
	err error
}

func NewTechModel(client TechClientInterface) TechModel {
	ti := textinput.New()
	ti.Placeholder = "タイトル、ジャンル、タグで検索..."
	ti.CharLimit = 50
	ti.Width = 40

	return TechModel{
		client:       client,
		genres:       append([]string(nil), DefaultGenres...),
		tags:         append([]string(nil), DefaultTags...),
		tab:          TechTabUnlabeled,
		mode:         TechViewModeBrowse,
		search:       ti,
		loading:      true,
		selectedTags: make(map[string]bool),
	}
}

func (m TechModel) Init() tea.Cmd {
	return m.fetchTechItemsCmd()
}

func (m TechModel) fetchTechItemsCmd() tea.Cmd {
	return func() tea.Msg {
		items, err := m.client.FetchTechItems(context.Background())
		if err != nil {
			return techErrMsg{err: err}
		}
		return techItemsLoadedMsg{items: items}
	}
}

func (m *TechModel) mergeDynamicMaster() {
	genreSet := make(map[string]bool)
	for _, g := range DefaultGenres {
		genreSet[g] = true
	}
	tagSet := make(map[string]bool)
	for _, t := range DefaultTags {
		tagSet[t] = true
	}

	for _, item := range m.allTechItems {
		if item.Genre != "" && !genreSet[item.Genre] {
			genreSet[item.Genre] = true
			m.genres = append(m.genres, item.Genre)
		}
		for _, tag := range item.Tags {
			if tag != "" && !tagSet[tag] {
				tagSet[tag] = true
				m.tags = append(m.tags, tag)
			}
		}
	}
}

func (m TechModel) filterItems() []domain.TechItem {
	query := strings.ToLower(strings.TrimSpace(m.search.Value()))
	var filtered []domain.TechItem

	for _, item := range m.allTechItems {
		// Tab filter
		switch m.tab {
		case TechTabUnlabeled:
			if !item.IsUnlabeled() {
				continue
			}
		case TechTabUnread:
			if !item.IsUnread() {
				continue
			}
		case TechTabRead:
			if !item.IsRead() {
				continue
			}
		case TechTabAll:
			// No filter
		}

		// Search filter
		if query != "" {
			match := strings.Contains(strings.ToLower(item.Title), query) ||
				strings.Contains(strings.ToLower(item.Genre), query)
			if !match {
				for _, t := range item.Tags {
					if strings.Contains(strings.ToLower(t), query) {
						match = true
						break
					}
				}
			}
			if !match {
				continue
			}
		}

		filtered = append(filtered, item)
	}

	return filtered
}

func (m *TechModel) applyFilter() {
	m.items = m.filterItems()
	if m.cursor >= len(m.items) {
		m.cursor = 0
		m.listOffset = 0
	}
}

func (m TechModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case techItemsLoadedMsg:
		m.allTechItems = msg.items
		m.mergeDynamicMaster()
		m.loading = false
		m.applyFilter()
		return m, nil

	case techItemUpdatedMsg:
		// In-memory update
		for i, item := range m.allTechItems {
			if item.ID == msg.id {
				if msg.genre != "" {
					m.allTechItems[i].Genre = msg.genre
				}
				if msg.tags != nil {
					m.allTechItems[i].Tags = msg.tags
				}
				if msg.status != "" {
					m.allTechItems[i].Status = msg.status
				}
				break
			}
		}
		m.statusMsg = "Notionに保存しました"
		m.applyFilter()
		return m, nil

	case techErrMsg:
		m.err = msg.err
		m.loading = false
		return m, nil

	case tea.KeyMsg:
		if m.loading {
			return m, nil
		}

		// Search mode
		if m.mode == TechViewModeSearch {
			switch msg.Type {
			case tea.KeyEsc, tea.KeyEnter:
				m.mode = TechViewModeBrowse
				m.search.Blur()
				m.applyFilter()
				return m, nil
			default:
				var cmd tea.Cmd
				m.search, cmd = m.search.Update(msg)
				m.applyFilter()
				return m, cmd
			}
		}

		// Edit Genre Mode
		if m.mode == TechViewModeEditGenre {
			switch msg.String() {
			case "esc":
				m.mode = TechViewModeBrowse
				return m, nil
			case "up", "k":
				if m.genreCursor > 0 {
					m.genreCursor--
				}
			case "down", "j":
				if m.genreCursor < len(m.genres) {
					m.genreCursor++
				}
			case "enter":
				if m.genreCursor < len(m.genres) {
					m.selectedGenre = m.genres[m.genreCursor]
				} else {
					m.selectedGenre = ""
				}
				m.mode = TechViewModeEditTags
				m.tagCursor = 0
				return m, nil
			}
			return m, nil
		}

		// Edit Tags Mode
		if m.mode == TechViewModeEditTags {
			switch msg.String() {
			case "esc":
				m.mode = TechViewModeBrowse
				return m, nil
			case "up", "k":
				if m.tagCursor > 0 {
					m.tagCursor--
				}
			case "down", "j":
				if m.tagCursor < len(m.tags)-1 {
					m.tagCursor++
				}
			case " ":
				tag := m.tags[m.tagCursor]
				m.selectedTags[tag] = !m.selectedTags[tag]
				return m, nil
			case "enter":
				var tags []string
				for _, t := range m.tags {
					if m.selectedTags[t] {
						tags = append(tags, t)
					}
				}
				item := m.items[m.editItemIdx]
				genre := m.selectedGenre
				m.mode = TechViewModeBrowse
				m.statusMsg = "ラベルを保存中..."

				// In-memory update immediately
				for i, it := range m.allTechItems {
					if it.ID == item.ID {
						m.allTechItems[i].Genre = genre
						m.allTechItems[i].Tags = tags
						break
					}
				}
				m.applyFilter()

				return m, func() tea.Msg {
					err := m.client.UpdateTechItem(context.Background(), item.ID, genre, tags, "", "")
					if err != nil {
						return techErrMsg{err: err}
					}
					return techItemUpdatedMsg{id: item.ID, genre: genre, tags: tags}
				}
			}
			return m, nil
		}

		// Edit Status Mode
		if m.mode == TechViewModeEditStatus {
			switch msg.String() {
			case "esc":
				m.mode = TechViewModeBrowse
				return m, nil
			case "up", "k":
				if m.statusCursor > 0 {
					m.statusCursor--
				}
			case "down", "j":
				if m.statusCursor < len(MasterStatuses)-1 {
					m.statusCursor++
				}
			case "enter":
				newStatus := MasterStatuses[m.statusCursor]
				item := m.items[m.editItemIdx]
				m.mode = TechViewModeBrowse
				m.statusMsg = "ステータス更新中..."

				for i, it := range m.allTechItems {
					if it.ID == item.ID {
						m.allTechItems[i].Status = newStatus
						break
					}
				}
				m.applyFilter()

				return m, func() tea.Msg {
					err := m.client.ToggleStatus(context.Background(), item.ID, newStatus)
					if err != nil {
						return techErrMsg{err: err}
					}
					return techItemUpdatedMsg{id: item.ID, status: newStatus}
				}
			}
			return m, nil
		}

		// Normal Browse mode
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit

		case "tab", "right", "l":
			m.tab = (m.tab + 1) % 4
			m.cursor = 0
			m.listOffset = 0
			m.applyFilter()
			return m, nil

		case "shift+tab", "left", "h":
			m.tab = (m.tab - 1 + 4) % 4
			m.cursor = 0
			m.listOffset = 0
			m.applyFilter()
			return m, nil

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				if m.cursor < m.listOffset {
					m.listOffset = m.cursor
				}
			}

		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
				if m.cursor >= m.listOffset+10 {
					m.listOffset++
				}
			}

		case "/":
			m.mode = TechViewModeSearch
			m.search.Focus()
			return m, textinput.Blink

		case "s":
			// Fast toggle: "未読" <-> "読了"
			if len(m.items) > 0 {
				item := m.items[m.cursor]
				newStatus := "読了"
				if item.Status == "読了" {
					newStatus = "未読"
				}
				m.statusMsg = fmt.Sprintf("ステータスを「%s」に更新中...", newStatus)

				for i, it := range m.allTechItems {
					if it.ID == item.ID {
						m.allTechItems[i].Status = newStatus
						break
					}
				}
				m.applyFilter()

				return m, func() tea.Msg {
					err := m.client.ToggleStatus(context.Background(), item.ID, newStatus)
					if err != nil {
						return techErrMsg{err: err}
					}
					return techItemUpdatedMsg{id: item.ID, status: newStatus}
				}
			}

		case "S":
			// Open detailed status selection modal
			if len(m.items) > 0 {
				item := m.items[m.cursor]
				m.editItemIdx = m.cursor
				m.mode = TechViewModeEditStatus
				m.statusCursor = 0
				for i, s := range MasterStatuses {
					if s == item.Status {
						m.statusCursor = i
						break
					}
				}
				return m, nil
			}

		case "o", "enter":
			// Open URL in browser
			if len(m.items) > 0 {
				item := m.items[m.cursor]
				if item.URL != "" {
					openBrowser(item.URL)
					m.statusMsg = fmt.Sprintf("ブラウザで開きました: %s", item.URL)
				} else {
					m.statusMsg = "URLが登録されていません"
				}
			}

		case "e":
			// Open label editor
			if len(m.items) > 0 {
				item := m.items[m.cursor]
				m.editItemIdx = m.cursor
				m.mode = TechViewModeEditGenre
				m.genreCursor = 0
				m.selectedGenre = item.Genre
				m.selectedTags = make(map[string]bool)
				for _, t := range item.Tags {
					m.selectedTags[t] = true
				}
				for i, g := range m.genres {
					if g == item.Genre {
						m.genreCursor = i
						break
					}
				}
				return m, nil
			}
		}
	}

	return m, nil
}

func (m TechModel) View() string {
	if m.err != nil {
		return fmt.Sprintf("\n  ❌ エラー: %v\n\n  q で終了します\n", m.err)
	}
	if m.loading {
		return "\n  🌐 NotionからTechBlog Stackデータを取得中...\n"
	}

	activeBorder := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("39")).Padding(1, 2).Width(80)

	// Modal: Edit Genre
	if m.mode == TechViewModeEditGenre {
		var sb strings.Builder
		sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render("🏷️  ジャンルを選択") + "\n\n")
		item := m.items[m.editItemIdx]
		sb.WriteString(fmt.Sprintf("対象: %s\n\n", lipgloss.NewStyle().Bold(true).Render(item.Title)))

		for i, g := range m.genres {
			cursor := "  "
			if i == m.genreCursor {
				cursor = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render("> ")
			}
			isSelected := ""
			if g == m.selectedGenre {
				isSelected = lipgloss.NewStyle().Foreground(lipgloss.Color("43")).Render(" (選択中)")
			}
			sb.WriteString(fmt.Sprintf("%s%s%s\n", cursor, g, isSelected))
		}
		noneIdx := len(m.genres)
		cursor := "  "
		if m.genreCursor == noneIdx {
			cursor = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render("> ")
		}
		sb.WriteString(fmt.Sprintf("%s(未設定)\n", cursor))

		ui := activeBorder.Render(sb.String())
		footer := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).MarginTop(1).Render("↑/↓: 選択  Enter: 決定してタグ選択へ  Esc: キャンセル")
		return "\n  " + strings.ReplaceAll(ui, "\n", "\n  ") + "\n  " + footer + "\n"
	}

	// Modal: Edit Tags
	if m.mode == TechViewModeEditTags {
		var sb strings.Builder
		sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render("🏷️  タグを選択 (複数選択可)") + "\n\n")
		item := m.items[m.editItemIdx]
		sb.WriteString(fmt.Sprintf("対象: %s\n", lipgloss.NewStyle().Bold(true).Render(item.Title)))
		sb.WriteString(fmt.Sprintf("ジャンル: %s\n\n", lipgloss.NewStyle().Foreground(lipgloss.Color("33")).Render(m.selectedGenre)))

		for i, t := range m.tags {
			cursor := "  "
			if i == m.tagCursor {
				cursor = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render("> ")
			}
			check := "[ ]"
			if m.selectedTags[t] {
				check = lipgloss.NewStyle().Foreground(lipgloss.Color("43")).Render("[✓]")
			}
			sb.WriteString(fmt.Sprintf("%s%s %s\n", cursor, check, t))
		}

		ui := activeBorder.Render(sb.String())
		footer := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).MarginTop(1).Render("↑/↓: 移動  Space: チェック切替  Enter: 保存  Esc: キャンセル")
		return "\n  " + strings.ReplaceAll(ui, "\n", "\n  ") + "\n  " + footer + "\n"
	}

	// Modal: Edit Status
	if m.mode == TechViewModeEditStatus {
		var sb strings.Builder
		sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render("📌 ステータスを選択") + "\n\n")
		item := m.items[m.editItemIdx]
		sb.WriteString(fmt.Sprintf("対象: %s\n\n", lipgloss.NewStyle().Bold(true).Render(item.Title)))

		for i, s := range MasterStatuses {
			cursor := "  "
			if i == m.statusCursor {
				cursor = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render("> ")
			}
			isSelected := ""
			if s == item.Status {
				isSelected = lipgloss.NewStyle().Foreground(lipgloss.Color("43")).Render(" (現在)")
			}
			sb.WriteString(fmt.Sprintf("%s%s%s\n", cursor, s, isSelected))
		}

		ui := activeBorder.Render(sb.String())
		footer := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).MarginTop(1).Render("↑/↓: 選択  Enter: 保存  Esc: キャンセル")
		return "\n  " + strings.ReplaceAll(ui, "\n", "\n  ") + "\n  " + footer + "\n"
	}

	// Main Browse View
	var sb strings.Builder

	// Header
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	sb.WriteString(titleStyle.Render("⚡ TechBlog Stack (ntui tech)") + "\n\n")

	// Tabs
	tabs := []string{"未分類", "すべて", "未読", "読了"}
	var tabStrs []string
	activeTabStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Underline(true)
	inactiveTabStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	for i, t := range tabs {
		label := fmt.Sprintf("[%s]", t)
		if TechTab(i) == m.tab {
			tabStrs = append(tabStrs, activeTabStyle.Render(label))
		} else {
			tabStrs = append(tabStrs, inactiveTabStyle.Render(label))
		}
	}
	sb.WriteString(strings.Join(tabStrs, "  ") + "\n\n")

	// Search bar if active
	if m.mode == TechViewModeSearch || m.search.Value() != "" {
		sb.WriteString(m.search.View() + "\n\n")
	}

	// Status notification
	if m.statusMsg != "" {
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("43")).Render("💡 "+m.statusMsg) + "\n\n")
	}

	// List
	if len(m.items) == 0 {
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("該当する記事はありません。\n"))
	} else {
		displayLimit := 10
		start := m.listOffset
		end := start + displayLimit
		if end > len(m.items) {
			end = len(m.items)
		}

		genreStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("33"))
		tagStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("141"))
		statusReadStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("43")).Bold(true)
		statusUnreadStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("75"))
		statusOtherStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
		unlabeledBadge := lipgloss.NewStyle().Background(lipgloss.Color("196")).Foreground(lipgloss.Color("231")).Bold(true).Render(" 未分類 ")

		for i := start; i < end; i++ {
			it := m.items[i]
			cursor := "  "
			if i == m.cursor {
				cursor = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render("> ")
			}

			statusIcon := statusUnreadStyle.Render("[未読]")
			if it.Status == "読了" {
				statusIcon = statusReadStyle.Render("[読了]")
			} else if it.Status == "読書中" {
				statusIcon = statusOtherStyle.Render("[読書中]")
			} else if it.Status == "要復習" {
				statusIcon = statusOtherStyle.Render("[要復習]")
			}

			genreText := ""
			if it.Genre != "" {
				genreText = genreStyle.Render(fmt.Sprintf("[%s]", it.Genre)) + " "
			}

			tagText := ""
			if len(it.Tags) > 0 {
				tagText = tagStyle.Render(fmt.Sprintf("(%s)", strings.Join(it.Tags, ", "))) + " "
			}

			badge := ""
			if it.IsUnlabeled() {
				badge = unlabeledBadge + " "
			}

			title := it.Title
			if i == m.cursor {
				title = lipgloss.NewStyle().Bold(true).Render(title)
			}

			line := fmt.Sprintf("%s%s %s%s%s%s", cursor, statusIcon, badge, genreText, tagText, title)
			sb.WriteString(line + "\n")

			// Display URL on active row if exists
			if i == m.cursor && it.URL != "" {
				urlStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("242")).PaddingLeft(6)
				sb.WriteString(urlStyle.Render("🔗 "+it.URL) + "\n")
			}
		}

		countInfo := fmt.Sprintf("\n  (全 %d 件中 %d〜%d 件目を表示)", len(m.items), start+1, end)
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(countInfo) + "\n")
	}

	ui := activeBorder.Render(sb.String())

	footer := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).MarginTop(1).Render(
		"↑/↓: 移動  Tab/←/→: タブ切替  o/Enter: ブラウザで開く  s: 未読/読了切替  e: ラベル編集  /: 検索  q: 終了",
	)

	return "\n  " + strings.ReplaceAll(ui, "\n", "\n  ") + "\n  " + footer + "\n"
}
