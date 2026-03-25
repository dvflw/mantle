package cel

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── normalizeJSONNumbers unit tests ──────────────────────────────────────────

func TestNormalizeJSONNumbers_ExponentFloat(t *testing.T) {
	result := normalizeJSONNumbers(json.Number("1e5"))
	assert.Equal(t, float64(100000), result)
}

func TestNormalizeJSONNumbers_NegativeFloat(t *testing.T) {
	result := normalizeJSONNumbers(json.Number("-3.14"))
	assert.Equal(t, float64(-3.14), result)
}

func TestNormalizeJSONNumbers_IntegerMaxInt64(t *testing.T) {
	// 9223372036854775807 is exactly math.MaxInt64 — must come back as int64.
	result := normalizeJSONNumbers(json.Number("9223372036854775807"))
	assert.Equal(t, int64(9223372036854775807), result)
}

func TestNormalizeJSONNumbers_OverflowInt64PreservedAsString(t *testing.T) {
	// One past MaxInt64 — cannot fit in int64; must preserve as string.
	result := normalizeJSONNumbers(json.Number("9223372036854775808"))
	assert.Equal(t, "9223372036854775808", result)
}

func TestNormalizeJSONNumbers_NestedMap(t *testing.T) {
	input := map[string]any{
		"count": json.Number("42"),
		"ratio": json.Number("0.5"),
		"label": "hello",
	}
	result := normalizeJSONNumbers(input)
	m := result.(map[string]any)
	assert.Equal(t, int64(42), m["count"])
	assert.Equal(t, float64(0.5), m["ratio"])
	assert.Equal(t, "hello", m["label"])
}

func TestNormalizeJSONNumbers_NestedArray(t *testing.T) {
	input := []any{json.Number("1"), json.Number("2.5"), "text"}
	result := normalizeJSONNumbers(input)
	arr := result.([]any)
	assert.Equal(t, int64(1), arr[0])
	assert.Equal(t, float64(2.5), arr[1])
	assert.Equal(t, "text", arr[2])
}

func TestNormalizeJSONNumbers_PassthroughTypes(t *testing.T) {
	// Non-number types should be returned unchanged.
	assert.Equal(t, true, normalizeJSONNumbers(true))
	assert.Equal(t, "hello", normalizeJSONNumbers("hello"))
	assert.Nil(t, normalizeJSONNumbers(nil))
}

func TestFunc_ToLower(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)
	tests := []struct {
		name string
		expr string
		want any
	}{
		{"basic", `"HELLO".toLower()`, "hello"},
		{"mixed", `"Hello World".toLower()`, "hello world"},
		{"already_lower", `"hello".toLower()`, "hello"},
		{"empty", `"".toLower()`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := eval.Eval(tt.expr, newTestContext())
			require.NoError(t, err)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestFunc_ToUpper(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)
	tests := []struct {
		name string
		expr string
		want any
	}{
		{"basic", `"hello".toUpper()`, "HELLO"},
		{"mixed", `"Hello World".toUpper()`, "HELLO WORLD"},
		{"empty", `"".toUpper()`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := eval.Eval(tt.expr, newTestContext())
			require.NoError(t, err)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestFunc_Trim(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)
	tests := []struct {
		name string
		expr string
		want any
	}{
		{"spaces", `"  hello  ".trim()`, "hello"},
		{"tabs", "\"\\thello\\t\".trim()", "hello"},
		{"no_whitespace", `"hello".trim()`, "hello"},
		{"empty", `"".trim()`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := eval.Eval(tt.expr, newTestContext())
			require.NoError(t, err)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestFunc_Replace(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)
	tests := []struct {
		name string
		expr string
		want any
	}{
		{"basic", `"hello world".replace("world", "CEL")`, "hello CEL"},
		{"multiple", `"aabbaa".replace("aa", "x")`, "xbbx"},
		{"no_match", `"hello".replace("xyz", "abc")`, "hello"},
		{"empty_replacement", `"hello world".replace("world", "")`, "hello "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := eval.Eval(tt.expr, newTestContext())
			require.NoError(t, err)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestFunc_Split(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)
	tests := []struct {
		name string
		expr string
		want any
	}{
		{"comma", `"a,b,c".split(",")`, []any{"a", "b", "c"}},
		{"space", `"hello world foo".split(" ")`, []any{"hello", "world", "foo"}},
		{"no_match", `"hello".split(",")`, []any{"hello"}},
		{"empty", `"".split(",")`, []any{""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := eval.Eval(tt.expr, newTestContext())
			require.NoError(t, err)
			assert.Equal(t, tt.want, result)
		})
	}
}

// Task 4: parseInt, parseFloat, toString

func TestFunc_ParseInt(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)
	tests := []struct {
		name    string
		expr    string
		want    any
		wantErr bool
	}{
		{"basic", `parseInt("42")`, int64(42), false},
		{"negative", `parseInt("-7")`, int64(-7), false},
		{"zero", `parseInt("0")`, int64(0), false},
		{"invalid", `parseInt("abc")`, nil, true},
		{"float_string", `parseInt("3.14")`, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := eval.Eval(tt.expr, newTestContext())
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, result)
			}
		})
	}
}

func TestFunc_ParseFloat(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)
	tests := []struct {
		name    string
		expr    string
		want    any
		wantErr bool
	}{
		{"basic", `parseFloat("3.14")`, float64(3.14), false},
		{"integer_string", `parseFloat("42")`, float64(42), false},
		{"negative", `parseFloat("-1.5")`, float64(-1.5), false},
		{"invalid", `parseFloat("abc")`, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := eval.Eval(tt.expr, newTestContext())
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, result)
			}
		})
	}
}

func TestFunc_ToString(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)
	tests := []struct {
		name string
		expr string
		want any
	}{
		{"int", `toString(42)`, "42"},
		{"bool", `toString(true)`, "true"},
		{"string", `toString("hello")`, "hello"},
		{"float", `toString(1.5)`, "1.5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := eval.Eval(tt.expr, newTestContext())
			require.NoError(t, err)
			assert.Equal(t, tt.want, result)
		})
	}
}

// Task 5: obj()

func TestFunc_Obj(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)
	tests := []struct {
		name    string
		expr    string
		want    any
		wantErr bool
	}{
		{
			name: "basic map",
			expr: `obj("name", "alice", "age", 30)`,
			want: map[string]any{"name": "alice", "age": int64(30)},
		},
		{
			name: "single pair",
			expr: `obj("key", "value")`,
			want: map[string]any{"key": "value"},
		},
		{
			name: "nested with step reference",
			expr: `obj("status", steps.fetch.output.status)`,
			want: map[string]any{"status": int64(200)},
		},
		{
			name:    "odd_args",
			expr:    `obj("key")`,
			wantErr: true,
		},
		{
			"non_string_key",
			`obj(1, "value")`,
			nil,
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := eval.Eval(tt.expr, newTestContext())
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, result)
			}
		})
	}
}

// Task 6: default() and flatten()

func TestFunc_Default(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)
	tests := []struct {
		name    string
		expr    string
		want    any
		wantErr bool
	}{
		{"value exists returns value", `default("hello", "fallback")`, "hello", false},
		{"empty string returns empty string", `default("", "fallback")`, "", false},
		{"null returns fallback", `default(null, "fallback")`, "fallback", false},
		{"non-null int unchanged", `default(42, 0)`, int64(42), false},
		{
			"missing_key_returns_fallback",
			`default(steps.fetch.output.missing, "fallback")`,
			"fallback",
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := eval.Eval(tt.expr, newTestContext())
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, result)
			}
		})
	}
}

func TestFunc_Flatten(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)
	tests := []struct {
		name string
		expr string
		want any
	}{
		{
			name: "nested lists → flat list",
			expr: `flatten([[1, 2], [3, 4], [5]])`,
			want: []any{int64(1), int64(2), int64(3), int64(4), int64(5)},
		},
		{
			name: "mixed nested and non-nested",
			expr: `flatten([[1], [3, 4]])`,
			want: []any{int64(1), int64(3), int64(4)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := eval.Eval(tt.expr, newTestContext())
			require.NoError(t, err)
			assert.Equal(t, tt.want, result)
		})
	}

	// Empty list: flatten([]) — must return a non-nil empty []any.
	t.Run("empty list", func(t *testing.T) {
		ctx := newTestContext()
		ctx.Inputs["empty"] = []any{}
		result, err := eval.Eval(`flatten(inputs.empty)`, ctx)
		require.NoError(t, err)
		list, ok := result.([]any)
		require.True(t, ok, "expected []any, got %T", result)
		assert.Empty(t, list)
	})
}

// Task 7: jsonEncode and jsonDecode

func TestFunc_JsonEncode(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)

	result, err := eval.Eval(`jsonEncode(obj("name", "alice", "score", 99))`, newTestContext())
	require.NoError(t, err)

	s, ok := result.(string)
	require.True(t, ok, "expected string result, got %T", result)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(s), &parsed))
	assert.Equal(t, "alice", parsed["name"])
	assert.Equal(t, float64(99), parsed["score"])
}

func TestFunc_JsonDecode(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)
	tests := []struct {
		name    string
		expr    string
		wantErr bool
		check   func(t *testing.T, result any)
	}{
		{
			name: "object",
			expr: `jsonDecode("{\"name\":\"bob\",\"age\":25}")`,
			check: func(t *testing.T, result any) {
				m, ok := result.(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "bob", m["name"])
				assert.Equal(t, int64(25), m["age"])
			},
		},
		{
			name: "array",
			expr: `jsonDecode("[1,2,3]")`,
			check: func(t *testing.T, result any) {
				assert.NotNil(t, result)
			},
		},
		{
			name:    "invalid",
			expr:    `jsonDecode("not json")`,
			wantErr: true,
		},
		{
			name:    "trailing_data",
			expr:    `jsonDecode("{} {}")`,
			wantErr: true,
		},
		{
			name:    "trailing_bracket",
			expr:    `jsonDecode("{}]")`,
			wantErr: true,
		},
		{
			name:    "trailing_brace",
			expr:    `jsonDecode("1}")`,
			wantErr: true,
		},
		{
			name: "large_integer_preserved",
			expr: `jsonDecode("9223372036854775808")`,
			check: func(t *testing.T, result any) {
				// int64 max is 9223372036854775807 — this overflows.
				// Should be preserved as string, not silently converted to float64.
				s, ok := result.(string)
				require.True(t, ok, "expected string for overflow int, got %T", result)
				assert.Equal(t, "9223372036854775808", s)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := eval.Eval(tt.expr, newTestContext())
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.check != nil {
					tt.check(t, result)
				}
			}
		})
	}
}

// Task 8: timestamp and formatTimestamp

func TestFunc_Timestamp(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)
	tests := []struct {
		name    string
		expr    string
		wantErr bool
	}{
		{"iso8601", `parseTimestamp("2024-01-15T00:00:00Z")`, false},
		{"with_offset", `parseTimestamp("2024-06-01T12:30:00+05:30")`, false},
		{"invalid", `parseTimestamp("not-a-date")`, true},
		{"date_only", `parseTimestamp("2026-03-24")`, false},
		{"us_date", `parseTimestamp("03/24/2026")`, false},
		{"rfc3339nano", `parseTimestamp("2026-03-24T19:00:00.123456789Z")`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := eval.Eval(tt.expr, newTestContext())
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}

func TestFunc_FormatTimestamp(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)
	tests := []struct {
		name string
		expr string
		want any
	}{
		{
			name: "date format",
			expr: `formatTimestamp(parseTimestamp("2024-01-15T00:00:00Z"), "2006-01-02")`,
			want: "2024-01-15",
		},
		{
			name: "time format",
			expr: `formatTimestamp(parseTimestamp("2024-01-15T14:30:00Z"), "15:04")`,
			want: "14:30",
		},
		{
			name: "named month format",
			expr: `formatTimestamp(parseTimestamp("2024-01-15T00:00:00Z"), "Jan 2, 2006")`,
			want: "Jan 15, 2024",
		},
		{
			name: "rfc3339 roundtrip",
			expr: `formatTimestamp(parseTimestamp("2026-03-24T00:00:00Z"), "2006-01-02T15:04:05Z07:00")`,
			want: "2026-03-24T00:00:00Z",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := eval.Eval(tt.expr, newTestContext())
			require.NoError(t, err)
			assert.Equal(t, tt.want, result)
		})
	}
}

// ── Additional boundary and regression tests ──────────────────────────────────

func TestFunc_Obj_MaxArity(t *testing.T) {
	// obj() supports up to 5 key-value pairs (10 args). Verify all registered
	// overloads (2, 4, 6, 8, 10 args) produce the correct maps.
	eval, err := NewEvaluator()
	require.NoError(t, err)

	t.Run("three pairs (6 args)", func(t *testing.T) {
		result, err := eval.Eval(`obj("a", 1, "b", 2, "c", 3)`, newTestContext())
		require.NoError(t, err)
		assert.Equal(t, map[string]any{"a": int64(1), "b": int64(2), "c": int64(3)}, result)
	})

	t.Run("four pairs (8 args)", func(t *testing.T) {
		result, err := eval.Eval(`obj("a", 1, "b", 2, "c", 3, "d", 4)`, newTestContext())
		require.NoError(t, err)
		assert.Equal(t, map[string]any{"a": int64(1), "b": int64(2), "c": int64(3), "d": int64(4)}, result)
	})

	t.Run("five pairs (10 args — max arity)", func(t *testing.T) {
		result, err := eval.Eval(`obj("a", 1, "b", 2, "c", 3, "d", 4, "e", 5)`, newTestContext())
		require.NoError(t, err)
		assert.Equal(t, map[string]any{
			"a": int64(1), "b": int64(2), "c": int64(3),
			"d": int64(4), "e": int64(5),
		}, result)
	})
}

func TestFunc_Flatten_MixedScalarAndSublist(t *testing.T) {
	// The flatten implementation passes non-list elements through unchanged.
	// A list like [1, [2, 3]] should produce [1, 2, 3].
	eval, err := NewEvaluator()
	require.NoError(t, err)

	ctx := newTestContext()
	ctx.Inputs["mixed"] = []any{int64(1), []any{int64(2), int64(3)}}
	result, err := eval.Eval(`flatten(inputs.mixed)`, ctx)
	require.NoError(t, err)
	assert.Equal(t, []any{int64(1), int64(2), int64(3)}, result)
}

func TestFunc_Default_FalsyButNonNull(t *testing.T) {
	// false and 0 are falsy but are NOT null — default() must return them unchanged.
	eval, err := NewEvaluator()
	require.NoError(t, err)

	t.Run("false returns false not fallback", func(t *testing.T) {
		result, err := eval.Eval(`default(false, true)`, newTestContext())
		require.NoError(t, err)
		assert.Equal(t, false, result)
	})

	t.Run("zero returns zero not fallback", func(t *testing.T) {
		result, err := eval.Eval(`default(0, 99)`, newTestContext())
		require.NoError(t, err)
		assert.Equal(t, int64(0), result)
	})

	t.Run("empty string returns empty string not fallback", func(t *testing.T) {
		result, err := eval.Eval(`default("", "fallback")`, newTestContext())
		require.NoError(t, err)
		assert.Equal(t, "", result)
	})
}

func TestFunc_Split_EmptySeparator(t *testing.T) {
	// strings.Split("abc", "") returns ["a", "b", "c"] — each character.
	eval, err := NewEvaluator()
	require.NoError(t, err)

	result, err := eval.Eval(`"abc".split("")`, newTestContext())
	require.NoError(t, err)
	assert.Equal(t, []any{"a", "b", "c"}, result)
}

func TestFunc_Replace_EmptyOldString(t *testing.T) {
	// strings.ReplaceAll("ab", "", "X") inserts X between and around each char.
	eval, err := NewEvaluator()
	require.NoError(t, err)

	result, err := eval.Eval(`"ab".replace("", "X")`, newTestContext())
	require.NoError(t, err)
	assert.Equal(t, "XaXbX", result)
}

func TestFunc_ParseInt_WhitespaceIsInvalid(t *testing.T) {
	// strconv.ParseInt is strict — " 42" (leading space) must fail.
	eval, err := NewEvaluator()
	require.NoError(t, err)

	_, err = eval.Eval(`parseInt(" 42")`, newTestContext())
	require.Error(t, err)
}

func TestFunc_JsonDecode_FloatValue(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)

	result, err := eval.Eval(`jsonDecode("3.14")`, newTestContext())
	require.NoError(t, err)
	assert.Equal(t, float64(3.14), result)
}

func TestFunc_JsonDecode_BoolValue(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)

	result, err := eval.Eval(`jsonDecode("true")`, newTestContext())
	require.NoError(t, err)
	assert.Equal(t, true, result)
}

func TestFunc_JsonDecode_ArrayOfIntegers(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)

	result, err := eval.Eval(`jsonDecode("[1, 2, 3]")`, newTestContext())
	require.NoError(t, err)

	arr, ok := result.([]any)
	require.True(t, ok, "expected []any, got %T", result)
	assert.Equal(t, []any{int64(1), int64(2), int64(3)}, arr)
}

func TestFunc_JsonDecode_NestedObject(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)

	result, err := eval.Eval(`jsonDecode("{\"a\":{\"b\":42}}")`, newTestContext())
	require.NoError(t, err)

	m, ok := result.(map[string]any)
	require.True(t, ok)
	inner, ok := m["a"].(map[string]any)
	require.True(t, ok, "expected inner map")
	assert.Equal(t, int64(42), inner["b"])
}

func TestFunc_JsonEncode_List(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)

	result, err := eval.Eval(`jsonEncode(["a", "b", "c"])`, newTestContext())
	require.NoError(t, err)

	s, ok := result.(string)
	require.True(t, ok)
	var arr []string
	require.NoError(t, json.Unmarshal([]byte(s), &arr))
	assert.Equal(t, []string{"a", "b", "c"}, arr)
}

func TestFunc_JsonEncode_Primitive(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)

	t.Run("integer", func(t *testing.T) {
		result, err := eval.Eval(`jsonEncode(42)`, newTestContext())
		require.NoError(t, err)
		assert.Equal(t, "42", result)
	})

	t.Run("boolean", func(t *testing.T) {
		result, err := eval.Eval(`jsonEncode(true)`, newTestContext())
		require.NoError(t, err)
		assert.Equal(t, "true", result)
	})

	t.Run("string", func(t *testing.T) {
		result, err := eval.Eval(`jsonEncode("hello")`, newTestContext())
		require.NoError(t, err)
		assert.Equal(t, `"hello"`, result)
	})
}

func TestFunc_ParseTimestamp_NamedMonthFormat(t *testing.T) {
	// "Jan 2, 2006" is one of the supported layouts in parseTimestamp.
	eval, err := NewEvaluator()
	require.NoError(t, err)

	result, err := eval.Eval(`formatTimestamp(parseTimestamp("Mar 15, 2025"), "2006-01-02")`, newTestContext())
	require.NoError(t, err)
	assert.Equal(t, "2025-03-15", result)
}

func TestFunc_StringChaining(t *testing.T) {
	// Verify that string methods can be chained.
	eval, err := NewEvaluator()
	require.NoError(t, err)

	result, err := eval.Eval(`"  HELLO WORLD  ".trim().toLower()`, newTestContext())
	require.NoError(t, err)
	assert.Equal(t, "hello world", result)
}

func TestFunc_ResolveString_WithCustomFunction(t *testing.T) {
	// Verify that custom functions work when embedded in {{ }} template strings.
	eval, err := NewEvaluator()
	require.NoError(t, err)

	result, err := eval.ResolveString(`tag:{{ "PRODUCTION".toLower() }}`, newTestContext())
	require.NoError(t, err)
	assert.Equal(t, "tag:production", result)
}

func TestFunc_JsonRoundtrip(t *testing.T) {
	// jsonEncode followed by jsonDecode must preserve the original value.
	eval, err := NewEvaluator()
	require.NoError(t, err)

	result, err := eval.Eval(`jsonDecode(jsonEncode(obj("x", 1, "y", "hello")))`, newTestContext())
	require.NoError(t, err)

	m, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, int64(1), m["x"])
	assert.Equal(t, "hello", m["y"])
}