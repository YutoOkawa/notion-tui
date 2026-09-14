package domain

import (
	"time"

	"github.com/jomei/notionapi"
)

// MorningReport は朝のキャッチアップで記録されたレポート情報を表すドメインモデルです。
type MorningReport struct {
	ID        notionapi.ObjectID
	Title     string
	Date      string
	Summary   string
	Tags      []string
	Status    string
	URL       string
	CreatedAt time.Time
	UpdatedAt time.Time
	Content   string // 本文のキャッシュ
}
