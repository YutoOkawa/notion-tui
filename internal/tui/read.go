package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jomei/notionapi"
	"github.com/ookawayuuto/notion-tui/internal/domain"
)

type ReadClientInterface interface {
	FetchBooks(ctx context.Context) ([]domain.Book, error)
	UpdateBookProgress(ctx context.Context, id notionapi.ObjectID, readPages int, newStatus string) error
	CreateBook(ctx context.Context, title string, totalPages int) error
}

type ReadMode int

const (
	ModeList ReadMode = iota
	ModeInputTitle
	ModeInputPages
)

type ReadModel struct {
	client ReadClientInterface

	allBooks []domain.Book
	books    []domain.Book

	statuses    []string
	statusIndex int

	cursor     int
	listOffset int

	loading bool
	err     error

	mode         ReadMode
	textInput    textinput.Model
	newBookTitle string
}

type booksLoadedMsg struct {
	books []domain.Book
}
type progressUpdatedMsg struct{}

func NewReadModel(client ReadClientInterface) ReadModel {
	ti := textinput.New()
	ti.Focus()

	return ReadModel{
		client:    client,
		loading:   true,
		statuses:  []string{"全て"},
		mode:      ModeList,
		textInput: ti,
	}
}

func (m ReadModel) Init() tea.Cmd {
	return func() tea.Msg {
		books, err := m.client.FetchBooks(context.Background())
		if err != nil {
			return errMsg{err}
		}
		return booksLoadedMsg{books: books}
	}
}

func filterBooks(all []domain.Book, status string) []domain.Book {
	if status == "全て" {
		return all
	}
	var filtered []domain.Book
	for _, b := range all {
		if b.Status == status {
			filtered = append(filtered, b)
		}
	}
	return filtered
}

func (m ReadModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case booksLoadedMsg:
		m.allBooks = msg.books

		// 以前の選択状態を記憶（初回は In Progress）
		targetStatus := "In Progress"
		if len(m.statuses) > m.statusIndex && m.statusIndex != 0 {
			targetStatus = m.statuses[m.statusIndex]
		}

		statusMap := make(map[string]bool)
		m.statuses = []string{"全て"}
		for _, b := range m.allBooks {
			if b.Status != "" && !statusMap[b.Status] {
				statusMap[b.Status] = true
				m.statuses = append(m.statuses, b.Status)
			}
		}

		m.statusIndex = 0 // 見つからなかった場合は「全て」
		for i, s := range m.statuses {
			if s == targetStatus || s == "In progress" {
				m.statusIndex = i
				break
			}
		}

		m.books = filterBooks(m.allBooks, m.statuses[m.statusIndex])
		if m.cursor >= len(m.books) {
			m.cursor = 0
			m.listOffset = 0
		}
		m.loading = false
		return m, nil

	case progressUpdatedMsg:
		m.loading = true
		return m, m.Init()

	case errMsg:
		m.err = msg.err
		m.loading = false
		return m, nil

	case tea.KeyMsg:
		if m.loading {
			return m, nil
		}

		if m.mode != ModeList {
			switch msg.String() {
			case "esc":
				m.mode = ModeList
				return m, nil
			case "enter":
				if m.mode == ModeInputTitle {
					m.newBookTitle = m.textInput.Value()
					if m.newBookTitle == "" {
						m.mode = ModeList
						return m, nil
					}
					m.mode = ModeInputPages
					m.textInput.SetValue("")
					m.textInput.Placeholder = "総ページ数を入力 (例: 300)..."
					return m, nil
				} else if m.mode == ModeInputPages {
					pagesStr := m.textInput.Value()
					totalPages, _ := strconv.Atoi(pagesStr)

					m.loading = true
					title := m.newBookTitle
					m.mode = ModeList
					return m, func() tea.Msg {
						err := m.client.CreateBook(context.Background(), title, totalPages)
						if err != nil {
							return errMsg{err}
						}
						return progressUpdatedMsg{}
					}
				}
			}

			var cmd tea.Cmd
			m.textInput, cmd = m.textInput.Update(msg)
			return m, cmd
		}

		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit

		case "a":
			m.mode = ModeInputTitle
			m.textInput.SetValue("")
			m.textInput.Placeholder = "書籍タイトルを入力..."
			m.textInput.Focus()
			return m, textinput.Blink

		case "/":
			if len(m.statuses) > 0 {
				m.statusIndex = (m.statusIndex + 1) % len(m.statuses)
				m.books = filterBooks(m.allBooks, m.statuses[m.statusIndex])
				m.cursor = 0
				m.listOffset = 0
			}

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				if m.cursor < m.listOffset {
					m.listOffset--
				}
			}

		case "down", "j":
			if m.cursor < len(m.books)-1 {
				m.cursor++
				if m.cursor >= m.listOffset+10 {
					m.listOffset++
				}
			}

		case "right", "l":
			if len(m.books) > 0 {
				b := m.books[m.cursor]
				if b.ReadPages < b.TotalPages || b.TotalPages == 0 {
					b.ReadPages++
					m.books[m.cursor] = b
					for i, ab := range m.allBooks {
						if ab.ID == b.ID {
							m.allBooks[i] = b
							break
						}
					}
				}
			}

		case "L", "shift+right":
			if len(m.books) > 0 {
				b := m.books[m.cursor]
				b.ReadPages += 10
				if b.TotalPages > 0 && b.ReadPages > b.TotalPages {
					b.ReadPages = b.TotalPages
				}
				m.books[m.cursor] = b
				for i, ab := range m.allBooks {
					if ab.ID == b.ID {
						m.allBooks[i] = b
						break
					}
				}
			}

		case "left", "h":
			if len(m.books) > 0 {
				b := m.books[m.cursor]
				if b.ReadPages > 0 {
					b.ReadPages--
					m.books[m.cursor] = b
					for i, ab := range m.allBooks {
						if ab.ID == b.ID {
							m.allBooks[i] = b
							break
						}
					}
				}
			}

		case "H", "shift+left":
			if len(m.books) > 0 {
				b := m.books[m.cursor]
				b.ReadPages -= 10
				if b.ReadPages < 0 {
					b.ReadPages = 0
				}
				m.books[m.cursor] = b
				for i, ab := range m.allBooks {
					if ab.ID == b.ID {
						m.allBooks[i] = b
						break
					}
				}
			}

		case "s":
			if len(m.books) > 0 {
				b := m.books[m.cursor]
				m.loading = true

				newStatus := ""
				if b.TotalPages > 0 && b.ReadPages >= b.TotalPages && b.Status != "Done" {
					newStatus = "Done"
				} else if b.ReadPages > 0 && b.ReadPages < b.TotalPages && (b.Status == "Not started" || b.Status == "Not Started" || b.Status == "未着手") {
					newStatus = "In Progress"
				}

				return m, func() tea.Msg {
					err := m.client.UpdateBookProgress(context.Background(), b.ID, b.ReadPages, newStatus)
					if err != nil {
						return errMsg{err}
					}
					return progressUpdatedMsg{}
				}
			}
		}
	}
	return m, nil
}

func (m ReadModel) View() string {
	if m.err != nil {
		return fmt.Sprintf("\n  ❌ エラー: %v\n\n  q で終了します\n", m.err)
	}
	if m.loading {
		return "\n  🔄 Notionと同期中...\n"
	}

	activeBorder := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("205")).Padding(1, 2).Width(60)

	if m.mode != ModeList {
		s := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Render("📚 新しい書籍を追加します") + "\n\n"
		if m.mode == ModeInputTitle {
			s += lipgloss.NewStyle().Foreground(lipgloss.Color("43")).Render("タイトル:") + "\n"
		} else {
			s += lipgloss.NewStyle().Foreground(lipgloss.Color("43")).Render(fmt.Sprintf("「%s」の総ページ数:", m.newBookTitle)) + "\n"
		}
		s += m.textInput.View() + "\n\n"
		
		ui := activeBorder.Render(s)
		footer := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).MarginTop(1).Render("[Enter]: 次へ/完了  [Esc]: キャンセル")
		return "\n  " + strings.ReplaceAll(ui, "\n", "\n  ") + "\n  " + footer + "\n"
	}

	titleStyle := lipgloss.NewStyle().Bold(true).MarginBottom(1)
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	progressStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("43"))

	s := titleStyle.Foreground(lipgloss.Color("205")).Render("📚 読書管理マスター") + "\n"

	tabStr := ""
	for i, stat := range m.statuses {
		if i == m.statusIndex {
			tabStr += lipgloss.NewStyle().Foreground(lipgloss.Color("43")).Bold(true).Render("["+stat+"]") + " "
		} else {
			tabStr += lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(stat) + " "
		}
	}
	s += tabStr + "\n\n"

	if len(m.books) == 0 {
		s += lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("データがありません\n")
	}

	for i := m.listOffset; i < m.listOffset+10 && i < len(m.books); i++ {
		b := m.books[i]

		blocks := 10
		filled := 0
		if b.TotalPages > 0 {
			filled = int((float64(b.ReadPages) / float64(b.TotalPages)) * float64(blocks))
			if filled > blocks {
				filled = blocks
			}
		}

		bar := ""
		for j := 0; j < blocks; j++ {
			if j < filled {
				bar += "█"
			} else {
				bar += "░"
			}
		}

		line := fmt.Sprintf("[%s] %s\n      進捗: [%s] %3d / %3d ページ\n", b.Status, b.Title, progressStyle.Render(bar), b.ReadPages, b.TotalPages)

		if i == m.cursor {
			s += selectedStyle.Render("> "+strings.ReplaceAll(line, "\n      ", "\n      ")) + "\n"
		} else {
			s += "  " + line + "\n"
		}
	}

	ui := activeBorder.Render(s)
	footer := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).MarginTop(1).Render("↑/↓: 選択  /: ステータス切替  ←/→: 進捗±1  Shift+←/→: 進捗±10\na: 追加  s: 保存  q: 終了")

	return "\n  " + strings.ReplaceAll(ui, "\n", "\n  ") + "\n  " + strings.ReplaceAll(footer, "\n", "\n  ") + "\n"
}
