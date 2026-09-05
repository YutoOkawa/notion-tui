package main

import (
	"context"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/joho/godotenv"
	"github.com/ookawayuuto/notion-tui/internal/notion"
	"github.com/ookawayuuto/notion-tui/internal/tui"
	"github.com/spf13/cobra"
)

var taskName string
var taskDue string

var taskCmd = &cobra.Command{
	Use:   "task",
	Short: "Personal タスクの管理（一覧・ステータス更新・期限設定）",
	Long:  `NotionのTasksデータベースから「Personal」カテゴリのタスクを取得し、ステータスの変更やDiscord通知用の期限(Due)を設定します。
--name オプションを指定すると、TUIを起動せずに直接タスクを追加できます。`,
	Run: func(cmd *cobra.Command, args []string) {
		godotenv.Load()

		token := os.Getenv("NOTION_TOKEN")
		tasksID := os.Getenv("NOTION_TASKS_DB_ID")

		if token == "" || tasksID == "" {
			fmt.Println("エラー: .env に NOTION_TOKEN と NOTION_TASKS_DB_ID を設定してください。")
			os.Exit(1)
		}

		client := notion.NewTaskClient(token, tasksID)

		if taskName != "" {
			fmt.Printf("タスク「%s」を登録中...\n", taskName)
			_, err := client.AddPersonalTask(context.Background(), taskName, taskDue)
			if err != nil {
				fmt.Printf("エラー: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("登録完了しました！")
			return
		}

		p := tea.NewProgram(tui.NewTaskModel(client), tea.WithAltScreen())

		if _, err := p.Run(); err != nil {
			fmt.Printf("TUIエラー: %v", err)
			os.Exit(1)
		}
	},
}

func init() {
	taskCmd.Flags().StringVarP(&taskName, "name", "n", "", "追加するタスクの名前")
	taskCmd.Flags().StringVarP(&taskDue, "due", "d", "", "タスクの期限 (YYYY-MM-DD形式)")
	rootCmd.AddCommand(taskCmd)
}
