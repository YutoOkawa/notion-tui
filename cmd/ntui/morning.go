package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/joho/godotenv"
	"github.com/ookawayuuto/notion-tui/internal/notion"
	"github.com/ookawayuuto/notion-tui/internal/tui"
	"github.com/spf13/cobra"
)

var morningCmd = &cobra.Command{
	Use:     "morning",
	Aliases: []string{"catchup"},
	Short:   "朝のキャッチアップレポートを閲覧・管理します",
	Long: `Notion の morning-catchup データベースに記録された調査レポートを
ターミナル上で一覧閲覧し、詳細をスクロールして読むことができるTUIです。`,
	Example: `  # 朝のキャッチアップTUIを起動
  ntui morning

  # エイリアスで起動
  ntui catchup`,
	Run: func(cmd *cobra.Command, args []string) {
		_ = godotenv.Load()

		token := os.Getenv("NOTION_TOKEN")
		dbID := os.Getenv("NOTION_MORNING_DB_ID")

		if token == "" || dbID == "" {
			fmt.Println("エラー: .env に NOTION_TOKEN と NOTION_MORNING_DB_ID を設定してください。")
			os.Exit(1)
		}

		client := notion.NewMorningClient(token, dbID)
		model := tui.NewMorningModel(client)

		p := tea.NewProgram(model, tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			fmt.Printf("エラー: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(morningCmd)
}
