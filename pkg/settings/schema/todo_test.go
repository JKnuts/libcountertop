package schema

import (
	"slices"
	"testing"
)

func TestMigrateTodoCarriesTheOneListOver(t *testing.T) {
	var todo Todo
	if err := MigrateTodo([]byte(`{"todo":{"entity":" todo.shopping_list "}}`), &todo); err != nil {
		t.Fatal(err)
	}
	if want := []string{"todo.shopping_list"}; !slices.Equal(todo.Entities, want) {
		t.Errorf("entities %v, want %v", todo.Entities, want)
	}
}

func TestMigrateTodoLeavesChosenListsAlone(t *testing.T) {
	todo := Todo{Entities: []string{"todo.hardware"}}
	if err := MigrateTodo([]byte(`{"todo":{"entity":"todo.shopping_list"}}`), &todo); err != nil {
		t.Fatal(err)
	}
	if want := []string{"todo.hardware"}; !slices.Equal(todo.Entities, want) {
		t.Errorf("entities %v, want %v", todo.Entities, want)
	}
}

func TestParseTodoListsDropsBlanksAndRepeats(t *testing.T) {
	got := ParseTodoLists(" todo.a, ,todo.b,todo.a ")
	if want := []string{"todo.a", "todo.b"}; !slices.Equal(got, want) {
		t.Errorf("parsed %v, want %v", got, want)
	}
}
