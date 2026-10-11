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

func getTechClient() *notion.TechClient {
	_ = godotenv.Load()
	token := os.Getenv("NOTION_TOKEN")
	techDBID := os.Getenv("NOTION_TECH_DB_ID")

	if token == "" || techDBID == "" {
		fmt.Fprintln(os.Stderr, "エラー: .env に NOTION_TOKEN と NOTION_TECH_DB_ID を設定してください。")
		os.Exit(1)
	}

	return notion.NewTechClient(token, techDBID)
}

var techCmd = &cobra.Command{
	Use:   "tech",
	Short: "TechBlog Stack管理（閲覧・検索・ラベル付与・ブラウザ起動）",
	Long: `NotionのTechBlog Stackデータベースから記事を取得・管理します。
引数なしで実行するとTUIが起動します。
サブコマンド（list, update, batch-update）を使ってCLIから直接未分類記事の取得やラベル付与を行えます。`,
	Run: func(cmd *cobra.Command, args []string) {
		client := getTechClient()
		model := tui.NewTechModel(client)

		p := tea.NewProgram(model, tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			fmt.Printf("TUIエラー: %v\n", err)
			os.Exit(1)
		}
	},
}

var (
	listTechUnlabeled bool
	listTechUnread    bool
	listTechRead      bool
	listTechTag       string
	listTechGenre     string
	listTechLimit     int
	listTechJSON      bool
)

var techListCmd = &cobra.Command{
	Use:   "list",
	Short: "技術記事一覧を表示します",
	Long:  `TechBlog Stackから記事を取得して一覧表示します。--unlabeled を指定すると未分類の記事のみ抽出します。`,
	Run: func(cmd *cobra.Command, args []string) {
		client := getTechClient()
		ctx := context.Background()

		items, err := client.FetchTechItems(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "エラー: %v\n", err)
			os.Exit(1)
		}

		var filtered []domain.TechItem
		for _, item := range items {
			if listTechUnlabeled && !item.IsUnlabeled() {
				continue
			}
			if listTechUnread && !item.IsUnread() {
				continue
			}
			if listTechRead && !item.IsRead() {
				continue
			}
			if listTechTag != "" {
				matched := false
				for _, t := range item.Tags {
					if strings.EqualFold(t, listTechTag) {
						matched = true
						break
					}
				}
				if !matched {
					continue
				}
			}
			if listTechGenre != "" && !strings.EqualFold(item.Genre, listTechGenre) {
				continue
			}

			filtered = append(filtered, item)
			if listTechLimit > 0 && len(filtered) >= listTechLimit {
				break
			}
		}

		if listTechJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(filtered); err != nil {
				fmt.Fprintf(os.Stderr, "JSONエンコードエラー: %v\n", err)
				os.Exit(1)
			}
			return
		}

		if len(filtered) == 0 {
			fmt.Println("条件に一致する記事はありませんでした。")
			return
		}

		fmt.Printf("取得件数: %d 件\n\n", len(filtered))
		for i, item := range filtered {
			statusStr := fmt.Sprintf("[%s]", item.Status)
			if item.Status == "" {
				statusStr = "[未読]"
			}

			var tags []string
			if item.Genre != "" {
				tags = append(tags, item.Genre)
			}
			if len(item.Tags) > 0 {
				tags = append(tags, strings.Join(item.Tags, ", "))
			}
			tagStr := ""
			if len(tags) > 0 {
				tagStr = fmt.Sprintf(" [%s]", strings.Join(tags, " / "))
			}

			unlabeledNotice := ""
			if item.IsUnlabeled() {
				unlabeledNotice = " (未分類)"
			}

			fmt.Printf("%3d. %s %s%s%s\n", i+1, statusStr, item.Title, tagStr, unlabeledNotice)
			fmt.Printf("     ID: %s\n", item.ID)
			if item.URL != "" {
				fmt.Printf("     URL: %s\n", item.URL)
			}
			fmt.Println()
		}
	},
}

var (
	updateTechGenre  string
	updateTechTags   string
	updateTechStatus string
	updateTechMemo   string
)

var techUpdateCmd = &cobra.Command{
	Use:   "update <tech-id>",
	Short: "技術記事のラベルやステータスを更新します",
	Long:  `指定したIDの記事に対して、ジャンル、タグ、ステータス、メモを更新します。`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		itemID := args[0]
		client := getTechClient()
		ctx := context.Background()

		var tags []string
		if cmd.Flags().Changed("tag") {
			rawTags := strings.Split(updateTechTags, ",")
			for _, t := range rawTags {
				trimmed := strings.TrimSpace(t)
				if trimmed != "" {
					tags = append(tags, trimmed)
				}
			}
		}

		err := client.UpdateTechItem(
			ctx,
			notionapi.ObjectID(itemID),
			updateTechGenre,
			tags,
			updateTechStatus,
			updateTechMemo,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "更新エラー: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("記事 (ID: %s) を更新しました！\n", itemID)
		if updateTechGenre != "" {
			fmt.Printf("  ジャンル: %s\n", updateTechGenre)
		}
		if len(tags) > 0 {
			fmt.Printf("  タグ: %s\n", strings.Join(tags, ", "))
		}
		if updateTechStatus != "" {
			fmt.Printf("  ステータス: %s\n", updateTechStatus)
		}
		if updateTechMemo != "" {
			fmt.Printf("  メモ: %s\n", updateTechMemo)
		}
	},
}

type BatchUpdateTechItem struct {
	ID     string   `json:"id"`
	Genre  string   `json:"genre,omitempty"`
	Tags   []string `json:"tags,omitempty"`
	Status string   `json:"status,omitempty"`
	Memo   string   `json:"memo,omitempty"`
}

var techBatchUpdateCmd = &cobra.Command{
	Use:   "batch-update",
	Short: "標準入力のJSONから複数記事を一括更新します",
	Long: `標準入力からJSON配列を受け取り、複数の記事を一括更新します。
AI Agentが推測したラベルをまとめて適用する際に便利です。

JSON例:
[
  {
    "id": "3f62e9fd-d1f7-8136-8521-ff4b99e4d063",
    "genre": "AI/ML",
    "tags": ["LLM"],
    "status": "未読"
  }
]`,
	Run: func(cmd *cobra.Command, args []string) {
		input, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "標準入力の読み込みエラー: %v\n", err)
			os.Exit(1)
		}

		var items []BatchUpdateTechItem
		if err := json.Unmarshal(input, &items); err != nil {
			fmt.Fprintf(os.Stderr, "JSONパースエラー: %v\n", err)
			os.Exit(1)
		}

		if len(items) == 0 {
			fmt.Println("更新対象のアイテムがありませんでした。")
			return
		}

		client := getTechClient()
		ctx := context.Background()

		fmt.Printf("%d 件の記事を一括更新します...\n", len(items))
		successCount := 0
		for _, item := range items {
			if item.ID == "" {
				continue
			}
			err := client.UpdateTechItem(
				ctx,
				notionapi.ObjectID(item.ID),
				item.Genre,
				item.Tags,
				item.Status,
				item.Memo,
			)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  ❌ ID: %s 更新失敗: %v\n", item.ID, err)
			} else {
				fmt.Printf("  ✓ ID: %s (ジャンル: %s, タグ: %v)\n", item.ID, item.Genre, item.Tags)
				successCount++
			}
		}

		fmt.Printf("\n一括更新完了: %d / %d 件 成功\n", successCount, len(items))
	},
}

func init() {
	// list flags
	techListCmd.Flags().BoolVarP(&listTechUnlabeled, "unlabeled", "u", false, "ジャンルまたはタグが未設定の記事のみ抽出")
	techListCmd.Flags().BoolVar(&listTechUnread, "unread", false, "未読記事のみ抽出")
	techListCmd.Flags().BoolVar(&listTechRead, "read", false, "読了記事のみ抽出")
	techListCmd.Flags().StringVarP(&listTechTag, "tag", "t", "", "タグで絞り込み")
	techListCmd.Flags().StringVarP(&listTechGenre, "genre", "g", "", "ジャンルで絞り込み")
	techListCmd.Flags().IntVarP(&listTechLimit, "limit", "l", 0, "最大取得件数 (0=全件)")
	techListCmd.Flags().BoolVar(&listTechJSON, "json", false, "JSON形式で出力")

	// update flags
	techUpdateCmd.Flags().StringVarP(&updateTechGenre, "genre", "g", "", "ジャンル（例: Backend）")
	techUpdateCmd.Flags().StringVarP(&updateTechTags, "tag", "t", "", "タグ（カンマ区切り、例: Go,AWS）")
	techUpdateCmd.Flags().StringVarP(&updateTechStatus, "status", "s", "", "ステータス（例: 読了）")
	techUpdateCmd.Flags().StringVarP(&updateTechMemo, "memo", "m", "", "メモ")

	techCmd.AddCommand(techListCmd)
	techCmd.AddCommand(techUpdateCmd)
	techCmd.AddCommand(techBatchUpdateCmd)

	rootCmd.AddCommand(techCmd)
}
