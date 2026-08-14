package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ookawayuuto/notion-tui/internal/notion"
	"github.com/ookawayuuto/notion-tui/internal/tui"
	"github.com/spf13/cobra"
)

var monstCmd = &cobra.Command{
	Use:   "monst",
	Short: "モンストの育成リスト・わくわくの実を管理します",
	Run: func(cmd *cobra.Command, args []string) {
		token := os.Getenv("NOTION_TOKEN")
		monsterID := os.Getenv("NOTION_MONSTER_DB_ID")
		wakuwakuID := os.Getenv("NOTION_WAKUWAKU_DB_ID")

		if token == "" || monsterID == "" || wakuwakuID == "" {
			fmt.Println("エラー: 環境変数に NOTION_TOKEN, NOTION_MONSTER_DB_ID, NOTION_WAKUWAKU_DB_ID を設定してください")
			os.Exit(1)
		}

		client := notion.NewMonstClient(token, monsterID, wakuwakuID)
		model := tui.NewMonstModel(client)

		p := tea.NewProgram(model, tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(monstCmd)
}
