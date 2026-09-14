package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/joho/godotenv"
	"github.com/ookawayuuto/notion-tui/internal/notion"
	"github.com/ookawayuuto/notion-tui/internal/tui"
	"github.com/spf13/cobra"
)

var taskName string
var taskDue string
var taskContent string
var taskEdit bool

var taskCmd = &cobra.Command{
	Use:   "task",
	Short: "Personal タスクの管理（一覧・ステータス更新・期限設定・詳細編集）",
	Long: `NotionのTasksデータベースから「Personal」カテゴリのタスクを取得・管理します。
引数なしで実行するとTUIが起動します。

【CLIモード（フラグ指定で直接実行）】
  --name (-n) オプションを指定すると、TUIを起動せずに直接タスクを追加できます。
  タスクの内容(本文)は、--content (-c)、標準入力(パイプ)、または --edit (-e) を使って柔軟に指定可能です。`,
	Example: `  # TUIを起動してタスクを管理する
  ntui task

  # CLIから直接タスクを登録する
  ntui task -n "ブログ記事の執筆" -d "2026-09-20"

  # ワンライナーで詳細内容付きでタスクを登録する
  ntui task -n "買い物" -c "牛乳、卵、パン"

  # エディタを起動してタスクの詳細を入力し、登録する
  ntui task -n "今週の振り返り" -e

  # ファイルや標準入力の内容をタスク詳細として登録する
  cat meeting_notes.md | ntui task -n "定例ミーティング議事録"`,
	Run: func(cmd *cobra.Command, args []string) {
		godotenv.Load()

		token := os.Getenv("NOTION_TOKEN")
		tasksID := os.Getenv("NOTION_TASKS_DB_ID")

		if token == "" || tasksID == "" {
			fmt.Println("エラー: .env に NOTION_TOKEN と NOTION_TASKS_DB_ID を設定してください。")
			os.Exit(1)
		}

		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			b, err := io.ReadAll(os.Stdin)
			if err == nil && len(b) > 0 {
				if taskContent != "" {
					taskContent += "\n\n"
				}
				taskContent += string(b)
			}
		}

		if taskEdit {
			editor := os.Getenv("EDITOR")
			if editor == "" {
				editor = "vim"
			}

			tmpFile, err := os.CreateTemp("", "ntui-task-*.md")
			if err != nil {
				fmt.Printf("一時ファイルの作成に失敗しました: %v\n", err)
				os.Exit(1)
			}
			tmpFilePath := tmpFile.Name()

			if taskContent != "" {
				tmpFile.WriteString(taskContent)
			}
			tmpFile.Close()
			defer os.Remove(tmpFilePath)

			cmdEdit := exec.Command(editor, tmpFilePath)
			cmdEdit.Stdin = os.Stdin
			cmdEdit.Stdout = os.Stdout
			cmdEdit.Stderr = os.Stderr

			if err := cmdEdit.Run(); err != nil {
				fmt.Printf("エディタの起動に失敗しました: %v\n", err)
				os.Exit(1)
			}

			b, err := os.ReadFile(tmpFilePath)
			if err == nil {
				taskContent = string(b)
			}
		}

		client := notion.NewTaskClient(token, tasksID)

		if taskName != "" {
			fmt.Printf("タスク「%s」を登録中...\n", taskName)
			_, err := client.AddPersonalTask(context.Background(), taskName, taskDue, taskContent)
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
	taskCmd.Flags().StringVarP(&taskContent, "content", "c", "", "タスクの詳細内容")
	taskCmd.Flags().BoolVarP(&taskEdit, "edit", "e", false, "エディタを起動して詳細内容を入力する")
	rootCmd.AddCommand(taskCmd)
}
