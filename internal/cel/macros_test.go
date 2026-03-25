package cel

import (
	"encoding/json"
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

// ── Additional macro tests ────────────────────────────────────────────────────

func TestMacro_Map_EmptyList(t *testing.T) {
	// map() on an empty list must return an empty list, not an error.
	eval, err := NewEvaluator()
	require.NoError(t, err)

	ctx := newListContext()
	ctx.Steps["fetch"]["output"].(map[string]any)["items"] = []any{}
	result, err := eval.Eval(`steps.fetch.output.items.map(item, item.name)`, ctx)
	require.NoError(t, err)

	items, ok := result.([]any)
	require.True(t, ok, "expected []any, got %T", result)
	assert.Empty(t, items)
}

func TestMacro_Map_StringConcatenation(t *testing.T) {
	// map() with a string concatenation expression inside.
	eval, err := NewEvaluator()
	require.NoError(t, err)

	result, err := eval.Eval(
		`steps.fetch.output.items.map(item, item.name + " (" + toString(item.age) + ")")`,
		newListContext(),
	)
	require.NoError(t, err)
	assert.Equal(t, []any{"alice (30)", "bob (17)", "charlie (25)"}, result)
}

func TestMacro_Filter_StringFieldWithToLower(t *testing.T) {
	// filter() on a string field using toLower() inside the predicate.
	eval, err := NewEvaluator()
	require.NoError(t, err)

	ctx := newListContext()
	// Add an item with an uppercase name.
	ctx.Steps["fetch"]["output"].(map[string]any)["items"] = append(
		ctx.Steps["fetch"]["output"].(map[string]any)["items"].([]any),
		map[string]any{"name": "ALICE_EXTRA", "age": int64(22)},
	)

	result, err := eval.Eval(
		`steps.fetch.output.items.filter(item, item.name.toLower().startsWith("alice"))`,
		ctx,
	)
	require.NoError(t, err)

	items, ok := result.([]any)
	require.True(t, ok)
	// Both "alice" and "ALICE_EXTRA" start with "alice" when lowercased.
	assert.Len(t, items, 2)
}

func TestMacro_Filter_PreservesOrder(t *testing.T) {
	// filter() must preserve the original list order of matching items.
	eval, err := NewEvaluator()
	require.NoError(t, err)

	result, err := eval.Eval(
		`steps.fetch.output.items.filter(item, item.age >= 21).map(item, item.name)`,
		newListContext(),
	)
	require.NoError(t, err)
	// alice (30) comes before charlie (25) in the original list.
	assert.Equal(t, []any{"alice", "charlie"}, result)
}

func TestMacro_All_FalseShortCircuits(t *testing.T) {
	// all() should return false as soon as one element fails the predicate.
	eval, err := NewEvaluator()
	require.NoError(t, err)

	// bob is age 17, which is < 18, so all(item, item.age >= 18) must be false.
	result, err := eval.Eval(
		`steps.fetch.output.items.all(item, item.age >= 18)`,
		newListContext(),
	)
	require.NoError(t, err)
	assert.Equal(t, false, result)
}

func TestMacro_ExistsOne_EmptyList(t *testing.T) {
	// exists_one() on an empty list must return false.
	eval, err := NewEvaluator()
	require.NoError(t, err)

	ctx := newListContext()
	ctx.Steps["fetch"]["output"].(map[string]any)["items"] = []any{}
	result, err := eval.Eval(`steps.fetch.output.items.exists_one(item, true)`, ctx)
	require.NoError(t, err)
	assert.Equal(t, false, result)
}

func TestMacro_Map_WithJsonEncode(t *testing.T) {
	// map() combined with jsonEncode() should produce JSON-encoded strings for each element.
	eval, err := NewEvaluator()
	require.NoError(t, err)

	result, err := eval.Eval(
		`steps.fetch.output.items.map(item, jsonEncode(obj("n", item.name, "a", item.age)))`,
		newListContext(),
	)
	require.NoError(t, err)

	arr, ok := result.([]any)
	require.True(t, ok, "expected []any, got %T", result)
	require.Len(t, arr, 3)

	// Each element should be a valid JSON string.
	for _, elem := range arr {
		s, ok := elem.(string)
		require.True(t, ok, "expected string element, got %T", elem)
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(s), &m), "element must be valid JSON: %s", s)
	}
}

func TestMacro_Filter_WithDefault(t *testing.T) {
	// filter() using default() inside the predicate to handle optional fields.
	eval, err := NewEvaluator()
	require.NoError(t, err)

	ctx := newListContext()
	// Add an item that is missing the "age" field (simulated by adding extra items).
	ctx.Steps["fetch"]["output"].(map[string]any)["items"] = []any{
		map[string]any{"name": "alice", "age": int64(30)},
		map[string]any{"name": "unknown"}, // no age field
		map[string]any{"name": "charlie", "age": int64(25)},
	}

	// Use default() to treat a missing age as 0.
	result, err := eval.Eval(
		`steps.fetch.output.items.filter(item, default(item.age, 0) >= 21).map(item, item.name)`,
		ctx,
	)
	require.NoError(t, err)
	assert.Equal(t, []any{"alice", "charlie"}, result)
}

func TestMacro_Map_WithSplit(t *testing.T) {
	// map() using split() to transform a comma-separated string field.
	eval, err := NewEvaluator()
	require.NoError(t, err)

	ctx := &Context{
		Steps: map[string]map[string]any{
			"fetch": {
				"output": map[string]any{
					"tags_list": []any{
						map[string]any{"tags": "go,cel,test"},
						map[string]any{"tags": "integration,unit"},
					},
				},
			},
		},
		Inputs: map[string]any{},
	}

	result, err := eval.Eval(
		`steps.fetch.output.tags_list.map(item, size(item.tags.split(",")))`,
		ctx,
	)
	require.NoError(t, err)
	assert.Equal(t, []any{int64(3), int64(2)}, result)
}