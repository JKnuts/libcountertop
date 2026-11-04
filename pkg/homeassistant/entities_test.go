package homeassistant

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestEveryDomainWithoutAFilter(t *testing.T) {
	tmpl := Filter{}.listing().page(0)
	if strings.Contains(tmpl, "selectattr") || !strings.Contains(tmpl, "for s in (states) |") {
		t.Errorf("an empty filter should walk every state:\n%s", tmpl)
	}
}

func TestEntitiesWalkInEntityOrder(t *testing.T) {
	tmpl := Filter{Domains: []string{"switch"}}.listing().page(0)
	if !strings.Contains(tmpl, "| sort(attribute='entity_id')") {
		t.Errorf("pages are not sorted by entity id:\n%s", tmpl)
	}
}

func TestAskedAttributesAreInTheTemplate(t *testing.T) {
	if tmpl := (Filter{Attributes: []string{"user_id"}}).listing().page(0); !strings.Contains(
		tmpl,
		`["user_id"]`,
	) {
		t.Errorf("the template does not ask for user_id:\n%s", tmpl)
	}
}

func TestEntitiesSortByAreaThenName(t *testing.T) {
	list := `{"next":null,"items":[{"id":"light.z","name":"zed","area":""},{"id":"light.b","name":"Bee","area":"kitchen"},{"id":"light.a","name":"Ay","area":"Kitchen"},{"id":"light.c","name":"Cee","area":"Attic"}]}`
	got, err := entitiesFrom(t, list)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, e := range got {
		ids = append(ids, e.ID)
	}
	if strings.Join(ids, " ") != "light.c light.a light.b light.z" {
		t.Errorf("order %v", ids)
	}
}

func TestEntitiesKeepAttributes(t *testing.T) {
	got, err := entitiesFrom(
		t,
		`{"next":null,"items":[{"id":"person.ada","name":"Ada","attributes":{"user_id":"u1"}},{"id":"person.bo","name":"Bo","attributes":{}}]}`,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Attributes["user_id"] != "u1" || got[1].Attributes["user_id"] != "" {
		t.Errorf("parsed %+v", got)
	}
}

func TestNoEntitiesIsNotNil(t *testing.T) {
	got, err := entitiesFrom(t, `{"next":null,"items":[]}`)
	if err != nil || got == nil {
		t.Errorf("got %v, %v; want an empty list", got, err)
	}
}

func entitiesFrom(t *testing.T, page string) ([]Entity, error) {
	t.Helper()
	quoted, _ := json.Marshal(page)
	out, err := listAll[Entity](
		context.Background(),
		func(context.Context, string) (json.RawMessage, error) { return quoted, nil },
		Filter{}.listing(),
	)
	if err != nil {
		return nil, err
	}
	sortEntities(out)
	return out, nil
}
