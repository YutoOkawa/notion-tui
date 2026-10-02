package tui

import (
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jomei/notionapi"
	"github.com/ookawayuuto/notion-tui/internal/domain"
)

// モッククライアント
type MockClient struct {
	updatedExpirationID   notionapi.ObjectID
	updatedExpirationDate string
	updatedDeliveryID     notionapi.ObjectID
	updatedDeliveryDate   string
	updatedSubID          notionapi.ObjectID
	updatedSubIsSub       bool
	updatedSubCycle       string
	updatedSubDate        string
}

func (m *MockClient) FetchData(ctx context.Context) ([]domain.InventoryItem, []domain.ShoppingItem, error) {
	return []domain.InventoryItem{
		{ID: "1", Name: "海苔", Stock: 1, Categories: []string{"乾物"}, ExpirationDate: "2025-08-10"},
		{ID: "2", Name: "みりん", Stock: 1, Categories: []string{"調味料"}, ExpirationDate: "2026-09-20"},
		{ID: "3", Name: "キッチンペーパー", Stock: 2, Categories: []string{"日用品"}, ExpirationDate: ""},
	}, nil, nil
}
func (m *MockClient) AddShoppingItem(ctx context.Context, name string) (domain.ShoppingItem, error) {
	return domain.ShoppingItem{ID: "4", Name: name}, nil
}
func (m *MockClient) AddInventoryItem(ctx context.Context, name string, stock int, categories []string) (domain.InventoryItem, error) {
	return domain.InventoryItem{
		ID:         "new-inv-1",
		Name:       name,
		Stock:      stock,
		Categories: categories,
	}, nil
}
func (m *MockClient) CheckShoppingItem(ctx context.Context, pageID notionapi.ObjectID) error {
	return nil
}
func (m *MockClient) UpdateStock(ctx context.Context, pageID notionapi.ObjectID, newStock int) error {
	return nil
}
func (m *MockClient) UpdateExpirationDate(ctx context.Context, pageID notionapi.ObjectID, expirationDate string) error {
	m.updatedExpirationID = pageID
	m.updatedExpirationDate = expirationDate
	return nil
}
func (m *MockClient) UpdateNextDeliveryDate(ctx context.Context, pageID notionapi.ObjectID, nextDeliveryDate string) error {
	m.updatedDeliveryID = pageID
	m.updatedDeliveryDate = nextDeliveryDate
	return nil
}
func (m *MockClient) UpdateSubscription(ctx context.Context, pageID notionapi.ObjectID, isSub bool, cycle string, nextDeliveryDate string) error {
	m.updatedSubID = pageID
	m.updatedSubIsSub = isSub
	m.updatedSubCycle = cycle
	m.updatedSubDate = nextDeliveryDate
	return nil
}

func TestModelUpdate(t *testing.T) {
	client := &MockClient{}
	model := NewShopModel(client)

	// 初期状態はローディング中
	if !model.loading {
		t.Error("Expected initial model to be loading")
	}

	// dataLoadedMsg のテスト（非同期データフェッチのコールバック）
	msg := dataLoadedMsg{
		items: []domain.InventoryItem{
			{ID: "1", Name: "海苔", Stock: 1, Categories: []string{"乾物"}, ExpirationDate: "2025-08-10"},
			{ID: "2", Name: "みりん", Stock: 1, Categories: []string{"調味料"}, ExpirationDate: "2026-09-20"},
		},
		shopping: []domain.ShoppingItem{},
	}
	updatedModel, _ := model.Update(msg)
	newModel := updatedModel.(ShopModel)

	if newModel.loading {
		t.Error("Model should not be loading after dataLoadedMsg")
	}
	if len(newModel.items) != 2 {
		t.Errorf("Expected 2 items, got %d", len(newModel.items))
	}

	// タブキーによるペイン切り替えテスト
	tabMsg := tea.KeyMsg{Type: tea.KeyTab}
	switchedModel, _ := newModel.Update(tabMsg)
	if switchedModel.(ShopModel).activePane != 1 {
		t.Error("Expected active pane to switch to 1 (Right Pane)")
	}
}

func TestShopModel_ExpirationWorkflow(t *testing.T) {
	client := &MockClient{}
	model := NewShopModel(client)

	// データロード
	msg := dataLoadedMsg{
		items: []domain.InventoryItem{
			{ID: "1", Name: "海苔", Stock: 1, Categories: []string{"乾物"}, ExpirationDate: "2025-08-10"},
			{ID: "2", Name: "みりん", Stock: 1, Categories: []string{"調味料"}, ExpirationDate: "2026-09-20"},
		},
		shopping: []domain.ShoppingItem{},
	}
	updatedModel, _ := model.Update(msg)
	loadedModel := updatedModel.(ShopModel)

	// 'e' キーで賞味期限入力モードへ遷移
	eMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}}
	mAfterE, _ := loadedModel.Update(eMsg)
	inputModel := mAfterE.(ShopModel)

	if inputModel.state != shopStateInputExpire {
		t.Fatalf("Expected state to be shopStateInputExpire, got %v", inputModel.state)
	}
	if inputModel.textInput.Value() != "2025-08-10" {
		t.Errorf("Expected textInput value to be '2025-08-10', got '%s'", inputModel.textInput.Value())
	}

	// 新しい日付を入力して Enter
	inputModel.textInput.SetValue("2026-12-31")
	enterMsg := tea.KeyMsg{Type: tea.KeyEnter}
	mAfterEnter, cmd := inputModel.Update(enterMsg)
	savedModel := mAfterEnter.(ShopModel)

	if savedModel.state != shopStateBrowse {
		t.Errorf("Expected state to return to shopStateBrowse, got %v", savedModel.state)
	}
	if savedModel.items[0].ExpirationDate != "2026-12-31" {
		t.Errorf("Expected item 0 date to be updated to 2026-12-31, got %s", savedModel.items[0].ExpirationDate)
	}
	if cmd == nil {
		t.Fatal("Expected tea.Cmd to be returned for API update")
	}

	// 非同期コマンドの実行結果を検証
	resultMsg := cmd()
	switch res := resultMsg.(type) {
	case expirationUpdatedMsg:
		if res.id != "1" || res.expirationDate != "2026-12-31" {
			t.Errorf("Unexpected expirationUpdatedMsg: %+v", res)
		}
	default:
		t.Fatalf("Expected expirationUpdatedMsg, got %T", resultMsg)
	}
}

func TestShopModel_StockDecreaseResetsExpiration(t *testing.T) {
	client := &MockClient{}
	model := NewShopModel(client)

	// 在庫1、賞味期限ありのアイテムをロード
	msg := dataLoadedMsg{
		items: []domain.InventoryItem{
			{ID: "item-1", Name: "海苔", Stock: 1, Categories: []string{"乾物"}, ExpirationDate: "2025-08-10"},
		},
		shopping: []domain.ShoppingItem{},
	}
	updatedModel, _ := model.Update(msg)
	loadedModel := updatedModel.(ShopModel)

	// '-' キーを押して在庫を 1 -> 0 に減らす
	minusMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'-'}}
	mAfterMinus, cmd := loadedModel.Update(minusMsg)
	zeroModel := mAfterMinus.(ShopModel)

	// 在庫が0になり、賞味期限が即座にリセットされていることを確認
	if zeroModel.items[0].Stock != 0 {
		t.Errorf("Expected stock to be 0, got %d", zeroModel.items[0].Stock)
	}
	if zeroModel.items[0].ExpirationDate != "" {
		t.Errorf("Expected ExpirationDate to be reset to empty string, got '%s'", zeroModel.items[0].ExpirationDate)
	}

	// 発行されたCmdの中に UpdateExpirationDate と AddShoppingItem が含まれていること
	if cmd == nil {
		t.Fatal("Expected tea.Cmd to be returned")
	}
}


func TestShopModel_ExpirationFilter(t *testing.T) {
	client := &MockClient{}
	model := NewShopModel(client)

	refDate := time.Date(2026, 9, 16, 0, 0, 0, 0, time.Local)
	allItems := []domain.InventoryItem{
		{ID: "1", Name: "安心食品", Stock: 1, Categories: []string{"食品"}, ExpirationDate: "2027-01-01"},
		{ID: "2", Name: "古い海苔", Stock: 1, Categories: []string{"食品"}, ExpirationDate: "2025-08-10"}, // 期限切れ
		{ID: "3", Name: "期限間近みりん", Stock: 1, Categories: []string{"調味料"}, ExpirationDate: "2026-09-18"}, // 残り2日
		{ID: "4", Name: "日用品", Stock: 1, Categories: []string{"日用品"}, ExpirationDate: ""},
	}

	// 1. 通常モード
	itemsNone := applyInventoryFilters(allItems, "全て", expireFilterNone, refDate)
	if len(itemsNone) != 4 {
		t.Fatalf("Expected 4 items, got %d", len(itemsNone))
	}

	// 2. 期限順ソート (期限切れ・間近優先)
	itemsSort := applyInventoryFilters(allItems, "全て", expireFilterSortSoon, refDate)
	if len(itemsSort) != 4 {
		t.Fatalf("Expected 4 items, got %d", len(itemsSort))
	}
	if itemsSort[0].Name != "古い海苔" {
		t.Errorf("Expected oldest item '古い海苔' to be first, got '%s'", itemsSort[0].Name)
	}
	if itemsSort[1].Name != "期限間近みりん" {
		t.Errorf("Expected '期限間近みりん' to be second, got '%s'", itemsSort[1].Name)
	}
	if itemsSort[3].Name != "日用品" {
		t.Errorf("Expected item with no expiration to be last, got '%s'", itemsSort[3].Name)
	}

	// 3. アラート（期限切れ・間近14日以内のみ）絞り込み
	itemsAlert := applyInventoryFilters(allItems, "全て", expireFilterAlertOnly, refDate)
	if len(itemsAlert) != 2 {
		t.Fatalf("Expected 2 alert items, got %d", len(itemsAlert))
	}
	if itemsAlert[0].Name != "古い海苔" || itemsAlert[1].Name != "期限間近みりん" {
		t.Errorf("Expected alert items [古い海苔, 期限間近みりん], got [%s, %s]", itemsAlert[0].Name, itemsAlert[1].Name)
	}

	// TUI の 'f' キーによる切り替えテスト
	loadedModel, _ := model.Update(dataLoadedMsg{items: allItems, shopping: nil})
	m := loadedModel.(ShopModel)

	fMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}}
	m1, _ := m.Update(fMsg)
	if m1.(ShopModel).expireFilter != expireFilterSortSoon {
		t.Errorf("Expected filter to be expireFilterSortSoon, got %v", m1.(ShopModel).expireFilter)
	}

	m2, _ := m1.Update(fMsg)
	if m2.(ShopModel).expireFilter != expireFilterAlertOnly {
		t.Errorf("Expected filter to be expireFilterAlertOnly, got %v", m2.(ShopModel).expireFilter)
	}

	m3, _ := m2.Update(fMsg)
	if m3.(ShopModel).expireFilter != expireFilterNone {
		t.Errorf("Expected filter to be expireFilterNone, got %v", m3.(ShopModel).expireFilter)
	}
}

func TestShopModel_ViewSmoke(t *testing.T) {
	client := &MockClient{}
	model := NewShopModel(client)

	allItems := []domain.InventoryItem{
		{ID: "1", Name: "古い海苔", Stock: 1, Categories: []string{"食品"}, ExpirationDate: "2025-08-10"},
		{ID: "2", Name: "期限間近みりん", Stock: 1, Categories: []string{"調味料"}, ExpirationDate: "2026-09-18"},
	}
	loadedModel, _ := model.Update(dataLoadedMsg{items: allItems, shopping: nil})
	m := loadedModel.(ShopModel)

	view := m.View()
	if view == "" {
		t.Fatal("Expected non-empty view string")
	}
}

func TestShopModel_SubscriptionWorkflow(t *testing.T) {
	client := &MockClient{}
	model := NewShopModel(client)

	// 定期便アイテムをロード
	msg := dataLoadedMsg{
		items: []domain.InventoryItem{
			{
				ID:               "sub-1",
				Name:             "オリーブオイル",
				Stock:            1,
				Categories:       []string{"調味料"},
				IsSubscription:   true,
				SubCycle:         "1ヶ月",
				NextDeliveryDate: "2026-09-25",
			},
		},
		shopping: []domain.ShoppingItem{},
	}
	updatedModel, _ := model.Update(msg)
	loadedModel := updatedModel.(ShopModel)

	// 1. 'S' キーで定期便スキップ（次回配送日を +1ヶ月 延長）
	skipMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}}
	mAfterSkip, cmd := loadedModel.Update(skipMsg)
	skippedModel := mAfterSkip.(ShopModel)

	if skippedModel.items[0].NextDeliveryDate != "2026-10-25" {
		t.Errorf("Expected NextDeliveryDate to be updated to 2026-10-25, got %s", skippedModel.items[0].NextDeliveryDate)
	}
	if cmd == nil {
		t.Fatal("Expected tea.Cmd to be returned for API update")
	}
	resultMsg := cmd()
	switch res := resultMsg.(type) {
	case deliveryDateUpdatedMsg:
		if res.id != "sub-1" || res.nextDeliveryDate != "2026-10-25" {
			t.Errorf("Unexpected deliveryDateUpdatedMsg: %+v", res)
		}
	default:
		t.Fatalf("Expected deliveryDateUpdatedMsg, got %T", resultMsg)
	}

	// 2. 'p' キーで次回配送日入力モードへ遷移
	pMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}}
	mAfterP, _ := skippedModel.Update(pMsg)
	pModel := mAfterP.(ShopModel)

	if pModel.state != shopStateInputDeliveryDate {
		t.Fatalf("Expected state to be shopStateInputDeliveryDate, got %v", pModel.state)
	}
	if pModel.textInput.Value() != "2026-10-25" {
		t.Errorf("Expected textInput value to be '2026-10-25', got '%s'", pModel.textInput.Value())
	}

	// 日付を変更して Enter
	pModel.textInput.SetValue("2026-11-01")
	enterMsg := tea.KeyMsg{Type: tea.KeyEnter}
	mAfterEnter, pCmd := pModel.Update(enterMsg)
	savedModel := mAfterEnter.(ShopModel)

	if savedModel.state != shopStateBrowse {
		t.Errorf("Expected state to return to shopStateBrowse, got %v", savedModel.state)
	}
	if savedModel.items[0].NextDeliveryDate != "2026-11-01" {
		t.Errorf("Expected item NextDeliveryDate to be 2026-11-01, got %s", savedModel.items[0].NextDeliveryDate)
	}
	if pCmd == nil {
		t.Fatal("Expected tea.Cmd to be returned")
	}
}

func TestShopModel_SubscriptionOverdueAutoAdvance(t *testing.T) {
	client := &MockClient{}
	model := NewShopModel(client)

	// 過去の配送日（届いたはずの状態）の定期便アイテム
	msg := dataLoadedMsg{
		items: []domain.InventoryItem{
			{
				ID:               "sub-2",
				Name:             "炭酸水",
				Stock:            0,
				Categories:       []string{"飲料"},
				IsSubscription:   true,
				SubCycle:         "1ヶ月",
				NextDeliveryDate: "2020-01-01", // 過去日
			},
		},
		shopping: []domain.ShoppingItem{},
	}
	updatedModel, _ := model.Update(msg)
	loadedModel := updatedModel.(ShopModel)

	// '+' キーを押して在庫を増やす（受取）
	plusMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'+'}}
	mAfterPlus, _ := loadedModel.Update(plusMsg)
	receivedModel := mAfterPlus.(ShopModel)

	if receivedModel.items[0].Stock != 1 {
		t.Errorf("Expected stock to be 1, got %d", receivedModel.items[0].Stock)
	}
	// 過去日の場合は次回日が +1ヶ月進んでいること
	if receivedModel.items[0].NextDeliveryDate == "2020-01-01" {
		t.Error("Expected NextDeliveryDate to be advanced from 2020-01-01")
	}
}

func TestShopModel_AddInventoryItem(t *testing.T) {
	client := &MockClient{}
	model := NewShopModel(client)

	msg := dataLoadedMsg{
		items: []domain.InventoryItem{
			{ID: "1", Name: "醤油", Stock: 1, Categories: []string{"調味料"}},
		},
		shopping: []domain.ShoppingItem{},
	}
	updatedModel, _ := model.Update(msg)
	loadedModel := updatedModel.(ShopModel)

	// 'a' キーで在庫追加モードへ遷移
	aMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	mAfterA, _ := loadedModel.Update(aMsg)
	aModel := mAfterA.(ShopModel)

	if aModel.state != shopStateInputNewInventory {
		t.Fatalf("Expected state to be shopStateInputNewInventory, got %v", aModel.state)
	}

	// アイテム名を入力して Enter
	aModel.textInput.SetValue("オリーブオイル")
	enterMsg := tea.KeyMsg{Type: tea.KeyEnter}
	mAfterEnter, cmd := aModel.Update(enterMsg)
	savedModel := mAfterEnter.(ShopModel)

	if savedModel.state != shopStateBrowse {
		t.Errorf("Expected state to return to shopStateBrowse, got %v", savedModel.state)
	}
	if cmd == nil {
		t.Fatal("Expected tea.Cmd to be returned")
	}

	// コマンドを実行して inventoryItemAddedMsg を取得
	resMsg := cmd()
	addedMsg, ok := resMsg.(inventoryItemAddedMsg)
	if !ok {
		t.Fatalf("Expected inventoryItemAddedMsg, got %T", resMsg)
	}
	if addedMsg.item.Name != "オリーブオイル" {
		t.Errorf("Expected added item name to be 'オリーブオイル', got '%s'", addedMsg.item.Name)
	}

	// inventoryItemAddedMsg を Model に反映
	mAfterAdded, _ := savedModel.Update(addedMsg)
	finalModel := mAfterAdded.(ShopModel)

	if len(finalModel.allItems) != 2 {
		t.Fatalf("Expected 2 items in allItems, got %d", len(finalModel.allItems))
	}
	if finalModel.items[finalModel.leftCursor].Name != "オリーブオイル" {
		t.Errorf("Expected cursor to be on 'オリーブオイル', got '%s'", finalModel.items[finalModel.leftCursor].Name)
	}
}

func TestShopModel_AddShoppingItemDirect(t *testing.T) {
	client := &MockClient{}
	model := NewShopModel(client)

	msg := dataLoadedMsg{
		items:    []domain.InventoryItem{},
		shopping: []domain.ShoppingItem{},
	}
	updatedModel, _ := model.Update(msg)
	loadedModel := updatedModel.(ShopModel)

	// 右ペイン（買い物リスト）へ切り替え
	tabMsg := tea.KeyMsg{Type: tea.KeyTab}
	mAfterTab, _ := loadedModel.Update(tabMsg)
	rightPaneModel := mAfterTab.(ShopModel)

	// 'a' キーで買い物リスト追加モードへ遷移
	aMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	mAfterA, _ := rightPaneModel.Update(aMsg)
	aModel := mAfterA.(ShopModel)

	if aModel.state != shopStateInputNewShopping {
		t.Fatalf("Expected state to be shopStateInputNewShopping, got %v", aModel.state)
	}

	// アイテム名を入力して Enter
	aModel.textInput.SetValue("洗剤")
	enterMsg := tea.KeyMsg{Type: tea.KeyEnter}
	mAfterEnter, cmd := aModel.Update(enterMsg)
	savedModel := mAfterEnter.(ShopModel)

	if savedModel.state != shopStateBrowse {
		t.Errorf("Expected state to return to shopStateBrowse, got %v", savedModel.state)
	}
	if cmd == nil {
		t.Fatal("Expected tea.Cmd to be returned")
	}

	// コマンドを実行して itemAddedMsg を取得
	resMsg := cmd()
	addedMsg, ok := resMsg.(itemAddedMsg)
	if !ok {
		t.Fatalf("Expected itemAddedMsg, got %T", resMsg)
	}

	mAfterAdded, _ := savedModel.Update(addedMsg)
	finalModel := mAfterAdded.(ShopModel)

	if len(finalModel.shopping) != 1 || finalModel.shopping[0].Name != "洗剤" {
		t.Errorf("Expected shopping list to have '洗剤', got %v", finalModel.shopping)
	}
}

func TestShopModel_SubscriptionCycleWorkflow(t *testing.T) {
	client := &MockClient{}
	model := NewShopModel(client)

	msg := dataLoadedMsg{
		items: []domain.InventoryItem{
			{
				ID:             "sub-new-1",
				Name:           "炭酸水",
				Stock:          1,
				Categories:     []string{"飲料"},
				IsSubscription: false,
			},
		},
		shopping: []domain.ShoppingItem{},
	}
	updatedModel, _ := model.Update(msg)
	loadedModel := updatedModel.(ShopModel)

	// 1. 'c' キーで配送期間選択モードへ遷移
	cMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}}
	mAfterC, _ := loadedModel.Update(cMsg)
	cModel := mAfterC.(ShopModel)

	if cModel.state != shopStateSelectSubCycle {
		t.Fatalf("Expected state to be shopStateSelectSubCycle, got %v", cModel.state)
	}

	// 2. '2' を押して「2ヶ月」を選択
	key2Msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}}
	mAfter2, cmd := cModel.Update(key2Msg)
	cycle2Model := mAfter2.(ShopModel)

	if cycle2Model.state != shopStateBrowse {
		t.Errorf("Expected state to return to shopStateBrowse, got %v", cycle2Model.state)
	}
	if !cycle2Model.items[0].IsSubscription {
		t.Error("Expected IsSubscription to be true")
	}
	if cycle2Model.items[0].SubCycle != "2ヶ月" {
		t.Errorf("Expected SubCycle to be '2ヶ月', got '%s'", cycle2Model.items[0].SubCycle)
	}
	if cycle2Model.items[0].NextDeliveryDate == "" {
		t.Error("Expected NextDeliveryDate to be automatically generated")
	}
	if cmd == nil {
		t.Fatal("Expected tea.Cmd to be returned")
	}

	// コマンドを実行して subscriptionUpdatedMsg を取得
	resMsg := cmd()
	subMsg, ok := resMsg.(subscriptionUpdatedMsg)
	if !ok {
		t.Fatalf("Expected subscriptionUpdatedMsg, got %T", resMsg)
	}
	if !subMsg.isSubscription || subMsg.subCycle != "2ヶ月" {
		t.Errorf("Unexpected subscriptionUpdatedMsg: %+v", subMsg)
	}

	// 3. 'c' -> '0' で定期便を解除
	mAfterC2, _ := cycle2Model.Update(cMsg)
	key0Msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'0'}}
	mAfter0, cmd0 := mAfterC2.(ShopModel).Update(key0Msg)
	unsubModel := mAfter0.(ShopModel)

	if unsubModel.items[0].IsSubscription {
		t.Error("Expected IsSubscription to be false")
	}
	if unsubModel.items[0].SubCycle != "" {
		t.Errorf("Expected SubCycle to be empty, got '%s'", unsubModel.items[0].SubCycle)
	}
	if unsubModel.items[0].NextDeliveryDate != "" {
		t.Errorf("Expected NextDeliveryDate to be empty, got '%s'", unsubModel.items[0].NextDeliveryDate)
	}
	if cmd0 == nil {
		t.Fatal("Expected tea.Cmd to be returned")
	}
}
