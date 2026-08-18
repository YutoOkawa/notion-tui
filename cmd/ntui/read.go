package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ookawayuuto/notion-tui/internal/notion"
	"github.com/ookawayuuto/notion-tui/internal/tui"
	"github.com/spf13/cobra"
)

var readCmd = &cobra.Command{
	Use:   "read",
	Short: "読書管理TUIを起動します",
	Run: func(cmd *cobra.Command, args []string) {
		token := os.Getenv("NOTION_TOKEN")
		tasksID := os.Getenv("NOTION_TASKS_DB_ID")
		projectID := os.Getenv("NOTION_READING_PROJECT_ID")

		if token == "" || tasksID == "" || projectID == "" {
			fmt.Println("エラー: 環境変数に NOTION_TOKEN, NOTION_TASKS_DB_ID, NOTION_READING_PROJECT_ID を設定してください")
			os.Exit(1)
		}

		client := notion.NewReadClient(token, tasksID, projectID)
		
		model := tui.NewReadModel(client)

		p := tea.NewProgram(model)
		if _, err := p.Run(); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(readCmd)
}
