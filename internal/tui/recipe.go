package tui

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jomei/notionapi"
	"github.com/ookawayuuto/notion-tui/internal/domain"
)

type RecipeClientInterface interface {
	FetchRecipes(ctx context.Context) ([]domain.RecipeItem, error)
	UpdateRecipe(ctx context.Context, pageID notionapi.ObjectID, categories []string, genre string, difficulty string, cooked *bool) error
	ToggleCooked(ctx context.Context, pageID notionapi.ObjectID, cooked bool) error
}

type RecipeTab int

const (
	TabUnlabeled RecipeTab = iota
	TabAll
	TabUncooked
	TabCooked
)

type RecipeViewMode int

const (
	ViewModeBrowse RecipeViewMode = iota
	ViewModeSearch
	ViewModeEditGenre
	ViewModeEditCategory
)

// Master options
var (
	MasterGenres = []string{
		"和食", "洋食", "中華", "イタリアン", "各国料理", "フレンチ", "スイーツ",
	}
	MasterCategories = []string{
		"ごはんもの", "麺類", "汁もの・スープ", "サラダ", "肉", "魚介類", "卵・大豆", "野菜", "お菓子・デザート",
	}
)

type RecipeModel struct {
	client     RecipeClientInterface
	allRecipes []domain.RecipeItem
	recipes    []domain.RecipeItem

	tab        RecipeTab
	cursor     int
	listOffset int

	mode   RecipeViewMode
	search textinput.Model

	// Label editing state
	editRecipeIdx   int
	selectedGenre   string
	genreCursor     int
	selectedCats    map[string]bool
	catCursor       int

	loading bool
	statusMsg string
	err     error
}

type recipesLoadedMsg struct {
	recipes []domain.RecipeItem
}

type recipeUpdatedMsg struct {
	id         notionapi.ObjectID
	categories []string
	genre      string
	difficulty string
	cooked     *bool
}

type recipeErrMsg struct {
	err error
}

func NewRecipeModel(client RecipeClientInterface) RecipeModel {
	ti := textinput.New()
	ti.Placeholder = "レシピ名、ジャンル、カテゴリーで検索..."
	ti.CharLimit = 50
	ti.Width = 40

	return RecipeModel{
		client:       client,
		tab:          TabUnlabeled,
		mode:         ViewModeBrowse,
		search:       ti,
		loading:      true,
		selectedCats: make(map[string]bool),
	}
}

func (m RecipeModel) Init() tea.Cmd {
	return m.fetchRecipesCmd()
}

func (m RecipeModel) fetchRecipesCmd() tea.Cmd {
	return func() tea.Msg {
		recipes, err := m.client.FetchRecipes(context.Background())
		if err != nil {
			return recipeErrMsg{err: err}
		}
		return recipesLoadedMsg{recipes: recipes}
	}
}

func (m RecipeModel) filterRecipes() []domain.RecipeItem {
	query := strings.ToLower(strings.TrimSpace(m.search.Value()))
	var filtered []domain.RecipeItem

	for _, r := range m.allRecipes {
		// Tab filter
		switch m.tab {
		case TabUnlabeled:
			if !r.IsUnlabeled() {
				continue
			}
		case TabUncooked:
			if r.Cooked {
				continue
			}
		case TabCooked:
			if !r.Cooked {
				continue
			}
		case TabAll:
			// No filter
		}

		// Search filter
		if query != "" {
			match := strings.Contains(strings.ToLower(r.Title), query) ||
				strings.Contains(strings.ToLower(r.Genre), query)
			if !match {
				for _, c := range r.Categories {
					if strings.Contains(strings.ToLower(c), query) {
						match = true
						break
					}
				}
			}
			if !match {
				continue
			}
		}

		filtered = append(filtered, r)
	}

	return filtered
}

func (m *RecipeModel) applyFilter() {
	m.recipes = m.filterRecipes()
	if m.cursor >= len(m.recipes) {
		m.cursor = 0
		m.listOffset = 0
	}
}

func (m RecipeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case recipesLoadedMsg:
		m.allRecipes = msg.recipes
		m.loading = false
		m.applyFilter()
		return m, nil

	case recipeUpdatedMsg:
		// In-memory update
		for i, r := range m.allRecipes {
			if r.ID == msg.id {
				if msg.categories != nil {
					m.allRecipes[i].Categories = msg.categories
				}
				if msg.genre != "" {
					m.allRecipes[i].Genre = msg.genre
				}
				if msg.difficulty != "" {
					m.allRecipes[i].Difficulty = msg.difficulty
				}
				if msg.cooked != nil {
					m.allRecipes[i].Cooked = *msg.cooked
				}
				break
			}
		}
		m.statusMsg = "Notionに保存しました"
		m.applyFilter()
		return m, nil

	case recipeErrMsg:
		m.err = msg.err
		m.loading = false
		return m, nil

	case tea.KeyMsg:
		if m.loading {
			return m, nil
		}

		// Handle search input mode
		if m.mode == ViewModeSearch {
			switch msg.Type {
			case tea.KeyEsc, tea.KeyEnter:
				m.mode = ViewModeBrowse
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

		// Handle Edit Genre Mode
		if m.mode == ViewModeEditGenre {
			switch msg.String() {
			case "esc":
				m.mode = ViewModeBrowse
				return m, nil
			case "up", "k":
				if m.genreCursor > 0 {
					m.genreCursor--
				}
			case "down", "j":
				if m.genreCursor < len(MasterGenres) {
					m.genreCursor++
				}
			case "enter":
				if m.genreCursor < len(MasterGenres) {
					m.selectedGenre = MasterGenres[m.genreCursor]
				} else {
					m.selectedGenre = "" // "(未設定)" option
				}
				// Next: edit categories
				m.mode = ViewModeEditCategory
				m.catCursor = 0
				return m, nil
			}
			return m, nil
		}

		// Handle Edit Category Mode
		if m.mode == ViewModeEditCategory {
			switch msg.String() {
			case "esc":
				m.mode = ViewModeBrowse
				return m, nil
			case "up", "k":
				if m.catCursor > 0 {
					m.catCursor--
				}
			case "down", "j":
				if m.catCursor < len(MasterCategories)-1 {
					m.catCursor++
				}
			case " ":
				cat := MasterCategories[m.catCursor]
				m.selectedCats[cat] = !m.selectedCats[cat]
				return m, nil
			case "enter":
				// Confirm labels
				var cats []string
				for _, c := range MasterCategories {
					if m.selectedCats[c] {
						cats = append(cats, c)
					}
				}
				recipe := m.recipes[m.editRecipeIdx]
				genre := m.selectedGenre
				m.mode = ViewModeBrowse
				m.statusMsg = "ラベルを保存中..."

				// Update in-memory immediately for responsiveness
				for i, r := range m.allRecipes {
					if r.ID == recipe.ID {
						m.allRecipes[i].Categories = cats
						m.allRecipes[i].Genre = genre
						break
					}
				}
				m.applyFilter()

				return m, func() tea.Msg {
					err := m.client.UpdateRecipe(context.Background(), recipe.ID, cats, genre, "", nil)
					if err != nil {
						return recipeErrMsg{err: err}
					}
					return recipeUpdatedMsg{id: recipe.ID, categories: cats, genre: genre}
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
			if m.cursor < len(m.recipes)-1 {
				m.cursor++
				if m.cursor >= m.listOffset+10 {
					m.listOffset++
				}
			}

		case "/":
			m.mode = ViewModeSearch
			m.search.Focus()
			return m, textinput.Blink

		case "c":
			// Toggle cooked
			if len(m.recipes) > 0 {
				r := m.recipes[m.cursor]
				newCooked := !r.Cooked
				m.statusMsg = "つくってみたステータス更新中..."

				// In-memory update
				for i, item := range m.allRecipes {
					if item.ID == r.ID {
						m.allRecipes[i].Cooked = newCooked
						break
					}
				}
				m.applyFilter()

				return m, func() tea.Msg {
					err := m.client.ToggleCooked(context.Background(), r.ID, newCooked)
					if err != nil {
						return recipeErrMsg{err: err}
					}
					return recipeUpdatedMsg{id: r.ID, cooked: &newCooked}
				}
			}

		case "o", "enter":
			// Open URL in browser
			if len(m.recipes) > 0 {
				r := m.recipes[m.cursor]
				if r.URL != "" {
					openBrowser(r.URL)
					m.statusMsg = fmt.Sprintf("ブラウザで開きました: %s", r.URL)
				} else {
					m.statusMsg = "URLが登録されていません"
				}
			}

		case "e":
			// Open label editor
			if len(m.recipes) > 0 {
				r := m.recipes[m.cursor]
				m.editRecipeIdx = m.cursor
				m.mode = ViewModeEditGenre
				m.genreCursor = 0
				m.selectedGenre = r.Genre
				m.selectedCats = make(map[string]bool)
				for _, c := range r.Categories {
					m.selectedCats[c] = true
				}
				// Set initial genre cursor
				for i, g := range MasterGenres {
					if g == r.Genre {
						m.genreCursor = i
						break
					}
				}
			}
		}
	}

	return m, nil
}

func (m RecipeModel) View() string {
	if m.err != nil {
		return fmt.Sprintf("\n  ❌ エラー: %v\n\n  q で終了します\n", m.err)
	}
	if m.loading {
		return "\n  🍳 Notionからレシピデータを取得中...\n"
	}

	activeBorder := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("205")).Padding(1, 2).Width(78)

	// Sub-views: Edit Genre or Edit Category
	if m.mode == ViewModeEditGenre {
		var sb strings.Builder
		sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Render("🏷️  ジャンルを選択") + "\n\n")
		recipe := m.recipes[m.editRecipeIdx]
		sb.WriteString(fmt.Sprintf("対象: %s\n\n", lipgloss.NewStyle().Bold(true).Render(recipe.Title)))

		for i, g := range MasterGenres {
			cursor := "  "
			if i == m.genreCursor {
				cursor = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Render("> ")
			}
			isSelected := ""
			if g == m.selectedGenre {
				isSelected = lipgloss.NewStyle().Foreground(lipgloss.Color("43")).Render(" (現在選択中)")
			}
			sb.WriteString(fmt.Sprintf("%s%s%s\n", cursor, g, isSelected))
		}
		// Option to clear genre
		noneIdx := len(MasterGenres)
		cursor := "  "
		if m.genreCursor == noneIdx {
			cursor = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Render("> ")
		}
		sb.WriteString(fmt.Sprintf("%s(未設定)\n", cursor))

		ui := activeBorder.Render(sb.String())
		footer := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).MarginTop(1).Render("↑/↓: 選択  Enter: 決定してカテゴリー選択へ  Esc: キャンセル")
		return "\n  " + strings.ReplaceAll(ui, "\n", "\n  ") + "\n  " + footer + "\n"
	}

	if m.mode == ViewModeEditCategory {
		var sb strings.Builder
		sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Render("🏷️  カテゴリーを選択 (複数選択可)") + "\n\n")
		recipe := m.recipes[m.editRecipeIdx]
		sb.WriteString(fmt.Sprintf("対象: %s\n", lipgloss.NewStyle().Bold(true).Render(recipe.Title)))
		sb.WriteString(fmt.Sprintf("ジャンル: %s\n\n", lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Render(m.selectedGenre)))

		for i, c := range MasterCategories {
			cursor := "  "
			if i == m.catCursor {
				cursor = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Render("> ")
			}
			check := "[ ]"
			if m.selectedCats[c] {
				check = lipgloss.NewStyle().Foreground(lipgloss.Color("43")).Render("[✓]")
			}
			sb.WriteString(fmt.Sprintf("%s%s %s\n", cursor, check, c))
		}

		ui := activeBorder.Render(sb.String())
		footer := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).MarginTop(1).Render("↑/↓: 移動  Space: チェック切替  Enter: 保存  Esc: キャンセル")
		return "\n  " + strings.ReplaceAll(ui, "\n", "\n  ") + "\n  " + footer + "\n"
	}

	var sb strings.Builder

	// Header
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	sb.WriteString(titleStyle.Render("🍳 レシピ管理 (ntui recipe)") + "\n\n")

	// Tabs
	tabs := []string{"未分類", "すべて", "未調理", "つくってみた"}
	var tabStrs []string
	activeTabStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Underline(true)
	inactiveTabStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	for i, t := range tabs {
		label := fmt.Sprintf("[%s]", t)
		if RecipeTab(i) == m.tab {
			tabStrs = append(tabStrs, activeTabStyle.Render(label))
		} else {
			tabStrs = append(tabStrs, inactiveTabStyle.Render(label))
		}
	}
	sb.WriteString(strings.Join(tabStrs, "  ") + "\n\n")

	// Search bar if active or has filter
	if m.mode == ViewModeSearch || m.search.Value() != "" {
		sb.WriteString(m.search.View() + "\n\n")
	}

	// Status message
	if m.statusMsg != "" {
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("43")).Render("💡 "+m.statusMsg) + "\n\n")
	}

	// List
	if len(m.recipes) == 0 {
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("該当するレシピがありません。\n"))
	} else {
		displayLimit := 10
		start := m.listOffset
		end := start + displayLimit
		if end > len(m.recipes) {
			end = len(m.recipes)
		}

		genreStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
		catStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
		cookedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("43")).Bold(true)
		unlabeledBadge := lipgloss.NewStyle().Background(lipgloss.Color("196")).Foreground(lipgloss.Color("231")).Bold(true).Render(" 未分類 ")

		for i := start; i < end; i++ {
			r := m.recipes[i]
			cursor := "  "
			if i == m.cursor {
				cursor = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Render("> ")
			}

			cookedIcon := "[ ]"
			if r.Cooked {
				cookedIcon = cookedStyle.Render("[✓]")
			}

			genreText := ""
			if r.Genre != "" {
				genreText = genreStyle.Render(fmt.Sprintf("[%s]", r.Genre)) + " "
			}

			catText := ""
			if len(r.Categories) > 0 {
				catText = catStyle.Render(fmt.Sprintf("(%s)", strings.Join(r.Categories, ", "))) + " "
			}

			badge := ""
			if r.IsUnlabeled() {
				badge = unlabeledBadge + " "
			}

			title := r.Title
			if i == m.cursor {
				title = lipgloss.NewStyle().Bold(true).Render(title)
			}

			line := fmt.Sprintf("%s%s %s%s%s%s", cursor, cookedIcon, badge, genreText, catText, title)
			sb.WriteString(line + "\n")

			// Display URL on active row if exists
			if i == m.cursor && r.URL != "" {
				urlStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("242")).PaddingLeft(6)
				sb.WriteString(urlStyle.Render("🔗 "+r.URL) + "\n")
			}
		}

		// Count summary
		countInfo := fmt.Sprintf("\n  (全 %d 件中 %d〜%d 件目を表示)", len(m.recipes), start+1, end)
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(countInfo) + "\n")
	}

	ui := activeBorder.Render(sb.String())

	// Footer
	footer := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).MarginTop(1).Render(
		"↑/↓: 移動  Tab/←/→: タブ切替  o/Enter: ブラウザで開く  c: つくってみた  e: ラベル編集  /: 検索  q: 終了",
	)

	return "\n  " + strings.ReplaceAll(ui, "\n", "\n  ") + "\n  " + footer + "\n"
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default: // linux, etc.
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
