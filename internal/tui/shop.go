package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jomei/notionapi"
	"github.com/ookawayuuto/notion-tui/internal/domain"
)

type NotionClient interface {
	FetchData(ctx context.Context) ([]domain.InventoryItem, []domain.ShoppingItem, error)
	AddShoppingItem(ctx context.Context, name string) (domain.ShoppingItem, error)
	CheckShoppingItem(ctx context.Context, pageID notionapi.ObjectID) error
	UpdateStock(ctx context.Context, pageID notionapi.ObjectID, newStock int) error
}

type ShopModel struct {
	client NotionClient

	activePane  int
	leftCursor  int
	rightCursor int

	allItems []domain.InventoryItem
	items    []domain.InventoryItem
	shopping []domain.ShoppingItem

	categories    []string
	categoryIndex int
	loading  bool
	err      error
}

type dataLoadedMsg struct {
	items    []domain.InventoryItem
	shopping []domain.ShoppingItem
}
type itemAddedMsg struct{ item domain.ShoppingItem }
type itemCheckedMsg struct {
	id       notionapi.ObjectID
	hasInv   bool
	invIndex int
}
type stockUpdatedMsg struct{}
type errMsg struct{ err error }

func NewShopModel(client NotionClient) ShopModel {
	return ShopModel{
		client:     client,
		loading:    true,
		categories: []string{"全て"},
	}
}

func filterInventoryItems(all []domain.InventoryItem, category string) []domain.InventoryItem {
	if category == "全て" {
		return all
	}
	var filtered []domain.InventoryItem
	for _, item := range all {
		for _, cat := range item.Categories {
			if cat == category {
				filtered = append(filtered, item)
				break
			}
		}
	}
	return filtered
}

func (m ShopModel) Init() tea.Cmd {
	return func() tea.Msg {
		items, shopping, err := m.client.FetchData(context.Background())
		if err != nil {
			return errMsg{err}
		}
		return dataLoadedMsg{items: items, shopping: shopping}
	}
}

func (m ShopModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case dataLoadedMsg:
		m.allItems = msg.items
		
		catMap := make(map[string]bool)
		m.categories = []string{"全て"}
		for _, item := range m.allItems {
			for _, cat := range item.Categories {
				if !catMap[cat] {
					catMap[cat] = true
					m.categories = append(m.categories, cat)
				}
			}
		}

		m.items = filterInventoryItems(m.allItems, m.categories[m.categoryIndex])
		m.shopping = msg.shopping
		m.loading = false
		return m, nil

	case itemAddedMsg:
		m.shopping = append(m.shopping, msg.item)
		return m, nil

	case itemCheckedMsg:
		var newShopping []domain.ShoppingItem
		for _, item := range m.shopping {
			if item.ID != msg.id {
				newShopping = append(newShopping, item)
			}
		}
		m.shopping = newShopping
		if m.rightCursor >= len(m.shopping) && m.rightCursor > 0 {
			m.rightCursor = len(m.shopping) - 1
		}
		if msg.hasInv && msg.invIndex < len(m.items) {
			m.items[msg.invIndex].Stock++
		}
		return m, nil

	case stockUpdatedMsg:
		return m, nil

	case errMsg:
		m.err = msg.err
		m.loading = false
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit

		case "tab", "right", "left":
			if m.activePane == 0 {
				m.activePane = 1
			} else {
				m.activePane = 0
			}

		case "/":
			if m.activePane == 0 && len(m.categories) > 0 {
				m.categoryIndex = (m.categoryIndex + 1) % len(m.categories)
				m.items = filterInventoryItems(m.allItems, m.categories[m.categoryIndex])
				m.leftCursor = 0
			}

		case "up", "k":
			if m.activePane == 0 && m.leftCursor > 0 {
				m.leftCursor--
			} else if m.activePane == 1 && m.rightCursor > 0 {
				m.rightCursor--
			}

		case "down", "j":
			if m.activePane == 0 && m.leftCursor < len(m.items)-1 {
				m.leftCursor++
			} else if m.activePane == 1 && m.rightCursor < len(m.shopping)-1 {
				m.rightCursor++
			}

		case "+":
			if m.activePane == 0 && len(m.items) > 0 {
				m.items[m.leftCursor].Stock++
				newStock := m.items[m.leftCursor].Stock
				id := m.items[m.leftCursor].ID
				return m, func() tea.Msg {
					err := m.client.UpdateStock(context.Background(), id, newStock)
					if err != nil {
						return errMsg{err}
					}
					return stockUpdatedMsg{}
				}
			}

		case "-", "enter":
			if m.loading {
				return m, nil
			}

			if m.activePane == 0 && len(m.items) > 0 {
				item := m.items[m.leftCursor]
				var cmds []tea.Cmd

				if item.Stock > 0 {
					m.items[m.leftCursor].Stock--
					newStock := m.items[m.leftCursor].Stock
					cmds = append(cmds, func() tea.Msg {
						err := m.client.UpdateStock(context.Background(), item.ID, newStock)
						if err != nil {
							return errMsg{err}
						}
						return stockUpdatedMsg{}
					})
				}

				if m.items[m.leftCursor].Stock <= 0 {
					exists := false
					for _, s := range m.shopping {
						if s.Name == item.Name {
							exists = true
							break
						}
					}
					if !exists {
						cmds = append(cmds, func() tea.Msg {
							added, err := m.client.AddShoppingItem(context.Background(), item.Name)
							if err != nil {
								return errMsg{err}
							}
							return itemAddedMsg{item: added}
						})
					}
				}

				return m, tea.Batch(cmds...)
			}

			if m.activePane == 1 && len(m.shopping) > 0 {
				item := m.shopping[m.rightCursor]

				var invItem *domain.InventoryItem
				var invIndex int
				for i, iv := range m.items {
					if iv.Name == item.Name {
						invItem = &m.items[i]
						invIndex = i
						break
					}
				}

				return m, func() tea.Msg {
					err := m.client.CheckShoppingItem(context.Background(), item.ID)
					if err != nil {
						return errMsg{err}
					}

					if invItem != nil {
						err = m.client.UpdateStock(context.Background(), invItem.ID, invItem.Stock+1)
						if err != nil {
							return errMsg{err}
						}
					}

					return itemCheckedMsg{id: item.ID, hasInv: invItem != nil, invIndex: invIndex}
				}
			}
		}
	}
	return m, nil
}

func (m ShopModel) View() string {
	if m.err != nil {
		return fmt.Sprintf("\n  ❌ エラー: %v\n\n  q で終了します\n", m.err)
	}
	if m.loading {
		return "\n  🔄 Notionと同期中...\n"
	}

	titleStyle := lipgloss.NewStyle().Bold(true).MarginBottom(1)
	activeBorder := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("205")).Padding(1, 2).Width(35)
	inactiveBorder := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(1, 2).Width(35)
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	warningStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)

	leftStyle := inactiveBorder
	if m.activePane == 0 {
		leftStyle = activeBorder
	}

	s := titleStyle.Foreground(lipgloss.Color("205")).Render("📦 在庫マスター") + "\n"
	
	// 分類タブを描画
	tabStr := ""
	for i, cat := range m.categories {
		if i == m.categoryIndex {
			tabStr += lipgloss.NewStyle().Foreground(lipgloss.Color("43")).Bold(true).Render("["+cat+"]") + " "
		} else {
			tabStr += lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(cat) + " "
		}
	}
	s += tabStr + "\n\n"

	if len(m.items) == 0 {
		s += lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("データがありません\n")
	}
	for i, item := range m.items {
		cursor := "  "
		stockStr := fmt.Sprintf("[%d] %s", item.Stock, item.Name)
		if item.Stock <= 0 {
			stockStr = warningStyle.Render(stockStr + " ⚠️ 空")
		}

		if m.activePane == 0 && m.leftCursor == i {
			cursor = "> "
			s += selectedStyle.Render(cursor) + stockStr + "\n"
		} else {
			s += cursor + stockStr + "\n"
		}
	}
	leftPane := leftStyle.Render(s)

	rightStyle := inactiveBorder
	if m.activePane == 1 {
		rightStyle = activeBorder
	}

	rightContent := titleStyle.Foreground(lipgloss.Color("43")).Render("🛒 買い物リスト") + "\n"
	if len(m.shopping) == 0 {
		rightContent += lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("リストは空です")
	} else {
		for i, item := range m.shopping {
			cursor := "  "
			if m.activePane == 1 && m.rightCursor == i {
				cursor = "> "
				rightContent += selectedStyle.Render(cursor+"[ ] "+item.Name) + "\n"
			} else {
				rightContent += cursor + "[ ] " + item.Name + "\n"
			}
		}
	}
	rightPane := rightStyle.Render(rightContent)

	ui := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, "   ", rightPane)
	footer := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).MarginTop(1).Render("Tab: ペイン切替  /: 分類切替  +/-/Enter: 在庫増減/完了  q: 終了")

	return "\n" + ui + "\n" + footer + "\n"
}
