package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jomei/notionapi"
	"github.com/ookawayuuto/notion-tui/internal/domain"
)

type MonstClientInterface interface {
	FetchWakuwaku(ctx context.Context) ([]domain.Wakuwaku, map[notionapi.ObjectID]domain.Wakuwaku, error)
	FetchMonsters(ctx context.Context, attribute string) ([]domain.Monster, error)
	UpdateMonsterRelations(ctx context.Context, monsterID notionapi.ObjectID, accountKey string, wakuwakuIDs []notionapi.ObjectID) error
}

type MonstState int

const (
	StateBrowse MonstState = iota
	StateSelectAccount
	StateEditWakuwaku
	StateFilterMenu
)

type MonstModel struct {
	client MonstClientInterface

	// Shared data
	wakuwakuList []domain.Wakuwaku
	wakuwakuDict map[notionapi.ObjectID]domain.Wakuwaku
	allMonsters  []domain.Monster
	monsters     []domain.Monster

	// UI State
	state   MonstState
	loading bool
	err     error

	// StateBrowse
	attrs        []string
	attrIndex    int
	monsterIndex int
	listOffset   int

	// StateSelectAccount
	accounts     []string
	accountIndex int

	// StateEditWakuwaku
	wakuwakuCursor int
	wakuOffset     int
	selectedWaku   map[notionapi.ObjectID]bool

	// StateFilterMenu
	events         []string
	eventIndex     int
	sortOrders     []string
	sortOrderIndex int
	filterCursor   int
}

type wakuwakuLoadedMsg struct {
	list []domain.Wakuwaku
	dict map[notionapi.ObjectID]domain.Wakuwaku
}
type monstersLoadedMsg struct{ monsters []domain.Monster }
type relationUpdatedMsg struct{}

func NewMonstModel(client MonstClientInterface) MonstModel {
	return MonstModel{
		client:     client,
		loading:    true,
		attrs:      []string{"全て", "火", "水", "木", "光", "闇"},
		accounts:   []string{"アカウントA", "アカウントB", "アカウントC", "アカウントD", "アカウントA-2", "アカウントB-2"},
		events:     []string{"全て"},
		sortOrders: []string{"優先度順", "属性順", "五十音順"},
	}
}

func (m MonstModel) Init() tea.Cmd {
	return func() tea.Msg {
		list, dict, err := m.client.FetchWakuwaku(context.Background())
		if err != nil {
			return errMsg{err}
		}
		return wakuwakuLoadedMsg{list: list, dict: dict}
	}
}

func (m MonstModel) fetchMonstersCmd() tea.Cmd {
	return func() tea.Msg {
		// "" を渡して「全て」のモンスターを一度に取得します
		monsters, err := m.client.FetchMonsters(context.Background(), "")
		if err != nil {
			return errMsg{err}
		}
		return monstersLoadedMsg{monsters: monsters}
	}
}

func filterMonsters(all []domain.Monster, attr string, event string, sortOrder string) []domain.Monster {
	var filtered []domain.Monster
	for _, mon := range all {
		if (attr == "全て" || mon.Attribute == attr) &&
			(event == "全て" || mon.Event == event) {
			filtered = append(filtered, mon)
		}
	}

	sort.SliceStable(filtered, func(i, j int) bool {
		switch sortOrder {
		case "優先度順":
			return priorityScore(filtered[i].Priority) > priorityScore(filtered[j].Priority)
		case "属性順":
			return filtered[i].Attribute < filtered[j].Attribute
		case "五十音順":
			return filtered[i].Name < filtered[j].Name
		}
		return false
	})
	return filtered
}

func priorityScore(p string) int {
	switch p {
	case "S":
		return 4
	case "A":
		return 3
	case "B":
		return 2
	case "C":
		return 1
	}
	return 0
}

func (m MonstModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case wakuwakuLoadedMsg:
		m.wakuwakuList = msg.list
		m.wakuwakuDict = msg.dict
		return m, m.fetchMonstersCmd()

	case monstersLoadedMsg:
		m.allMonsters = msg.monsters
		eventMap := make(map[string]bool)
		m.events = []string{"全て"}
		for _, mon := range m.allMonsters {
			if mon.Event != "" && !eventMap[mon.Event] {
				eventMap[mon.Event] = true
				m.events = append(m.events, mon.Event)
			}
		}
		m.monsters = filterMonsters(m.allMonsters, m.attrs[m.attrIndex], m.events[m.eventIndex], m.sortOrders[m.sortOrderIndex])
		if m.monsterIndex >= len(m.monsters) {
			m.monsterIndex = 0
			m.listOffset = 0
		}
		m.loading = false
		return m, nil

	case relationUpdatedMsg:
		m.loading = true
		m.state = StateBrowse
		return m, m.fetchMonstersCmd()

	case errMsg:
		m.err = msg.err
		m.loading = false
		return m, nil

	case tea.KeyMsg:
		if m.loading {
			return m, nil // ローディング中は操作ブロック
		}

		switch msg.String() {
		case "ctrl+c", "q":
			if m.state == StateBrowse {
				return m, tea.Quit
			}
		case "esc":
			if m.state == StateSelectAccount {
				m.state = StateBrowse
			} else if m.state == StateEditWakuwaku {
				m.state = StateSelectAccount
			} else if m.state == StateFilterMenu {
				m.state = StateBrowse
			}
			return m, nil
		}

		switch m.state {
		case StateBrowse:
			switch msg.String() {
			case "f":
				m.state = StateFilterMenu
				m.filterCursor = 0
			case "left", "h":
				if m.attrIndex > 0 {
					m.attrIndex--
					m.monsters = filterMonsters(m.allMonsters, m.attrs[m.attrIndex], m.events[m.eventIndex], m.sortOrders[m.sortOrderIndex])
					m.monsterIndex = 0
					m.listOffset = 0
					return m, nil
				}
			case "right", "l":
				if m.attrIndex < len(m.attrs)-1 {
					m.attrIndex++
					m.monsters = filterMonsters(m.allMonsters, m.attrs[m.attrIndex], m.events[m.eventIndex], m.sortOrders[m.sortOrderIndex])
					m.monsterIndex = 0
					m.listOffset = 0
					return m, nil
				}
			case "up", "k":
				if m.monsterIndex > 0 {
					m.monsterIndex--
					if m.monsterIndex < m.listOffset {
						m.listOffset--
					}
				}
			case "down", "j":
				if m.monsterIndex < len(m.monsters)-1 {
					m.monsterIndex++
					if m.monsterIndex >= m.listOffset+10 {
						m.listOffset++
					}
				}
			case "enter":
				if len(m.monsters) > 0 {
					m.state = StateSelectAccount
					m.accountIndex = 0
				}
			}

		case StateFilterMenu:
			switch msg.String() {
			case "up", "k":
				if m.filterCursor > 0 {
					m.filterCursor--
				}
			case "down", "j":
				if m.filterCursor < 2 {
					m.filterCursor++
				}
			case "left", "h":
				switch m.filterCursor {
				case 0:
					if m.attrIndex > 0 {
						m.attrIndex--
					}
				case 1:
					if m.eventIndex > 0 {
						m.eventIndex--
					}
				case 2:
					if m.sortOrderIndex > 0 {
						m.sortOrderIndex--
					}
				}
				m.monsters = filterMonsters(m.allMonsters, m.attrs[m.attrIndex], m.events[m.eventIndex], m.sortOrders[m.sortOrderIndex])
				m.monsterIndex = 0
				m.listOffset = 0
			case "right", "l":
				switch m.filterCursor {
				case 0:
					if m.attrIndex < len(m.attrs)-1 {
						m.attrIndex++
					}
				case 1:
					if m.eventIndex < len(m.events)-1 {
						m.eventIndex++
					}
				case 2:
					if m.sortOrderIndex < len(m.sortOrders)-1 {
						m.sortOrderIndex++
					}
				}
				m.monsters = filterMonsters(m.allMonsters, m.attrs[m.attrIndex], m.events[m.eventIndex], m.sortOrders[m.sortOrderIndex])
				m.monsterIndex = 0
				m.listOffset = 0
			}

		case StateSelectAccount:
			switch msg.String() {
			case "up", "k":
				if m.accountIndex > 0 {
					m.accountIndex--
				}
			case "down", "j":
				if m.accountIndex < len(m.accounts)-1 {
					m.accountIndex++
				}
			case "enter":
				// 現在のモンスターの、選択されたアカウントの実を取得してチェックを入れる
				m.state = StateEditWakuwaku
				m.wakuwakuCursor = 0
				m.wakuOffset = 0
				m.selectedWaku = make(map[notionapi.ObjectID]bool)

				monster := m.monsters[m.monsterIndex]
				var currentIds []notionapi.ObjectID
				switch m.accountIndex {
				case 0:
					currentIds = monster.AccountA
				case 1:
					currentIds = monster.AccountB
				case 2:
					currentIds = monster.AccountC
				case 3:
					currentIds = monster.AccountD
				case 4:
					currentIds = monster.AccountA2
				case 5:
					currentIds = monster.AccountB2
				}
				for _, id := range currentIds {
					m.selectedWaku[id] = true
				}
			}

		case StateEditWakuwaku:
			switch msg.String() {
			case "up", "k":
				if m.wakuwakuCursor > 0 {
					m.wakuwakuCursor--
					if m.wakuwakuCursor < m.wakuOffset {
						m.wakuOffset--
					}
				}
			case "down", "j":
				if m.wakuwakuCursor < len(m.wakuwakuList)-1 {
					m.wakuwakuCursor++
					if m.wakuwakuCursor >= m.wakuOffset+10 {
						m.wakuOffset++
					}
				}
			case " ", "enter": // Space or Enter for toggle
				id := m.wakuwakuList[m.wakuwakuCursor].ID
				m.selectedWaku[id] = !m.selectedWaku[id]
			case "s", "S":
				// Save changes
				m.loading = true
				monsterID := m.monsters[m.monsterIndex].ID
				accountKey := m.accounts[m.accountIndex]
				
				var mishojiID notionapi.ObjectID
				for _, w := range m.wakuwakuList {
					if w.Name == "未所持" {
						mishojiID = w.ID
						break
					}
				}

				hasOther := false
				for id, selected := range m.selectedWaku {
					if selected && id != mishojiID {
						hasOther = true
					}
				}

				var newIDs []notionapi.ObjectID
				for id, selected := range m.selectedWaku {
					if selected {
						if id == mishojiID && hasOther {
							continue
						}
						newIDs = append(newIDs, id)
					}
				}
				return m, func() tea.Msg {
					err := m.client.UpdateMonsterRelations(context.Background(), monsterID, accountKey, newIDs)
					if err != nil {
						return errMsg{err}
					}
					return relationUpdatedMsg{}
				}
			}
		}
	}
	return m, nil
}

func (m MonstModel) View() string {
	if m.err != nil {
		return fmt.Sprintf("\n  ❌ エラー: %v\n\n  q/esc で戻ります\n", m.err)
	}
	if m.loading {
		return "\n  🔄 Notionと同期中...\n"
	}

	// 共通スタイル
	titleStyle := lipgloss.NewStyle().Bold(true).MarginBottom(1)
	activeBorder := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("205")).Padding(1, 1).Width(35).Height(15)
	inactiveBorder := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(1, 1).Width(35).Height(15)
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	attrStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("43")).Bold(true).Padding(0, 1)
	inactiveAttrStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Padding(0, 1)

	// トップバー（属性選択）
	topBar := ""
	for i, attr := range m.attrs {
		if i == m.attrIndex {
			topBar += attrStyle.Render("["+attr+"]") + " "
		} else {
			topBar += inactiveAttrStyle.Render(attr) + " "
		}
	}
	topBar = "\n  " + topBar + "\n\n"

	// 左ペイン（モンスター一覧）
	leftStyle := activeBorder
	if m.state != StateBrowse {
		leftStyle = inactiveBorder
	}
	leftContent := titleStyle.Foreground(lipgloss.Color("205")).Render("📦 モンスター一覧") + "\n"
	if len(m.monsters) == 0 {
		leftContent += lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("データがありません")
	} else {
		for i := m.listOffset; i < m.listOffset+10 && i < len(m.monsters); i++ {
			monster := m.monsters[i]
			cursor := "  "
			status := "[未]"
			if monster.Has {
				status = "[済]"
			}
			line := fmt.Sprintf("%s %s", status, monster.Name)
			// 名前が長すぎる場合は切り詰める処理を後で入れるとより綺麗です

			if m.monsterIndex == i {
				leftContent += selectedStyle.Render("> "+line) + "\n"
			} else {
				leftContent += cursor + line + "\n"
			}
		}
	}
	leftPane := leftStyle.Render(leftContent)

	// 右ペイン（詳細・アカウント選択・わくわく選択）
	rightStyle := activeBorder
	if m.state != StateBrowse {
		rightStyle = activeBorder // Edit系は右ペインがアクティブ
	}

	rightContent := ""
	if len(m.monsters) > 0 {
		monster := m.monsters[m.monsterIndex]
		rightContent += titleStyle.Foreground(lipgloss.Color("43")).Render("🍎 【"+monster.Name+"】") + "\n"
		rightContent += fmt.Sprintf("属性: %s  /  優先度: %s\n\n", monster.Attribute, monster.Priority)

		if m.state == StateFilterMenu {
			rightContent += lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Render("🔍 フィルター＆ソート設定") + "\n\n"
			
			renderOption := func(label string, options []string, index int, isActive bool) string {
				optStr := options[index]
				if index > 0 { optStr = "◁ " + optStr } else { optStr = "  " + optStr }
				if index < len(options)-1 { optStr += " ▷" } else { optStr += "  " }
				
				line := fmt.Sprintf("%-6s: %s", label, optStr)
				if isActive {
					return selectedStyle.Render("> " + line) + "\n"
				}
				return "  " + line + "\n"
			}
			
			rightContent += renderOption("属性", m.attrs, m.attrIndex, m.filterCursor == 0)
			rightContent += renderOption("イベント", m.events, m.eventIndex, m.filterCursor == 1)
			rightContent += renderOption("ソート順", m.sortOrders, m.sortOrderIndex, m.filterCursor == 2)
		} else if m.state == StateEditWakuwaku {
			// わくわくの実ピッカー
			rightContent += lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Render("📝 "+m.accounts[m.accountIndex]+" の実を編集") + "\n"
			for i := m.wakuOffset; i < m.wakuOffset+10 && i < len(m.wakuwakuList); i++ {
				w := m.wakuwakuList[i]
				cursor := "  "
				check := "[ ]"
				if m.selectedWaku[w.ID] {
					check = "[x]"
				}

				if m.wakuwakuCursor == i {
					cursor = "> "
					rightContent += selectedStyle.Render(fmt.Sprintf("%s%s %s", cursor, check, w.Name)) + "\n"
				} else {
					rightContent += fmt.Sprintf("%s%s %s\n", cursor, check, w.Name)
				}
			}
		} else {
			// アカウントごとの実の状況一覧
			for i, accName := range m.accounts {
				var ids []notionapi.ObjectID
				switch i {
				case 0:
					ids = monster.AccountA
				case 1:
					ids = monster.AccountB
				case 2:
					ids = monster.AccountC
				case 3:
					ids = monster.AccountD
				case 4:
					ids = monster.AccountA2
				case 5:
					ids = monster.AccountB2
				}

				var wakuNames []string
				for _, id := range ids {
					if w, ok := m.wakuwakuDict[id]; ok {
						wakuNames = append(wakuNames, w.Name)
					}
				}
				wakuStr := "未厳選"
				if len(wakuNames) > 0 {
					wakuStr = strings.Join(wakuNames, ", ")
				}

				accHeader := accName
				if m.state == StateSelectAccount && m.accountIndex == i {
					accHeader = selectedStyle.Render("> " + accName)
				}

				rightContent += fmt.Sprintf("%s\n  - %s\n", accHeader, wakuStr)
			}
		}
	}
	rightPane := rightStyle.Render(rightContent)

	ui := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, "   ", rightPane)

	footerStr := "←/→: 属性切替   ↑/↓: 移動   Enter: 選択/編集   q: 終了"
	if m.state == StateEditWakuwaku {
		footerStr = "↑/↓: 移動   Enter/Space: 選択/解除   s: 保存   Esc: キャンセル"
	} else if m.state == StateSelectAccount {
		footerStr = "↑/↓: アカウント選択   Enter: 実を編集   Esc: 戻る"
	} else if m.state == StateFilterMenu {
		footerStr = "↑/↓: 項目選択   ←/→: 値変更   Esc: 戻る"
	}
	footer := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).MarginTop(1).Render(footerStr)

	return topBar + ui + "\n" + footer + "\n"
}
