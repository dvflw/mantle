package cel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
