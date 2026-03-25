package cel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newListContext() *Context {
	return &Context{
		Steps: map[string]map[string]any{
			"fetch": {
				"output": map[string]any{
					"items": []any{
						map[string]any{"name": "alice", "age": int64(30)},
						map[string]any{"name": "bob", "age": int64(17)},
						map[string]any{"name": "charlie", "age": int64(25)},
					},
				},
			},
		},
		Inputs: map[string]any{},
	}
}

func TestMacro_Map(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)

	result, err := eval.Eval(`steps.fetch.output.items.map(item, item.name)`, newListContext())
	require.NoError(t, err)
	assert.Equal(t, []any{"alice", "bob", "charlie"}, result)
}

func TestMacro_Filter(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)

	result, err := eval.Eval(`steps.fetch.output.items.filter(item, item.age >= 21)`, newListContext())
	require.NoError(t, err)

	items, ok := result.([]any)
	require.True(t, ok)
	assert.Len(t, items, 2)
}

func TestMacro_Exists(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)

	result, err := eval.Eval(`steps.fetch.output.items.exists(item, item.name == "bob")`, newListContext())
	require.NoError(t, err)
	assert.Equal(t, true, result)

	result, err = eval.Eval(`steps.fetch.output.items.exists(item, item.name == "dave")`, newListContext())
	require.NoError(t, err)
	assert.Equal(t, false, result)
}

func TestMacro_All(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)

	result, err := eval.Eval(`steps.fetch.output.items.all(item, item.age > 0)`, newListContext())
	require.NoError(t, err)
	assert.Equal(t, true, result)

	result, err = eval.Eval(`steps.fetch.output.items.all(item, item.age >= 21)`, newListContext())
	require.NoError(t, err)
	assert.Equal(t, false, result)
}

func TestMacro_ExistsOne(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)

	result, err := eval.Eval(`steps.fetch.output.items.exists_one(item, item.name == "alice")`, newListContext())
	require.NoError(t, err)
	assert.Equal(t, true, result)
}

func TestMacro_MapAndFilter_Chained(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)

	result, err := eval.Eval(`steps.fetch.output.items.filter(item, item.age >= 21).map(item, item.name)`, newListContext())
	require.NoError(t, err)
	assert.Equal(t, []any{"alice", "charlie"}, result)
}

func TestMacro_ExistsOne_NoMatch(t *testing.T) {
	// exists_one must return false when NO element satisfies the predicate.
	eval, err := NewEvaluator()
	require.NoError(t, err)

	result, err := eval.Eval(`steps.fetch.output.items.exists_one(item, item.name == "dave")`, newListContext())
	require.NoError(t, err)
	assert.Equal(t, false, result)
}

func TestMacro_ExistsOne_MultipleMatches(t *testing.T) {
	// exists_one must return false when MORE THAN ONE element satisfies the predicate.
	eval, err := NewEvaluator()
	require.NoError(t, err)

	// alice (30) and charlie (25) are both >= 21.
	result, err := eval.Eval(`steps.fetch.output.items.exists_one(item, item.age >= 21)`, newListContext())
	require.NoError(t, err)
	assert.Equal(t, false, result)
}

func TestMacro_Filter_NoMatches(t *testing.T) {
	// filter that matches nothing returns an empty list, not an error.
	eval, err := NewEvaluator()
	require.NoError(t, err)

	result, err := eval.Eval(`steps.fetch.output.items.filter(item, item.age > 100)`, newListContext())
	require.NoError(t, err)

	items, ok := result.([]any)
	require.True(t, ok, "expected []any, got %T", result)
	assert.Empty(t, items)
}

func TestMacro_Map_WithCustomFunction(t *testing.T) {
	// map() can use custom functions (toLower) inside the transform expression.
	eval, err := NewEvaluator()
	require.NoError(t, err)

	result, err := eval.Eval(`steps.fetch.output.items.map(item, item.name.toUpper())`, newListContext())
	require.NoError(t, err)
	assert.Equal(t, []any{"ALICE", "BOB", "CHARLIE"}, result)
}

func TestMacro_Map_WithObj(t *testing.T) {
	// map() combined with obj() should reshape each element into a new map.
	// Fields on the resulting objects must be accessible via CEL field access.
	eval, err := NewEvaluator()
	require.NoError(t, err)

	// Verify correct result count.
	size, err := eval.Eval(
		`size(steps.fetch.output.items.filter(item, item.age >= 21).map(item, obj("display_name", item.name, "years", item.age)))`,
		newListContext(),
	)
	require.NoError(t, err)
	assert.Equal(t, int64(2), size)

	// Verify the first element's fields are accessible by index within CEL.
	name, err := eval.Eval(
		`steps.fetch.output.items.filter(item, item.age >= 21).map(item, obj("display_name", item.name, "years", item.age))[0].display_name`,
		newListContext(),
	)
	require.NoError(t, err)
	assert.Equal(t, "alice", name)

	years, err := eval.Eval(
		`steps.fetch.output.items.filter(item, item.age >= 21).map(item, obj("display_name", item.name, "years", item.age))[0].years`,
		newListContext(),
	)
	require.NoError(t, err)
	assert.Equal(t, int64(30), years)
}

func TestMacro_All_EmptyList(t *testing.T) {
	// all() on an empty list is vacuously true.
	eval, err := NewEvaluator()
	require.NoError(t, err)

	ctx := newListContext()
	ctx.Steps["fetch"]["output"].(map[string]any)["items"] = []any{}
	result, err := eval.Eval(`steps.fetch.output.items.all(item, false)`, ctx)
	require.NoError(t, err)
	assert.Equal(t, true, result)
}

func TestMacro_Exists_EmptyList(t *testing.T) {
	// exists() on an empty list returns false.
	eval, err := NewEvaluator()
	require.NoError(t, err)

	ctx := newListContext()
	ctx.Steps["fetch"]["output"].(map[string]any)["items"] = []any{}
	result, err := eval.Eval(`steps.fetch.output.items.exists(item, true)`, ctx)
	require.NoError(t, err)
	assert.Equal(t, false, result)
}