package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/joho/godotenv"
	"github.com/ookawayuuto/notion-tui/internal/notion"
	"github.com/ookawayuuto/notion-tui/internal/tui"
	"github.com/spf13/cobra"
)

var memoCmd = &cobra.Command{
	Use:   "memo",
	Short: "アイデア＆思考ログの管理 (一覧/作成)",
	Run: func(cmd *cobra.Command, args []string) {
		godotenv.Load()
		token := os.Getenv("NOTION_TOKEN")
		memoDBID := os.Getenv("NOTION_MEMO_DB_ID")

		if token == "" || memoDBID == "" {
			fmt.Println("エラー: .env に NOTION_TOKEN と NOTION_MEMO_DB_ID を設定してください。")
			os.Exit(1)
		}

		client := notion.NewMemoClient(token, memoDBID)
		p := tea.NewProgram(tui.NewMemoModel(client), tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			fmt.Printf("TUIエラー: %v", err)
			os.Exit(1)
		}
	},
}

var memoAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Vim等のエディタを開いて新規メモを作成する",
	Run: func(cmd *cobra.Command, args []string) {
		godotenv.Load()
		token := os.Getenv("NOTION_TOKEN")
		memoDBID := os.Getenv("NOTION_MEMO_DB_ID")

		if token == "" || memoDBID == "" {
			fmt.Println("エラー: .env に NOTION_TOKEN と NOTION_MEMO_DB_ID を設定してください。")
			os.Exit(1)
		}

		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "vim"
		}

		// Create temp file
		tmpFile, err := os.CreateTemp("", "ntui-memo-*.md")
		if err != nil {
			fmt.Printf("一時ファイルの作成に失敗しました: %v\n", err)
			os.Exit(1)
		}
		defer os.Remove(tmpFile.Name())

		// Close so editor can open it without issues
		tmpFile.Close() 

		// Run editor
		c := exec.Command(editor, tmpFile.Name())
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr

		if err := c.Run(); err != nil {
			fmt.Printf("エディタの終了時にエラーが発生しました: %v\n", err)
			os.Exit(1)
		}

		// Read file
		content, err := os.ReadFile(tmpFile.Name())
		if err != nil {
			fmt.Printf("ファイルの読み込みに失敗しました: %v\n", err)
			os.Exit(1)
		}

		text := strings.TrimSpace(string(content))
		if text == "" {
			fmt.Println("メモが空のため保存をキャンセルしました。")
			return
		}

		// Parse title and body
		lines := strings.Split(text, "\n")
		title := strings.TrimSpace(lines[0])
		
		// Remove markdown heading (#) if present for title
		title = strings.TrimPrefix(title, "# ")
		title = strings.TrimSpace(title)

		body := ""
		if len(lines) > 1 {
			body = strings.TrimSpace(strings.Join(lines[1:], "\n"))
		}

		fmt.Println("Notionへ保存中...")
		client := notion.NewMemoClient(token, memoDBID)
		err = client.AddMemo(context.Background(), title, body)
		if err != nil {
			fmt.Printf("エラー: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✅ 「%s」をアイデア&思考ログに保存しました！\n", title)
	},
}

func init() {
	memoCmd.AddCommand(memoAddCmd)
	rootCmd.AddCommand(memoCmd)
}
