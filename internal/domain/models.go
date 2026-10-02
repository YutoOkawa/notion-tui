package domain

import (
	"time"

	"github.com/jomei/notionapi"
)

type ExpirationStatus int

const (
	ExpirationStatusNone ExpirationStatus = iota
	ExpirationStatusSafe
	ExpirationStatusExpiringSoon
	ExpirationStatusExpired
)

type InventoryItem struct {
	ID               notionapi.ObjectID
	Name             string
	Stock            int
	Categories       []string
	ExpirationDate   string // "YYYY-MM-DD"
	IsSubscription   bool   // 定期便対象か
	SubCycle         string // 配送サイクル ("1ヶ月", "2ヶ月", ...)
	NextDeliveryDate string // 次回配送予定日 "YYYY-MM-DD"
}

type SubscriptionStatus int

const (
	SubscriptionStatusNone SubscriptionStatus = iota
	SubscriptionStatusNormal                  // 通常 (次回配送まで8日以上)
	SubscriptionStatusAlert                   // スキップ検討アラート (7日以内 & 在庫1個以上)
	SubscriptionStatusDue                     // もうすぐ届く (7日以内 & 在庫0個)
	SubscriptionStatusOverdue                 // 配送予定日超過 (届いたはず / 受取待ち)
)

type SubscriptionInfo struct {
	Status SubscriptionStatus
	Days   int
}

// GetSubscriptionInfo は指定された基準日（通常は現在日）に基づいて定期便の状態を判定します。
func (item InventoryItem) GetSubscriptionInfo(now time.Time) SubscriptionInfo {
	if !item.IsSubscription || item.NextDeliveryDate == "" {
		return SubscriptionInfo{Status: SubscriptionStatusNone, Days: 0}
	}

	targetDate, err := time.Parse("2006-01-02", item.NextDeliveryDate)
	if err != nil {
		return SubscriptionInfo{Status: SubscriptionStatusNone, Days: 0}
	}

	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	target := time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), 0, 0, 0, 0, now.Location())

	diffDays := int(target.Sub(today).Hours() / 24)

	if diffDays < 0 {
		return SubscriptionInfo{Status: SubscriptionStatusOverdue, Days: diffDays}
	}
	if diffDays <= 7 {
		if item.Stock >= 1 {
			return SubscriptionInfo{Status: SubscriptionStatusAlert, Days: diffDays}
		}
		return SubscriptionInfo{Status: SubscriptionStatusDue, Days: diffDays}
	}
	return SubscriptionInfo{Status: SubscriptionStatusNormal, Days: diffDays}
}

// AdvanceDeliveryDate は指定された基準日（空なら現在日）に配送サイクル分の月数を加算した日付文字列(YYYY-MM-DD)を返します。
func AdvanceDeliveryDate(currentDate string, cycle string) string {
	var base time.Time
	if currentDate != "" {
		parsed, err := time.Parse("2006-01-02", currentDate)
		if err == nil {
			base = parsed
		}
	}
	if base.IsZero() {
		base = time.Now()
	}

	months := 1
	switch cycle {
	case "2ヶ月":
		months = 2
	case "3ヶ月":
		months = 3
	case "4ヶ月":
		months = 4
	case "5ヶ月":
		months = 5
	case "6ヶ月":
		months = 6
	default:
		months = 1
	}

	next := base.AddDate(0, months, 0)
	return next.Format("2006-01-02")
}

// ExpirationInfo は賞味期限の状態と基準日からの残り日数を表します。
type ExpirationInfo struct {
	Status ExpirationStatus
	Days   int
}

// GetExpirationInfo は指定された基準日（通常は現在日）に基づいて賞味期限の状態を計算します。
func (item InventoryItem) GetExpirationInfo(now time.Time) ExpirationInfo {
	// 在庫が0以下の場合は手元にストックが存在しないため賞味期限の判定・アラート対象外とする
	if item.Stock <= 0 || item.ExpirationDate == "" {
		return ExpirationInfo{Status: ExpirationStatusNone, Days: 0}
	}

	targetDate, err := time.Parse("2006-01-02", item.ExpirationDate)
	if err != nil {
		return ExpirationInfo{Status: ExpirationStatusNone, Days: 0}
	}

	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	target := time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), 0, 0, 0, 0, now.Location())

	diffDays := int(target.Sub(today).Hours() / 24)

	if diffDays < 0 {
		return ExpirationInfo{Status: ExpirationStatusExpired, Days: diffDays}
	}
	if diffDays <= 14 {
		return ExpirationInfo{Status: ExpirationStatusExpiringSoon, Days: diffDays}
	}
	return ExpirationInfo{Status: ExpirationStatusSafe, Days: diffDays}
}

type ShoppingItem struct {
	ID   notionapi.ObjectID
	Name string
}
