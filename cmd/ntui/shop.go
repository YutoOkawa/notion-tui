package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ookawayuuto/notion-tui/internal/domain"
	"github.com/ookawayuuto/notion-tui/internal/notion"
	"github.com/ookawayuuto/notion-tui/internal/tui"
	"github.com/spf13/cobra"
)

var alertOnly bool

var shopCmd = &cobra.Command{
	Use:   "shop",
	Short: "買い物リストと在庫管理TUIを起動します",
	Long: `買い物リストと在庫管理（賞味期限管理対応）TUIを起動します。
--alert (-a) フラグを付けると、TUIを起動せずに賞味期限切れ・間近の食品をターミナルに一覧表示（通知）できます。`,
	Run: func(cmd *cobra.Command, args []string) {
		token := os.Getenv("NOTION_TOKEN")
		invID := os.Getenv("NOTION_INVENTORY_DB_ID")
		shopID := os.Getenv("NOTION_SHOPPING_DB_ID")

		if token == "" || invID == "" || shopID == "" {
			fmt.Println("エラー: 環境変数に NOTION_TOKEN, NOTION_INVENTORY_DB_ID, NOTION_SHOPPING_DB_ID を設定してください")
			os.Exit(1)
		}

		client := notion.NewClient(token, invID, shopID)

		if alertOnly {
			runExpirationAlert(client)
			return
		}

		// tuiパッケージの買い物用モデルを初期化
		model := tui.NewShopModel(client)

		p := tea.NewProgram(model)
		if _, err := p.Run(); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	},
}

func runExpirationAlert(client *notion.Client) {
	items, _, err := client.FetchData(context.Background())
	if err != nil {
		fmt.Printf("データ取得エラー: %v\n", err)
		os.Exit(1)
	}

	now := time.Now()
	var expired []domain.InventoryItem
	var soon []domain.InventoryItem

	for _, item := range items {
		info := item.GetExpirationInfo(now)
		if info.Status == domain.ExpirationStatusExpired {
			expired = append(expired, item)
		} else if info.Status == domain.ExpirationStatusExpiringSoon {
			soon = append(soon, item)
		}
	}

	sort.SliceStable(expired, func(i, j int) bool {
		return expired[i].ExpirationDate < expired[j].ExpirationDate
	})
	sort.SliceStable(soon, func(i, j int) bool {
		return soon[i].ExpirationDate < soon[j].ExpirationDate
	})

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	expiredStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196"))
	soonStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))

	fmt.Println(headerStyle.Render("\n📦 【食品・調味料 賞味期限チェック】"))
	fmt.Println("--------------------------------------------------")

	if len(expired) == 0 && len(soon) == 0 {
		fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("✅ 賞味期限切れ・間近の食品はありません。素晴らしい管理状態です！"))
		fmt.Println("--------------------------------------------------")
	} else {
		if len(expired) > 0 {
			fmt.Printf("%s\n", expiredStyle.Render(fmt.Sprintf("🚨 期限切れ (%d件) - 台所の奥底を確認してください:", len(expired))))
			for _, item := range expired {
				info := item.GetExpirationInfo(now)
				var elapsedStr string
				if info.Days <= -365 {
					elapsedStr = fmt.Sprintf("%d年前", (-info.Days)/365)
				} else if info.Days <= -30 {
					elapsedStr = fmt.Sprintf("%dヶ月前", (-info.Days)/30)
				} else {
					elapsedStr = fmt.Sprintf("%d日前", -info.Days)
				}
				fmt.Printf("  • %-16s (期限: %s / %s に超過)\n", item.Name, item.ExpirationDate, elapsedStr)
			}
			fmt.Println()
		}

		if len(soon) > 0 {
			fmt.Printf("%s\n", soonStyle.Render(fmt.Sprintf("⏳ 14日以内に切れる食品 (%d件):", len(soon))))
			for _, item := range soon {
				info := item.GetExpirationInfo(now)
				var remainStr string
				if info.Days == 0 {
					remainStr = "本日期限!"
				} else {
					remainStr = fmt.Sprintf("残り%d日", info.Days)
				}
				fmt.Printf("  • %-16s (期限: %s / %s)\n", item.Name, item.ExpirationDate, remainStr)
			}
			fmt.Println()
		}
		fmt.Println("--------------------------------------------------")
	}

	// 定期便の次回お届け確認
	var subAlerts []domain.InventoryItem
	var subDues []domain.InventoryItem
	var subOverdues []domain.InventoryItem

	for _, item := range items {
		subInfo := item.GetSubscriptionInfo(now)
		switch subInfo.Status {
		case domain.SubscriptionStatusAlert:
			subAlerts = append(subAlerts, item)
		case domain.SubscriptionStatusDue:
			subDues = append(subDues, item)
		case domain.SubscriptionStatusOverdue:
			subOverdues = append(subOverdues, item)
		}
	}

	sort.SliceStable(subAlerts, func(i, j int) bool {
		return subAlerts[i].NextDeliveryDate < subAlerts[j].NextDeliveryDate
	})
	sort.SliceStable(subDues, func(i, j int) bool {
		return subDues[i].NextDeliveryDate < subDues[j].NextDeliveryDate
	})
	sort.SliceStable(subOverdues, func(i, j int) bool {
		return subOverdues[i].NextDeliveryDate < subOverdues[j].NextDeliveryDate
	})

	subHeaderStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("45"))
	subAlertStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("208"))

	fmt.Println(subHeaderStyle.Render("\n🚚 【Amazon定期便 次回お届け確認】"))
	fmt.Println("--------------------------------------------------")

	if len(subAlerts) == 0 && len(subDues) == 0 && len(subOverdues) == 0 {
		fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("✅ 直近（7日以内）に届く予定の定期便はありません。"))
		fmt.Println("--------------------------------------------------")
	} else {
		if len(subAlerts) > 0 {
			fmt.Printf("%s\n", subAlertStyle.Render(fmt.Sprintf("⚡ スキップ検討アラート (7日以内に届く予定 & 在庫あり: %d件):", len(subAlerts))))
			for _, item := range subAlerts {
				subInfo := item.GetSubscriptionInfo(now)
				cycleStr := item.SubCycle
				if cycleStr == "" {
					cycleStr = "未設定"
				}
				fmt.Printf("  • %-16s (次回: %s / あと%d日) [現在在庫: %d個 / サイクル: %s]\n", item.Name, item.NextDeliveryDate, subInfo.Days, item.Stock, cycleStr)
				fmt.Println("    ※ 在庫が残っています。不要な場合はAmazonでスキップしてください (ntui上で 'S' で次回日に更新可能)。")
			}
			fmt.Println()
		}

		if len(subDues) > 0 {
			fmt.Printf("%s\n", lipgloss.NewStyle().Foreground(lipgloss.Color("45")).Bold(true).Render(fmt.Sprintf("📦 もうすぐ到着 (7日以内に届く予定 & 在庫0個: %d件):", len(subDues))))
			for _, item := range subDues {
				subInfo := item.GetSubscriptionInfo(now)
				fmt.Printf("  • %-16s (次回: %s / あと%d日) [現在在庫: 0個]\n", item.Name, item.NextDeliveryDate, subInfo.Days)
			}
			fmt.Println()
		}

		if len(subOverdues) > 0 {
			fmt.Printf("%s\n", lipgloss.NewStyle().Foreground(lipgloss.Color("135")).Bold(true).Render(fmt.Sprintf("📫 配送予定日超過 (受取確認待ち: %d件):", len(subOverdues))))
			for _, item := range subOverdues {
				subInfo := item.GetSubscriptionInfo(now)
				fmt.Printf("  • %-16s (予定日: %s / %d日超過)\n", item.Name, item.NextDeliveryDate, -subInfo.Days)
			}
			fmt.Println()
		}
		fmt.Println("--------------------------------------------------")
	}

	fmt.Println("※ 定期便・賞味期限の確認・更新は 'ntui shop' のTUI上から行えます。")
}


func init() {
	shopCmd.Flags().BoolVarP(&alertOnly, "alert", "a", false, "TUIを起動せず、賞味期限切れ・間近の食品アラートのみを出力します")
	rootCmd.AddCommand(shopCmd)
}
