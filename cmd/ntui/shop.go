package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ookawayuuto/notion-tui/internal/notion"
	"github.com/ookawayuuto/notion-tui/internal/tui"
	"github.com/spf13/cobra"
)

var shopCmd = &cobra.Command{
	Use:   "shop",
	Short: "買い物リストと在庫管理TUIを起動します",
	Run: func(cmd *cobra.Command, args []string) {
		token := os.Getenv("NOTION_TOKEN")
		invID := os.Getenv("NOTION_INVENTORY_DB_ID")
		shopID := os.Getenv("NOTION_SHOPPING_DB_ID")

		if token == "" || invID == "" || shopID == "" {
			fmt.Println("エラー: 環境変数に NOTION_TOKEN, NOTION_INVENTORY_DB_ID, NOTION_SHOPPING_DB_ID を設定してください")
			os.Exit(1)
		}

		client := notion.NewClient(token, invID, shopID)
		
		// tuiパッケージの買い物用モデルを初期化
		model := tui.NewShopModel(client)

		p := tea.NewProgram(model)
		if _, err := p.Run(); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(shopCmd)
}
