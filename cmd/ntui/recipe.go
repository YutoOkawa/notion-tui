package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/joho/godotenv"
	"github.com/jomei/notionapi"
	"github.com/ookawayuuto/notion-tui/internal/domain"
	"github.com/ookawayuuto/notion-tui/internal/notion"
	"github.com/ookawayuuto/notion-tui/internal/tui"
	"github.com/spf13/cobra"
)

func getRecipeClient() *notion.RecipeClient {
	_ = godotenv.Load()
	token := os.Getenv("NOTION_TOKEN")
	recipeDBID := os.Getenv("NOTION_RECIPE_DB_ID")

	if token == "" || recipeDBID == "" {
		fmt.Fprintln(os.Stderr, "エラー: .env に NOTION_TOKEN と NOTION_RECIPE_DB_ID を設定してください。")
		os.Exit(1)
	}

	return notion.NewRecipeClient(token, recipeDBID)
}

var recipeCmd = &cobra.Command{
	Use:   "recipe",
	Short: "レシピ管理（閲覧・検索・ラベル付与・ブラウザ起動）",
	Long: `Notionのレシピデータベースからレシピを取得・管理します。
引数なしで実行するとTUIが起動します。
サブコマンド（list, update, batch-update）を使ってCLIから直接未分類レシピの取得やラベル付与を行えます。`,
	Run: func(cmd *cobra.Command, args []string) {
		client := getRecipeClient()
		model := tui.NewRecipeModel(client)

		p := tea.NewProgram(model, tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			fmt.Printf("TUIエラー: %v\n", err)
			os.Exit(1)
		}
	},
}

var (
	listUnlabeled bool
	listCooked    bool
	listUncooked  bool
	listCategory  string
	listGenre     string
	listLimit     int
	listJSON      bool
)

var recipeListCmd = &cobra.Command{
	Use:   "list",
	Short: "レシピ一覧を表示します",
	Long:  `レシピデータベースからレシピを取得して一覧表示します。--unlabeled を指定するとラベル未付与のレシピのみ抽出します。`,
	Run: func(cmd *cobra.Command, args []string) {
		client := getRecipeClient()
		ctx := context.Background()

		recipes, err := client.FetchRecipes(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "エラー: %v\n", err)
			os.Exit(1)
		}

		var filtered []domain.RecipeItem
		for _, r := range recipes {
			if listUnlabeled && !r.IsUnlabeled() {
				continue
			}
			if listCooked && !r.Cooked {
				continue
			}
			if listUncooked && r.Cooked {
				continue
			}
			if listCategory != "" {
				matched := false
				for _, c := range r.Categories {
					if strings.EqualFold(c, listCategory) {
						matched = true
						break
					}
				}
				if !matched {
					continue
				}
			}
			if listGenre != "" && !strings.EqualFold(r.Genre, listGenre) {
				continue
			}

			filtered = append(filtered, r)
			if listLimit > 0 && len(filtered) >= listLimit {
				break
			}
		}

		if listJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(filtered); err != nil {
				fmt.Fprintf(os.Stderr, "JSONエンコードエラー: %v\n", err)
				os.Exit(1)
			}
			return
		}

		if len(filtered) == 0 {
			fmt.Println("条件に一致するレシピはありませんでした。")
			return
		}

		fmt.Printf("取得件数: %d 件\n\n", len(filtered))
		for i, r := range filtered {
			cookedStr := "[ ]"
			if r.Cooked {
				cookedStr = "[✓]"
			}

			var tags []string
			if r.Genre != "" {
				tags = append(tags, r.Genre)
			}
			if len(r.Categories) > 0 {
				tags = append(tags, strings.Join(r.Categories, ", "))
			}
			tagStr := ""
			if len(tags) > 0 {
				tagStr = fmt.Sprintf(" [%s]", strings.Join(tags, " / "))
			}

			unlabeledNotice := ""
			if r.IsUnlabeled() {
				unlabeledNotice = " (未分類)"
			}

			fmt.Printf("%3d. %s %s%s%s\n", i+1, cookedStr, r.Title, tagStr, unlabeledNotice)
			fmt.Printf("     ID: %s\n", r.ID)
			if r.URL != "" {
				fmt.Printf("     URL: %s\n", r.URL)
			}
			fmt.Println()
		}
	},
}

var (
	updateCategories string
	updateGenre      string
	updateDifficulty string
	updateCookedStr  string
)

var recipeUpdateCmd = &cobra.Command{
	Use:   "update <recipe-id>",
	Short: "レシピのラベルやステータスを更新します",
	Long:  `指定したIDのレシピに対して、カテゴリー、ジャンル、難易度、つくってみたステータスを更新します。`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		recipeID := args[0]
		client := getRecipeClient()
		ctx := context.Background()

		var cats []string
		if cmd.Flags().Changed("category") {
			rawCats := strings.Split(updateCategories, ",")
			for _, c := range rawCats {
				trimmed := strings.TrimSpace(c)
				if trimmed != "" {
					cats = append(cats, trimmed)
				}
			}
		}

		var cookedPtr *bool
		if cmd.Flags().Changed("cooked") {
			val := strings.ToLower(updateCookedStr) == "true" || updateCookedStr == "1"
			cookedPtr = &val
		}

		err := client.UpdateRecipe(
			ctx,
			notionapi.ObjectID(recipeID),
			cats,
			updateGenre,
			updateDifficulty,
			cookedPtr,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "更新エラー: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("レシピ (ID: %s) を更新しました！\n", recipeID)
		if len(cats) > 0 {
			fmt.Printf("  カテゴリー: %s\n", strings.Join(cats, ", "))
		}
		if updateGenre != "" {
			fmt.Printf("  ジャンル: %s\n", updateGenre)
		}
		if updateDifficulty != "" {
			fmt.Printf("  難易度: %s\n", updateDifficulty)
		}
		if cookedPtr != nil {
			fmt.Printf("  つくってみた: %v\n", *cookedPtr)
		}
	},
}

type BatchUpdateItem struct {
	ID         string   `json:"id"`
	Categories []string `json:"categories,omitempty"`
	Genre      string   `json:"genre,omitempty"`
	Difficulty string   `json:"difficulty,omitempty"`
	Cooked     *bool    `json:"cooked,omitempty"`
}

var recipeBatchUpdateCmd = &cobra.Command{
	Use:   "batch-update",
	Short: "標準入力のJSONから複数レシピを一括更新します",
	Long: `標準入力からJSON配列を受け取り、複数のレシピを一括更新します。
AI Agentが推測したラベルをまとめて適用する際に便利です。

JSON例:
[
  {
    "id": "3d32e9fd-d1f7-8112-8e63-fb7d92a0df76",
    "categories": ["麺類"],
    "genre": "中華"
  }
]`,
	Run: func(cmd *cobra.Command, args []string) {
		input, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "標準入力の読み込みエラー: %v\n", err)
			os.Exit(1)
		}

		var items []BatchUpdateItem
		if err := json.Unmarshal(input, &items); err != nil {
			fmt.Fprintf(os.Stderr, "JSONパースエラー: %v\n", err)
			os.Exit(1)
		}

		if len(items) == 0 {
			fmt.Println("更新対象のアイテムがありませんでした。")
			return
		}

		client := getRecipeClient()
		ctx := context.Background()

		fmt.Printf("%d 件のレシピを一括更新します...\n", len(items))
		successCount := 0
		for _, item := range items {
			if item.ID == "" {
				continue
			}
			err := client.UpdateRecipe(
				ctx,
				notionapi.ObjectID(item.ID),
				item.Categories,
				item.Genre,
				item.Difficulty,
				item.Cooked,
			)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  ❌ ID: %s 更新失敗: %v\n", item.ID, err)
			} else {
				fmt.Printf("  ✓ ID: %s (ジャンル: %s, カテゴリー: %v)\n", item.ID, item.Genre, item.Categories)
				successCount++
			}
		}

		fmt.Printf("\n一括更新完了: %d / %d 件 成功\n", successCount, len(items))
	},
}

func init() {
	// list flags
	recipeListCmd.Flags().BoolVarP(&listUnlabeled, "unlabeled", "u", false, "カテゴリーまたはジャンルが未設定のレシピのみ抽出")
	recipeListCmd.Flags().BoolVar(&listCooked, "cooked", false, "調理済みレシピのみ抽出")
	recipeListCmd.Flags().BoolVar(&listUncooked, "uncooked", false, "未調理レシピのみ抽出")
	recipeListCmd.Flags().StringVarP(&listCategory, "category", "c", "", "カテゴリーで絞り込み")
	recipeListCmd.Flags().StringVarP(&listGenre, "genre", "g", "", "ジャンルで絞り込み")
	recipeListCmd.Flags().IntVarP(&listLimit, "limit", "l", 0, "最大取得件数 (0=全件)")
	recipeListCmd.Flags().BoolVar(&listJSON, "json", false, "JSON形式で出力")

	// update flags
	recipeUpdateCmd.Flags().StringVarP(&updateCategories, "category", "c", "", "カテゴリー（カンマ区切り、例: 肉,野菜）")
	recipeUpdateCmd.Flags().StringVarP(&updateGenre, "genre", "g", "", "ジャンル（例: 和食）")
	recipeUpdateCmd.Flags().StringVarP(&updateDifficulty, "difficulty", "d", "", "難易度（例: ★）")
	recipeUpdateCmd.Flags().StringVar(&updateCookedStr, "cooked", "", "つくってみたフラグ (true / false)")

	recipeCmd.AddCommand(recipeListCmd)
	recipeCmd.AddCommand(recipeUpdateCmd)
	recipeCmd.AddCommand(recipeBatchUpdateCmd)

	rootCmd.AddCommand(recipeCmd)
}
