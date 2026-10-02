package domain

import (
	"testing"
	"time"
)

func TestInventoryItem_GetExpirationInfo(t *testing.T) {
	refTime := time.Date(2026, 9, 16, 12, 0, 0, 0, time.Local)

	tests := []struct {
		name           string
		stock          int
		expirationDate string
		expectedStatus ExpirationStatus
		expectedDays   int
	}{
		{
			name:           "empty date with stock",
			stock:          1,
			expirationDate: "",
			expectedStatus: ExpirationStatusNone,
			expectedDays:   0,
		},
		{
			name:           "invalid format with stock",
			stock:          1,
			expirationDate: "invalid-date",
			expectedStatus: ExpirationStatusNone,
			expectedDays:   0,
		},
		{
			name:           "expired 1 year ago with stock",
			stock:          1,
			expirationDate: "2025-09-16",
			expectedStatus: ExpirationStatusExpired,
			expectedDays:   -365,
		},
		{
			name:           "expired 1 year ago but stock is 0 (should be None)",
			stock:          0,
			expirationDate: "2025-09-16",
			expectedStatus: ExpirationStatusNone,
			expectedDays:   0,
		},
		{
			name:           "expired yesterday with stock",
			stock:          1,
			expirationDate: "2026-09-15",
			expectedStatus: ExpirationStatusExpired,
			expectedDays:   -1,
		},
		{
			name:           "expires today with stock",
			stock:          1,
			expirationDate: "2026-09-16",
			expectedStatus: ExpirationStatusExpiringSoon,
			expectedDays:   0,
		},
		{
			name:           "expires today but stock is 0 (should be None)",
			stock:          0,
			expirationDate: "2026-09-16",
			expectedStatus: ExpirationStatusNone,
			expectedDays:   0,
		},
		{
			name:           "expires in 3 days with stock",
			stock:          1,
			expirationDate: "2026-09-19",
			expectedStatus: ExpirationStatusExpiringSoon,
			expectedDays:   3,
		},
		{
			name:           "expires in 14 days (boundary)",
			stock:          1,
			expirationDate: "2026-09-30",
			expectedStatus: ExpirationStatusExpiringSoon,
			expectedDays:   14,
		},
		{
			name:           "safe (expires in 15 days)",
			stock:          1,
			expirationDate: "2026-10-01",
			expectedStatus: ExpirationStatusSafe,
			expectedDays:   15,
		},
		{
			name:           "safe (expires next year)",
			stock:          2,
			expirationDate: "2027-09-16",
			expectedStatus: ExpirationStatusSafe,
			expectedDays:   365,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := InventoryItem{
				Name:           "Test Item",
				Stock:          tt.stock,
				ExpirationDate: tt.expirationDate,
			}
			info := item.GetExpirationInfo(refTime)
			if info.Status != tt.expectedStatus {
				t.Errorf("expected status %v, got %v", tt.expectedStatus, info.Status)
			}
			if info.Days != tt.expectedDays {
				t.Errorf("expected days %v, got %v", tt.expectedDays, info.Days)
			}
		})
	}
}

func TestInventoryItem_GetSubscriptionInfo(t *testing.T) {
	refTime := time.Date(2026, 9, 22, 12, 0, 0, 0, time.Local)

	tests := []struct {
		name             string
		isSubscription   bool
		nextDeliveryDate string
		stock            int
		expectedStatus   SubscriptionStatus
		expectedDays     int
	}{
		{
			name:             "not subscription",
			isSubscription:   false,
			nextDeliveryDate: "2026-09-25",
			stock:            1,
			expectedStatus:   SubscriptionStatusNone,
			expectedDays:     0,
		},
		{
			name:             "empty delivery date",
			isSubscription:   true,
			nextDeliveryDate: "",
			stock:            1,
			expectedStatus:   SubscriptionStatusNone,
			expectedDays:     0,
		},
		{
			name:             "normal (8 days later)",
			isSubscription:   true,
			nextDeliveryDate: "2026-09-30",
			stock:            1,
			expectedStatus:   SubscriptionStatusNormal,
			expectedDays:     8,
		},
		{
			name:             "alert (7 days later, stock >= 1)",
			isSubscription:   true,
			nextDeliveryDate: "2026-09-29",
			stock:            1,
			expectedStatus:   SubscriptionStatusAlert,
			expectedDays:     7,
		},
		{
			name:             "alert (3 days later, stock >= 1, skip recommendation)",
			isSubscription:   true,
			nextDeliveryDate: "2026-09-25",
			stock:            2,
			expectedStatus:   SubscriptionStatusAlert,
			expectedDays:     3,
		},
		{
			name:             "due (3 days later, stock == 0, waiting delivery)",
			isSubscription:   true,
			nextDeliveryDate: "2026-09-25",
			stock:            0,
			expectedStatus:   SubscriptionStatusDue,
			expectedDays:     3,
		},
		{
			name:             "alert (today, stock >= 1)",
			isSubscription:   true,
			nextDeliveryDate: "2026-09-22",
			stock:            1,
			expectedStatus:   SubscriptionStatusAlert,
			expectedDays:     0,
		},
		{
			name:             "overdue (yesterday)",
			isSubscription:   true,
			nextDeliveryDate: "2026-09-21",
			stock:            1,
			expectedStatus:   SubscriptionStatusOverdue,
			expectedDays:     -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := InventoryItem{
				Name:             "Test Sub Item",
				Stock:            tt.stock,
				IsSubscription:   tt.isSubscription,
				NextDeliveryDate: tt.nextDeliveryDate,
			}
			info := item.GetSubscriptionInfo(refTime)
			if info.Status != tt.expectedStatus {
				t.Errorf("expected status %v, got %v", tt.expectedStatus, info.Status)
			}
			if info.Days != tt.expectedDays {
				t.Errorf("expected days %v, got %v", tt.expectedDays, info.Days)
			}
		})
	}
}

func TestAdvanceDeliveryDate(t *testing.T) {
	tests := []struct {
		currentDate string
		cycle       string
		expected    string
	}{
		{"2026-09-25", "1ヶ月", "2026-10-25"},
		{"2026-09-25", "2ヶ月", "2026-11-25"},
		{"2026-09-25", "3ヶ月", "2026-12-25"},
		{"2026-09-25", "6ヶ月", "2027-03-25"},
		{"2026-09-25", "その他", "2026-10-25"}, // default 1 month
	}

	for _, tt := range tests {
		t.Run(tt.cycle, func(t *testing.T) {
			got := AdvanceDeliveryDate(tt.currentDate, tt.cycle)
			if got != tt.expected {
				t.Errorf("AdvanceDeliveryDate(%s, %s) = %s, want %s", tt.currentDate, tt.cycle, got, tt.expected)
			}
		})
	}
}

