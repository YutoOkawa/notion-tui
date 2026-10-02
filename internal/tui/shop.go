package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jomei/notionapi"
	"github.com/ookawayuuto/notion-tui/internal/domain"
)

type NotionClient interface {
	FetchData(ctx context.Context) ([]domain.InventoryItem, []domain.ShoppingItem, error)
	AddShoppingItem(ctx context.Context, name string) (domain.ShoppingItem, error)
	AddInventoryItem(ctx context.Context, name string, stock int, categories []string) (domain.InventoryItem, error)
	CheckShoppingItem(ctx context.Context, pageID notionapi.ObjectID) error
	UpdateStock(ctx context.Context, pageID notionapi.ObjectID, newStock int) error
	UpdateExpirationDate(ctx context.Context, pageID notionapi.ObjectID, expirationDate string) error
	UpdateNextDeliveryDate(ctx context.Context, pageID notionapi.ObjectID, nextDeliveryDate string) error
	UpdateSubscription(ctx context.Context, pageID notionapi.ObjectID, isSub bool, cycle string, nextDeliveryDate string) error
}

type ShopState int

const (
	shopStateBrowse ShopState = iota
	shopStateInputExpire
	shopStateInputDeliveryDate
	shopStateSelectSubCycle
	shopStateInputNewInventory
	shopStateInputNewShopping
)

type ExpireFilterMode int

const (
	expireFilterNone ExpireFilterMode = iota
	expireFilterSortSoon
	expireFilterAlertOnly
)

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
	expireFilter  ExpireFilterMode

	state     ShopState
	textInput textinput.Model

	loading bool
	err     error
}

type dataLoadedMsg struct {
	items    []domain.InventoryItem
	shopping []domain.ShoppingItem
}
type itemAddedMsg struct{ item domain.ShoppingItem }
type inventoryItemAddedMsg struct{ item domain.InventoryItem }
type itemCheckedMsg struct {
	id       notionapi.ObjectID
	hasInv   bool
	invIndex int
}
type stockUpdatedMsg struct{}
type expirationUpdatedMsg struct {
	id             notionapi.ObjectID
	expirationDate string
}
type deliveryDateUpdatedMsg struct {
	id               notionapi.ObjectID
	nextDeliveryDate string
}
type subscriptionUpdatedMsg struct {
	id               notionapi.ObjectID
	isSubscription   bool
	subCycle         string
	nextDeliveryDate string
}
type errMsg struct{ err error }

func NewShopModel(client NotionClient) ShopModel {
	ti := textinput.New()
	ti.CharLimit = 10
	ti.Width = 20
	ti.Placeholder = "YYYY-MM-DD"

	return ShopModel{
		client:       client,
		loading:      true,
		categories:   []string{"全て"},
		expireFilter: expireFilterNone,
		state:        shopStateBrowse,
		textInput:    ti,
	}
}

func applyInventoryFilters(all []domain.InventoryItem, category string, mode ExpireFilterMode, now time.Time) []domain.InventoryItem {
	var filtered []domain.InventoryItem
	for _, item := range all {
		if category == "全て" {
			filtered = append(filtered, item)
			continue
		}
		for _, cat := range item.Categories {
			if cat == category {
				filtered = append(filtered, item)
				break
			}
		}
	}

	if mode == expireFilterAlertOnly {
		var alertItems []domain.InventoryItem
		for _, item := range filtered {
			expInfo := item.GetExpirationInfo(now)
			subInfo := item.GetSubscriptionInfo(now)
			hasExpAlert := expInfo.Status == domain.ExpirationStatusExpired || expInfo.Status == domain.ExpirationStatusExpiringSoon
			hasSubAlert := subInfo.Status == domain.SubscriptionStatusAlert || subInfo.Status == domain.SubscriptionStatusOverdue
			if hasExpAlert || hasSubAlert {
				alertItems = append(alertItems, item)
			}
		}
		filtered = alertItems
	}

	if mode == expireFilterSortSoon || mode == expireFilterAlertOnly {
		sort.SliceStable(filtered, func(i, j int) bool {
			// 定期便アラートまたは賞味期限が近いものを優先
			dateI := filtered[i].ExpirationDate
			dateJ := filtered[j].ExpirationDate
			if dateI == "" && dateJ == "" {
				// 次回配送日で比較
				subI := filtered[i].NextDeliveryDate
				subJ := filtered[j].NextDeliveryDate
				if subI == "" && subJ == "" {
					return false
				}
				if subI == "" {
					return false
				}
				if subJ == "" {
					return true
				}
				return subI < subJ
			}
			if dateI == "" {
				return false
			}
			if dateJ == "" {
				return true
			}
			return dateI < dateJ
		})
	}

	return filtered
}

func (m ShopModel) Init() tea.Cmd {
	return tea.Batch(
		func() tea.Msg {
			items, shopping, err := m.client.FetchData(context.Background())
			if err != nil {
				return errMsg{err}
			}
			return dataLoadedMsg{items: items, shopping: shopping}
		},
		textinput.Blink,
	)
}

func (m ShopModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

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

		m.items = applyInventoryFilters(m.allItems, m.categories[m.categoryIndex], m.expireFilter, time.Now())
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
			for i := range m.allItems {
				if m.allItems[i].ID == msg.id {
					m.allItems[i].Stock++
					break
				}
			}
		}
		return m, nil

	case stockUpdatedMsg:
		return m, nil

	case expirationUpdatedMsg:
		for i := range m.allItems {
			if m.allItems[i].ID == msg.id {
				m.allItems[i].ExpirationDate = msg.expirationDate
				break
			}
		}
		m.items = applyInventoryFilters(m.allItems, m.categories[m.categoryIndex], m.expireFilter, time.Now())
		return m, nil

	case deliveryDateUpdatedMsg:
		for i := range m.allItems {
			if m.allItems[i].ID == msg.id {
				m.allItems[i].NextDeliveryDate = msg.nextDeliveryDate
				break
			}
		}
		m.items = applyInventoryFilters(m.allItems, m.categories[m.categoryIndex], m.expireFilter, time.Now())
		return m, nil

	case subscriptionUpdatedMsg:
		for i := range m.allItems {
			if m.allItems[i].ID == msg.id {
				m.allItems[i].IsSubscription = msg.isSubscription
				m.allItems[i].SubCycle = msg.subCycle
				m.allItems[i].NextDeliveryDate = msg.nextDeliveryDate
				break
			}
		}
		m.items = applyInventoryFilters(m.allItems, m.categories[m.categoryIndex], m.expireFilter, time.Now())
		return m, nil

	case inventoryItemAddedMsg:
		m.allItems = append(m.allItems, msg.item)

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

		m.items = applyInventoryFilters(m.allItems, m.categories[m.categoryIndex], m.expireFilter, time.Now())
		for i, it := range m.items {
			if it.ID == msg.item.ID {
				m.leftCursor = i
				break
			}
		}
		return m, nil

	case errMsg:
		m.err = msg.err
		m.loading = false
		return m, nil

	case tea.KeyMsg:
		if m.state == shopStateInputExpire {
			switch msg.Type {
			case tea.KeyEsc:
				m.state = shopStateBrowse
				m.err = nil
				m.textInput.Blur()
				return m, nil

			case tea.KeyEnter:
				val := strings.TrimSpace(m.textInput.Value())
				if val != "" {
					if _, err := time.Parse("2006-01-02", val); err != nil {
						m.err = fmt.Errorf("日付形式は YYYY-MM-DD で入力してください (例: 2026-09-30)")
						return m, nil
					}
				}
				m.err = nil
				m.state = shopStateBrowse
				m.textInput.Blur()

				if len(m.items) == 0 {
					return m, nil
				}
				item := m.items[m.leftCursor]

				// ローカルキャッシュを即座に更新
				for i := range m.allItems {
					if m.allItems[i].ID == item.ID {
						m.allItems[i].ExpirationDate = val
						break
					}
				}
				m.items = applyInventoryFilters(m.allItems, m.categories[m.categoryIndex], m.expireFilter, time.Now())

				return m, func() tea.Msg {
					err := m.client.UpdateExpirationDate(context.Background(), item.ID, val)
					if err != nil {
						return errMsg{err}
					}
					return expirationUpdatedMsg{id: item.ID, expirationDate: val}
				}

			default:
				m.textInput, cmd = m.textInput.Update(msg)
				return m, cmd
			}
		}

		if m.state == shopStateInputDeliveryDate {
			switch msg.Type {
			case tea.KeyEsc:
				m.state = shopStateBrowse
				m.err = nil
				m.textInput.Blur()
				return m, nil

			case tea.KeyEnter:
				val := strings.TrimSpace(m.textInput.Value())
				if val != "" {
					if _, err := time.Parse("2006-01-02", val); err != nil {
						m.err = fmt.Errorf("日付形式は YYYY-MM-DD で入力してください (例: 2026-09-30)")
						return m, nil
					}
				}
				m.err = nil
				m.state = shopStateBrowse
				m.textInput.Blur()

				if len(m.items) == 0 {
					return m, nil
				}
				item := m.items[m.leftCursor]

				isSub := item.IsSubscription
				cycle := item.SubCycle
				if val != "" {
					isSub = true
					if cycle == "" {
						cycle = "1ヶ月"
					}
				} else {
					isSub = false
					cycle = ""
				}

				for i := range m.allItems {
					if m.allItems[i].ID == item.ID {
						m.allItems[i].NextDeliveryDate = val
						m.allItems[i].IsSubscription = isSub
						m.allItems[i].SubCycle = cycle
						break
					}
				}
				m.items = applyInventoryFilters(m.allItems, m.categories[m.categoryIndex], m.expireFilter, time.Now())

				return m, func() tea.Msg {
					err := m.client.UpdateSubscription(context.Background(), item.ID, isSub, cycle, val)
					if err != nil {
						return errMsg{err}
					}
					return subscriptionUpdatedMsg{
						id:               item.ID,
						isSubscription:   isSub,
						subCycle:         cycle,
						nextDeliveryDate: val,
					}
				}

			default:
				m.textInput, cmd = m.textInput.Update(msg)
				return m, cmd
			}
		}

		if m.state == shopStateSelectSubCycle {
			switch msg.Type {
			case tea.KeyEsc:
				m.state = shopStateBrowse
				m.err = nil
				return m, nil

			case tea.KeyRunes:
				if len(m.items) == 0 {
					m.state = shopStateBrowse
					return m, nil
				}
				item := m.items[m.leftCursor]
				r := msg.Runes[0]

				if r >= '1' && r <= '6' {
					cycle := fmt.Sprintf("%cヶ月", r)
					isSub := true
					nextDeliveryDate := item.NextDeliveryDate
					if nextDeliveryDate == "" {
						nextDeliveryDate = domain.AdvanceDeliveryDate("", cycle)
					}
					m.state = shopStateBrowse

					for i := range m.allItems {
						if m.allItems[i].ID == item.ID {
							m.allItems[i].IsSubscription = isSub
							m.allItems[i].SubCycle = cycle
							m.allItems[i].NextDeliveryDate = nextDeliveryDate
							break
						}
					}
					m.items = applyInventoryFilters(m.allItems, m.categories[m.categoryIndex], m.expireFilter, time.Now())

					return m, func() tea.Msg {
						err := m.client.UpdateSubscription(context.Background(), item.ID, isSub, cycle, nextDeliveryDate)
						if err != nil {
							return errMsg{err}
						}
						return subscriptionUpdatedMsg{
							id:               item.ID,
							isSubscription:   isSub,
							subCycle:         cycle,
							nextDeliveryDate: nextDeliveryDate,
						}
					}
				} else if r == '0' {
					m.state = shopStateBrowse
					isSub := false
					cycle := ""
					nextDeliveryDate := ""

					for i := range m.allItems {
						if m.allItems[i].ID == item.ID {
							m.allItems[i].IsSubscription = isSub
							m.allItems[i].SubCycle = cycle
							m.allItems[i].NextDeliveryDate = nextDeliveryDate
							break
						}
					}
					m.items = applyInventoryFilters(m.allItems, m.categories[m.categoryIndex], m.expireFilter, time.Now())

					return m, func() tea.Msg {
						err := m.client.UpdateSubscription(context.Background(), item.ID, false, "", "")
						if err != nil {
							return errMsg{err}
						}
						return subscriptionUpdatedMsg{
							id:               item.ID,
							isSubscription:   isSub,
							subCycle:         cycle,
							nextDeliveryDate: nextDeliveryDate,
						}
					}
				}
			}
			return m, nil
		}

		if m.state == shopStateInputNewInventory {
			switch msg.Type {
			case tea.KeyEsc:
				m.state = shopStateBrowse
				m.err = nil
				m.textInput.Blur()
				return m, nil

			case tea.KeyEnter:
				name := strings.TrimSpace(m.textInput.Value())
				m.state = shopStateBrowse
				m.textInput.Blur()
				if name == "" {
					return m, nil
				}

				var categories []string
				if m.categoryIndex > 0 && m.categoryIndex < len(m.categories) {
					categories = []string{m.categories[m.categoryIndex]}
				}

				return m, func() tea.Msg {
					added, err := m.client.AddInventoryItem(context.Background(), name, 1, categories)
					if err != nil {
						return errMsg{err}
					}
					return inventoryItemAddedMsg{item: added}
				}

			default:
				m.textInput, cmd = m.textInput.Update(msg)
				return m, cmd
			}
		}

		if m.state == shopStateInputNewShopping {
			switch msg.Type {
			case tea.KeyEsc:
				m.state = shopStateBrowse
				m.err = nil
				m.textInput.Blur()
				return m, nil

			case tea.KeyEnter:
				name := strings.TrimSpace(m.textInput.Value())
				m.state = shopStateBrowse
				m.textInput.Blur()
				if name == "" {
					return m, nil
				}

				return m, func() tea.Msg {
					added, err := m.client.AddShoppingItem(context.Background(), name)
					if err != nil {
						return errMsg{err}
					}
					return itemAddedMsg{item: added}
				}

			default:
				m.textInput, cmd = m.textInput.Update(msg)
				return m, cmd
			}
		}

		// shopStateBrowse モード
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit

		case "a":
			if m.activePane == 0 {
				m.state = shopStateInputNewInventory
				catHint := ""
				if m.categoryIndex > 0 && m.categoryIndex < len(m.categories) {
					catHint = fmt.Sprintf(" [%s]", m.categories[m.categoryIndex])
				}
				m.textInput.Placeholder = "新しい管理物名を入力..." + catHint
				m.textInput.SetValue("")
				m.textInput.Focus()
				return m, textinput.Blink
			} else if m.activePane == 1 {
				m.state = shopStateInputNewShopping
				m.textInput.Placeholder = "買い物リストに追加するアイテム名..."
				m.textInput.SetValue("")
				m.textInput.Focus()
				return m, textinput.Blink
			}

		case "tab", "right", "left":
			if m.activePane == 0 {
				m.activePane = 1
			} else {
				m.activePane = 0
			}

		case "/":
			if m.activePane == 0 && len(m.categories) > 0 {
				m.categoryIndex = (m.categoryIndex + 1) % len(m.categories)
				m.items = applyInventoryFilters(m.allItems, m.categories[m.categoryIndex], m.expireFilter, time.Now())
				m.leftCursor = 0
			}

		case "f":
			if m.activePane == 0 {
				m.expireFilter = (m.expireFilter + 1) % 3
				m.items = applyInventoryFilters(m.allItems, m.categories[m.categoryIndex], m.expireFilter, time.Now())
				m.leftCursor = 0
			}

		case "e", "d":
			if m.activePane == 0 && len(m.items) > 0 {
				m.state = shopStateInputExpire
				m.textInput.Placeholder = "YYYY-MM-DD (賞味期限)"
				m.textInput.SetValue(m.items[m.leftCursor].ExpirationDate)
				m.textInput.Focus()
				return m, textinput.Blink
			}

		case "c":
			if m.activePane == 0 && len(m.items) > 0 {
				m.state = shopStateSelectSubCycle
				m.err = nil
				return m, nil
			}

		case "p":
			if m.activePane == 0 && len(m.items) > 0 {
				m.state = shopStateInputDeliveryDate
				m.textInput.Placeholder = "YYYY-MM-DD (次回配送日)"
				m.textInput.SetValue(m.items[m.leftCursor].NextDeliveryDate)
				m.textInput.Focus()
				return m, textinput.Blink
			}

		case "S": // Shift+S: 定期便スキップ (次回配送日を +1サイクル延長)
			if m.activePane == 0 && len(m.items) > 0 {
				item := m.items[m.leftCursor]
				if item.IsSubscription && item.NextDeliveryDate != "" {
					nextDate := domain.AdvanceDeliveryDate(item.NextDeliveryDate, item.SubCycle)
					for i := range m.allItems {
						if m.allItems[i].ID == item.ID {
							m.allItems[i].NextDeliveryDate = nextDate
							break
						}
					}
					m.items = applyInventoryFilters(m.allItems, m.categories[m.categoryIndex], m.expireFilter, time.Now())

					return m, func() tea.Msg {
						err := m.client.UpdateNextDeliveryDate(context.Background(), item.ID, nextDate)
						if err != nil {
							return errMsg{err}
						}
						return deliveryDateUpdatedMsg{id: item.ID, nextDeliveryDate: nextDate}
					}
				}
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
				item := m.items[m.leftCursor]

				var cmds []tea.Cmd
				for i := range m.allItems {
					if m.allItems[i].ID == id {
						m.allItems[i].Stock = newStock
						break
					}
				}
				cmds = append(cmds, func() tea.Msg {
					err := m.client.UpdateStock(context.Background(), id, newStock)
					if err != nil {
						return errMsg{err}
					}
					return stockUpdatedMsg{}
				})

				// 定期便アイテムで、次回配送日を既に過ぎていた場合は受取とみなして次回日を1サイクル進める
				subInfo := item.GetSubscriptionInfo(time.Now())
				if item.IsSubscription && item.NextDeliveryDate != "" && subInfo.Status == domain.SubscriptionStatusOverdue {
					nextDate := domain.AdvanceDeliveryDate(item.NextDeliveryDate, item.SubCycle)
					for i := range m.allItems {
						if m.allItems[i].ID == id {
							m.allItems[i].NextDeliveryDate = nextDate
							break
						}
					}
					m.items[m.leftCursor].NextDeliveryDate = nextDate
					cmds = append(cmds, func() tea.Msg {
						err := m.client.UpdateNextDeliveryDate(context.Background(), id, nextDate)
						if err != nil {
							return errMsg{err}
						}
						return deliveryDateUpdatedMsg{id: id, nextDeliveryDate: nextDate}
					})
				}

				return m, tea.Batch(cmds...)
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
					resetExpiration := false

					// 在庫が0になった場合、使い切ったため賞味期限を自動リセット
					if newStock == 0 && item.ExpirationDate != "" {
						resetExpiration = true
						m.items[m.leftCursor].ExpirationDate = ""
					}

					for i := range m.allItems {
						if m.allItems[i].ID == item.ID {
							m.allItems[i].Stock = newStock
							if resetExpiration {
								m.allItems[i].ExpirationDate = ""
							}
							break
						}
					}

					cmds = append(cmds, func() tea.Msg {
						err := m.client.UpdateStock(context.Background(), item.ID, newStock)
						if err != nil {
							return errMsg{err}
						}
						return stockUpdatedMsg{}
					})

					if resetExpiration {
						cmds = append(cmds, func() tea.Msg {
							err := m.client.UpdateExpirationDate(context.Background(), item.ID, "")
							if err != nil {
								return errMsg{err}
							}
							return expirationUpdatedMsg{id: item.ID, expirationDate: ""}
						})
					}
				} else if item.Stock == 0 && item.ExpirationDate != "" {
					// 既に在庫0でも期限が残っている場合はリセット
					m.items[m.leftCursor].ExpirationDate = ""
					for i := range m.allItems {
						if m.allItems[i].ID == item.ID {
							m.allItems[i].ExpirationDate = ""
							break
						}
					}
					cmds = append(cmds, func() tea.Msg {
						err := m.client.UpdateExpirationDate(context.Background(), item.ID, "")
						if err != nil {
							return errMsg{err}
						}
						return expirationUpdatedMsg{id: item.ID, expirationDate: ""}
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
	if m.loading {
		return "\n  🔄 Notionと同期中...\n"
	}

	now := time.Now()

	// 賞味期限 & 定期便アラート集計（全体データから算出）
	var expiredCount, soonCount, subAlertCount int
	for _, it := range m.allItems {
		expInfo := it.GetExpirationInfo(now)
		if expInfo.Status == domain.ExpirationStatusExpired {
			expiredCount++
		} else if expInfo.Status == domain.ExpirationStatusExpiringSoon {
			soonCount++
		}

		subInfo := it.GetSubscriptionInfo(now)
		if subInfo.Status == domain.SubscriptionStatusAlert {
			subAlertCount++
		}
	}

	titleStyle := lipgloss.NewStyle().Bold(true).MarginBottom(1)
	activeBorder := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("205")).Padding(1, 2).Width(58)
	inactiveBorder := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(1, 2).Width(58)
	rightActiveBorder := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("43")).Padding(1, 2).Width(35)
	rightInactiveBorder := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(1, 2).Width(35)
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	warningStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)

	// アラートバナー
	alertBanner := ""
	if expiredCount > 0 || soonCount > 0 || subAlertCount > 0 {
		var parts []string
		if expiredCount > 0 {
			parts = append(parts, lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true).Render(fmt.Sprintf("🚨 期限切れ: %d件", expiredCount)))
		}
		if soonCount > 0 {
			parts = append(parts, lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true).Render(fmt.Sprintf("⏳ まもなく期限: %d件", soonCount)))
		}
		if subAlertCount > 0 {
			parts = append(parts, lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Bold(true).Render(fmt.Sprintf("🚚 定期便スキップ検討: %d件", subAlertCount)))
		}
		bannerText := "【アラート】 " + strings.Join(parts, "  |  ")
		alertBanner = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("208")).
			Padding(0, 1).
			Render(bannerText) + "\n\n"
	}

	leftStyle := inactiveBorder
	if m.activePane == 0 {
		leftStyle = activeBorder
	}

	s := titleStyle.Foreground(lipgloss.Color("205")).Render("📦 在庫マスター") + "\n"

	// 分類タブ
	tabStr := ""
	for i, cat := range m.categories {
		if i == m.categoryIndex {
			tabStr += lipgloss.NewStyle().Foreground(lipgloss.Color("43")).Bold(true).Render("["+cat+"]") + " "
		} else {
			tabStr += lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(cat) + " "
		}
	}
	s += tabStr + "\n"

	// フィルター状態
	filterLabel := ""
	switch m.expireFilter {
	case expireFilterSortSoon:
		filterLabel = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("⚡ 期限順ソート中")
	case expireFilterAlertOnly:
		filterLabel = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true).Render("🚨 期限・定期便アラートのみ表示中")
	}
	if filterLabel != "" {
		s += filterLabel + "\n"
	}
	s += "\n"

	if len(m.items) == 0 {
		s += lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("該当する食品・在庫はありません\n")
	}

	for i, item := range m.items {
		cursor := "  "
		stockStr := fmt.Sprintf("[%d] %s", item.Stock, item.Name)
		if item.Stock <= 0 {
			stockStr = warningStyle.Render(stockStr + " ⚠️空")
		}

		// 賞味期限ラベルの生成
		expLabel := ""
		info := item.GetExpirationInfo(now)
		switch info.Status {
		case domain.ExpirationStatusExpired:
			if info.Days <= -365 {
				years := (-info.Days) / 365
				expLabel = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true).Render(fmt.Sprintf(" 🔴期限切(%d年前:%s)", years, item.ExpirationDate))
			} else if info.Days <= -30 {
				months := (-info.Days) / 30
				expLabel = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true).Render(fmt.Sprintf(" 🔴期限切(%dヶ月前:%s)", months, item.ExpirationDate))
			} else {
				expLabel = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true).Render(fmt.Sprintf(" 🔴期限切(%d日前:%s)", -info.Days, item.ExpirationDate))
			}
		case domain.ExpirationStatusExpiringSoon:
			if info.Days == 0 {
				expLabel = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true).Render(" 🟡本日期限!")
			} else {
				expLabel = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true).Render(fmt.Sprintf(" 🟡残%d日(%s)", info.Days, item.ExpirationDate))
			}
		case domain.ExpirationStatusSafe:
			expLabel = lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Render(fmt.Sprintf(" (%s)", item.ExpirationDate))
		}

		// 定期便ラベルの生成
		subLabel := ""
		subInfo := item.GetSubscriptionInfo(now)
		cycleTag := ""
		if item.SubCycle != "" {
			cycleTag = "[" + item.SubCycle + "]"
		}
		switch subInfo.Status {
		case domain.SubscriptionStatusAlert:
			subLabel = lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Bold(true).Render(fmt.Sprintf(" 🚚%s届く%s⚠️(残%d日/在庫%d)[S]スキップ", item.NextDeliveryDate, cycleTag, subInfo.Days, item.Stock))
		case domain.SubscriptionStatusDue:
			subLabel = lipgloss.NewStyle().Foreground(lipgloss.Color("45")).Render(fmt.Sprintf(" 🚚%s届く%s(残%d日)", item.NextDeliveryDate, cycleTag, subInfo.Days))
		case domain.SubscriptionStatusOverdue:
			subLabel = lipgloss.NewStyle().Foreground(lipgloss.Color("135")).Render(fmt.Sprintf(" 🚚%s届いたはず%s(受取確認)", item.NextDeliveryDate, cycleTag))
		case domain.SubscriptionStatusNormal:
			subLabel = lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Render(fmt.Sprintf(" 🚚定期%s(%s)", cycleTag, item.NextDeliveryDate))
		}

		line := stockStr + expLabel + subLabel
		if m.activePane == 0 && m.leftCursor == i {
			cursor = "> "
			s += selectedStyle.Render(cursor) + line + "\n"
		} else {
			s += cursor + line + "\n"
		}
	}

	// 入力モード時のプロンプト
	if m.state == shopStateInputExpire {
		s += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true).Render("📅 賞味期限を入力 (YYYY-MM-DD / 空で解除):") + "\n"
		s += m.textInput.View() + "\n"
		s += lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("Enter: 保存  Esc: キャンセル\n")
	} else if m.state == shopStateInputDeliveryDate {
		s += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("45")).Bold(true).Render("🚚 次回配送日を入力 (YYYY-MM-DD / 空で解除):") + "\n"
		s += m.textInput.View() + "\n"
		s += lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("Enter: 保存  Esc: キャンセル\n")
	} else if m.state == shopStateSelectSubCycle {
		s += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("45")).Bold(true).Render("🚚 配送期間(サイクル)を選択:") + "\n"
		s += lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Render(" [1] 1ヶ月  [2] 2ヶ月  [3] 3ヶ月  [4] 4ヶ月  [5] 5ヶ月  [6] 6ヶ月  [0] 定期便解除") + "\n"
		s += lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(" Esc: キャンセル\n")
	} else if m.state == shopStateInputNewInventory {
		catHint := "全て"
		if m.categoryIndex > 0 && m.categoryIndex < len(m.categories) {
			catHint = m.categories[m.categoryIndex]
		}
		s += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("43")).Bold(true).Render(fmt.Sprintf("✨ 新規管理物を登録 (分類: %s / 初期在庫: 1):", catHint)) + "\n"
		s += m.textInput.View() + "\n"
		s += lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("Enter: 登録  Esc: キャンセル\n")
	}

	leftPane := leftStyle.Render(s)

	rightStyle := rightInactiveBorder
	if m.activePane == 1 {
		rightStyle = rightActiveBorder
	}

	rightContent := titleStyle.Foreground(lipgloss.Color("43")).Render("🛒 買い物リスト") + "\n\n"
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
	if m.state == shopStateInputNewShopping {
		rightContent += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("43")).Bold(true).Render("🛒 買い物リストへ追加:") + "\n"
		rightContent += m.textInput.View() + "\n"
		rightContent += lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("Enter: 追加  Esc: キャンセル\n")
	}
	rightPane := rightStyle.Render(rightContent)

	ui := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, "   ", rightPane)

	errorMsg := ""
	if m.err != nil {
		errorMsg = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true).Render(fmt.Sprintf("❌ エラー: %v\n", m.err))
	}

	footer := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).MarginTop(1).Render(
		"Tab: 切替 | a: 追加 | c: 配送期間 | p: 配送日 | S: スキップ | e: 期限 | /: 分類 | f: 絞込 | +/-: 在庫 | q: 終了",
	)

	return "\n" + alertBanner + errorMsg + ui + "\n" + footer + "\n"
}
