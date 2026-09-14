package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/joho/godotenv"
	"github.com/ookawayuuto/notion-tui/internal/notion"
	"github.com/ookawayuuto/notion-tui/internal/tui"
	"github.com/spf13/cobra"
)

var kbCmd = &cobra.Command{
	Use:   "kb [theme]",
	Short: "AIに調査してほしいテーマをナレッジベースに追加",
	Long: `ナレッジベースDBに「下書き」ラベルで新規作成を行うコマンドです。
引数なしで実行するとTUIが起動します。

【CLIモード（引数ありで実行）】
  テーマ名を引数として直接渡すことで、TUIをスキップして即座に下書きアイテムを登録できます。`,
	Example: `  # TUIを起動してカテゴリを選びながらテーマを登録する
  ntui kb

  # コマンドラインから直接テーマを登録する
  ntui kb "Go言語のインターフェースのベストプラクティス"
  ntui kb GraphQLとREST APIの比較`,
	Run: func(cmd *cobra.Command, args []string) {
		godotenv.Load()

		token := os.Getenv("NOTION_TOKEN")
		kbID := os.Getenv("NOTION_KB_DB_ID")

		if token == "" || kbID == "" {
			fmt.Println("エラー: .env に NOTION_TOKEN と NOTION_KB_DB_ID を設定してください。")
			os.Exit(1)
		}

		client := notion.NewKBClient(token, kbID)
		ctx := context.Background()

		if len(args) > 0 {
			// CLI Mode: Direct post
			title := strings.Join(args, " ")
			fmt.Printf("「%s」をナレッジベースに登録中...\n", title)
			url, err := client.AddDraftItem(ctx, title, "")
			if err != nil {
				fmt.Printf("エラー: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("登録完了しました！\nURL: %s\n", url)
			return
		}

		// TUI Mode
		fmt.Println("ナレッジベース情報を読み込み中...")
		categories, err := client.FetchCategories(ctx)
		if err != nil {
			fmt.Printf("警告: カテゴリの取得に失敗しました (%v)\n", err)
		}

		p := tea.NewProgram(tui.NewKBModel(client, categories))
		if _, err := p.Run(); err != nil {
			fmt.Printf("TUIエラー: %v", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(kbCmd)
}
