package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type KBClient interface {
	FetchCategories(ctx context.Context) ([]string, error)
	AddDraftItem(ctx context.Context, title string, category string) (string, error)
}

type kbState int

const (
	kbStateInput kbState = iota
	kbStateCategory
	kbStateCreating
	kbStateDone
)

type KBModel struct {
	client     KBClient
	state      kbState
	textInput  textinput.Model
	list       list.Model
	categories []string
	title      string
	category   string
	err        error
	resultURL  string
}

type categoryItem string

func (i categoryItem) Title() string       { return string(i) }
func (i categoryItem) Description() string { return "" }
func (i categoryItem) FilterValue() string { return string(i) }

func NewKBModel(client KBClient, categories []string) *KBModel {
	ti := textinput.New()
	ti.Placeholder = "調査してほしいテーマを入力..."
	ti.Focus()
	ti.CharLimit = 156
	ti.Width = 50

	var items []list.Item
	for _, c := range categories {
		items = append(items, categoryItem(c))
	}
	
	// Create list if we have categories, else it will be empty
	l := list.New(items, list.NewDefaultDelegate(), 30, 15)
	l.Title = "カテゴリを選択"
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)

	return &KBModel{
		client:     client,
		state:      kbStateInput,
		textInput:  ti,
		list:       l,
		categories: categories,
	}
}

type createMsg struct {
	url string
}

type kbErrMsg struct{ err error }

func (e kbErrMsg) Error() string { return e.err.Error() }

func (m *KBModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m *KBModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		case tea.KeyEnter:
			if m.state == kbStateInput {
				val := strings.TrimSpace(m.textInput.Value())
				if val != "" {
					m.title = val
					if len(m.categories) > 0 {
						m.state = kbStateCategory
						return m, nil
					}
					// No categories to select, go straight to creating
					m.state = kbStateCreating
					return m, m.createDraft(m.title, "")
				}
			} else if m.state == kbStateCategory {
				if i, ok := m.list.SelectedItem().(categoryItem); ok {
					m.category = string(i)
				}
				m.state = kbStateCreating
				return m, m.createDraft(m.title, m.category)
			} else if m.state == kbStateDone {
				return m, tea.Quit
			}
		}

	case createMsg:
		m.state = kbStateDone
		m.resultURL = msg.url
		return m, tea.Quit

	case kbErrMsg:
		m.err = msg.err
		return m, tea.Quit
	}

	var cmd tea.Cmd
	if m.state == kbStateInput {
		m.textInput, cmd = m.textInput.Update(msg)
	} else if m.state == kbStateCategory {
		m.list, cmd = m.list.Update(msg)
	}
	return m, cmd
}

func (m *KBModel) View() string {
	if m.err != nil {
		return fmt.Sprintf("エラーが発生しました: %v\n", m.err)
	}

	switch m.state {
	case kbStateInput:
		return fmt.Sprintf(
			"AIに調査してほしいテーマを入力してください:\n\n%s\n\n%s",
			m.textInput.View(),
			lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("(esc to quit)"),
		)
	case kbStateCategory:
		return fmt.Sprintf(
			"テーマ: %s\n\n%s\n",
			lipgloss.NewStyle().Foreground(lipgloss.Color("36")).Render(m.title),
			m.list.View(),
		)
	case kbStateCreating:
		return "ナレッジベースに登録中...\n"
	case kbStateDone:
		return fmt.Sprintf("登録完了しました！\nURL: %s\n", m.resultURL)
	}

	return ""
}

func (m *KBModel) createDraft(title, category string) tea.Cmd {
	return func() tea.Msg {
		url, err := m.client.AddDraftItem(context.Background(), title, category)
		if err != nil {
			return kbErrMsg{err}
		}
		return createMsg{url}
	}
}
