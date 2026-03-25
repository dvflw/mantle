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
