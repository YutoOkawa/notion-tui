package main

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "ntui",
	Short: "Notion TUI Client",
	Long:  `ntui は Notion をターミナルから爆速で操作するための CLI/TUI ツールです。`,
	Run: func(cmd *cobra.Command, args []string) {
		// 将来ここを「総合ダッシュボードの起動（ハイブリッド型）」に変更します
		fmt.Println("総合ダッシュボード機能は現在開発中です。")
		fmt.Println("買い物リストを起動するには 'ntui shop' コマンドを実行してください。")
		fmt.Println("その他のコマンドは 'ntui --help' で確認できます。")
	},
}

func Execute() {
	// プロジェクトルートで実行されることを想定して .env を読み込み
	_ = godotenv.Load() 

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
