package notion

import (
	"context"
	"fmt"

	"github.com/jomei/notionapi"
	"github.com/ookawayuuto/notion-tui/internal/domain"
)

type MonstClient struct {
	api        *notionapi.Client
	monsterDB  notionapi.DatabaseID
	wakuwakuDB notionapi.DatabaseID
}

func NewMonstClient(token, monsterID, wakuwakuID string) *MonstClient {
	return &MonstClient{
		api:        notionapi.NewClient(notionapi.Token(token)),
		monsterDB:  notionapi.DatabaseID(monsterID),
		wakuwakuDB: notionapi.DatabaseID(wakuwakuID),
	}
}

func (c *MonstClient) FetchWakuwaku(ctx context.Context) ([]domain.Wakuwaku, map[notionapi.ObjectID]domain.Wakuwaku, error) {
	hasMore := true
	cursor := notionapi.Cursor("")

	var list []domain.Wakuwaku
	dict := make(map[notionapi.ObjectID]domain.Wakuwaku)

	for hasMore {
		req := &notionapi.DatabaseQueryRequest{
			StartCursor: cursor,
			PageSize:    100,
		}
		if cursor == "" {
			req.StartCursor = ""
		}

		res, err := c.api.Database.Query(ctx, c.wakuwakuDB, req)
		if err != nil {
			return nil, nil, fmt.Errorf("わくわくの実DBエラー: %w", err)
		}

		for _, page := range res.Results {
			name := extractTitle(page, "名前", "Name", "title")
			w := domain.Wakuwaku{
				ID:   notionapi.ObjectID(page.ID),
				Name: name,
			}
			list = append(list, w)
			dict[w.ID] = w
		}

		hasMore = res.HasMore
		cursor = res.NextCursor
	}

	return list, dict, nil
}

func (c *MonstClient) FetchMonsters(ctx context.Context, attribute string) ([]domain.Monster, error) {
	var filter notionapi.Filter
	if attribute != "" && attribute != "全て" {
		filter = &notionapi.PropertyFilter{
			Property: "属性",
			Select: &notionapi.SelectFilterCondition{
				Equals: attribute,
			},
		}
	}

	hasMore := true
	cursor := notionapi.Cursor("")
	var monsters []domain.Monster

	for hasMore {
		req := &notionapi.DatabaseQueryRequest{
			Filter:      filter,
			StartCursor: cursor,
			PageSize:    100,
		}
		if cursor == "" {
			req.StartCursor = ""
		}

		res, err := c.api.Database.Query(ctx, c.monsterDB, req)
		if err != nil {
			return nil, fmt.Errorf("モンスターDBエラー: %w", err)
		}

		for _, page := range res.Results {
			name := extractTitle(page, "モンスター名", "title", "名前")
			attr := extractSelect(page, "属性")
			priority := extractSelect(page, "優先度")
			has := extractCheckbox(page, "所持")

			monsters = append(monsters, domain.Monster{
				ID:        notionapi.ObjectID(page.ID),
				Name:      name,
				Attribute: attr,
				Priority:  priority,
				Has:       has,
				AccountA:  extractRelations(page, "アカウントA"),
				AccountB:  extractRelations(page, "アカウントB"),
				AccountC:  extractRelations(page, "アカウントC"),
				AccountD:  extractRelations(page, "アカウントD"),
				AccountA2: extractRelations(page, "アカウントA-2"),
				AccountB2: extractRelations(page, "アカウントB-2"),
			})
		}
		hasMore = res.HasMore
		cursor = res.NextCursor
	}

	return monsters, nil
}

func (c *MonstClient) UpdateMonsterRelations(ctx context.Context, monsterID notionapi.ObjectID, accountKey string, wakuwakuIDs []notionapi.ObjectID) error {
	relations := make([]notionapi.Relation, len(wakuwakuIDs))
	for i, id := range wakuwakuIDs {
		relations[i] = notionapi.Relation{ID: notionapi.PageID(id)}
	}

	props := notionapi.Properties{
		accountKey: notionapi.RelationProperty{Relation: relations},
	}

	_, err := c.api.Page.Update(ctx, notionapi.PageID(monsterID), &notionapi.PageUpdateRequest{
		Properties: props,
	})
	return err
}

func extractSelect(page notionapi.Page, key string) string {
	if prop, ok := page.Properties[key].(*notionapi.SelectProperty); ok && prop.Select.Name != "" {
		return prop.Select.Name
	}
	return ""
}

func extractCheckbox(page notionapi.Page, key string) bool {
	if prop, ok := page.Properties[key].(*notionapi.CheckboxProperty); ok {
		return prop.Checkbox
	}
	return false
}

func extractRelations(page notionapi.Page, key string) []notionapi.ObjectID {
	var ids []notionapi.ObjectID
	if prop, ok := page.Properties[key].(*notionapi.RelationProperty); ok {
		for _, rel := range prop.Relation {
			ids = append(ids, notionapi.ObjectID(rel.ID))
		}
	}
	return ids
}
