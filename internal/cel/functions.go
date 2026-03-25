package cel

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/common/types/traits"
)

// customFunctions returns all custom CEL function options for the Mantle environment.
func customFunctions() []cel.EnvOption {
	return []cel.EnvOption{
		stringFunctions(),
		typeFunctions(),
		collectionFunctions(),
		jsonFunctions(),
		timeFunctions(),
	}
}

func stringFunctions() cel.EnvOption {
	return cel.Lib(&stringLib{})
}

type stringLib struct{}

func (l *stringLib) CompileOptions() []cel.EnvOption {
	return []cel.EnvOption{
		cel.Function("toLower",
			cel.MemberOverload("string_toLower",
				[]*cel.Type{cel.StringType},
				cel.StringType,
				cel.UnaryBinding(func(val ref.Val) ref.Val {
					return types.String(strings.ToLower(string(val.(types.String))))
				}),
			),
		),
		cel.Function("toUpper",
			cel.MemberOverload("string_toUpper",
				[]*cel.Type{cel.StringType},
				cel.StringType,
				cel.UnaryBinding(func(val ref.Val) ref.Val {
					return types.String(strings.ToUpper(string(val.(types.String))))
				}),
			),
		),
		cel.Function("trim",
			cel.MemberOverload("string_trim",
				[]*cel.Type{cel.StringType},
				cel.StringType,
				cel.UnaryBinding(func(val ref.Val) ref.Val {
					return types.String(strings.TrimSpace(string(val.(types.String))))
				}),
			),
		),
		cel.Function("replace",
			cel.MemberOverload("string_replace",
				[]*cel.Type{cel.StringType, cel.StringType, cel.StringType},
				cel.StringType,
				cel.FunctionBinding(func(args ...ref.Val) ref.Val {
					s := string(args[0].(types.String))
					old := string(args[1].(types.String))
					newStr := string(args[2].(types.String))
					return types.String(strings.ReplaceAll(s, old, newStr))
				}),
			),
		),
		cel.Function("split",
			cel.MemberOverload("string_split",
				[]*cel.Type{cel.StringType, cel.StringType},
				cel.ListType(cel.StringType),
				cel.BinaryBinding(func(lhs, rhs ref.Val) ref.Val {
					s := string(lhs.(types.String))
					sep := string(rhs.(types.String))
					parts := strings.Split(s, sep)
					return types.DefaultTypeAdapter.NativeToValue(parts)
				}),
			),
		),
	}
}

func (l *stringLib) ProgramOptions() []cel.ProgramOption {
	return nil
}

// ── Type coercion functions ───────────────────────────────────────────────────

func typeFunctions() cel.EnvOption {
	return cel.Lib(&typeLib{})
}

type typeLib struct{}

func (l *typeLib) CompileOptions() []cel.EnvOption {
	return []cel.EnvOption{
		cel.Function("parseInt",
			cel.Overload("parseInt_string",
				[]*cel.Type{cel.StringType},
				cel.IntType,
				cel.UnaryBinding(func(val ref.Val) ref.Val {
					s := string(val.(types.String))
					n, err := strconv.ParseInt(s, 10, 64)
					if err != nil {
						return types.NewErr("parseInt: %v", err)
					}
					return types.Int(n)
				}),
			),
		),
		cel.Function("parseFloat",
			cel.Overload("parseFloat_string",
				[]*cel.Type{cel.StringType},
				cel.DoubleType,
				cel.UnaryBinding(func(val ref.Val) ref.Val {
					s := string(val.(types.String))
					f, err := strconv.ParseFloat(s, 64)
					if err != nil {
						return types.NewErr("parseFloat: %v", err)
					}
					return types.Double(f)
				}),
			),
		),
		cel.Function("toString",
			cel.Overload("toString_any",
				[]*cel.Type{cel.DynType},
				cel.StringType,
				cel.UnaryBinding(func(val ref.Val) ref.Val {
					return types.String(fmt.Sprintf("%v", val.Value()))
				}),
			),
		),
	}
}

func (l *typeLib) ProgramOptions() []cel.ProgramOption {
	return nil
}

// ── Collection functions ──────────────────────────────────────────────────────

func collectionFunctions() cel.EnvOption {
	return cel.Lib(&collectionLib{})
}

type collectionLib struct{}

func (l *collectionLib) CompileOptions() []cel.EnvOption {
	return []cel.EnvOption{
		cel.Function("default",
			cel.Overload("default_any_any",
				[]*cel.Type{cel.DynType, cel.DynType},
				cel.DynType,
				cel.OverloadIsNonStrict(),
				cel.BinaryBinding(func(lhs, rhs ref.Val) ref.Val {
					if types.IsError(lhs) || types.IsUnknown(lhs) || lhs == types.NullValue {
						return rhs
					}
					return lhs
				}),
			),
		),
		cel.Function("flatten",
			cel.Overload("flatten_list",
				[]*cel.Type{cel.ListType(cel.DynType)},
				cel.ListType(cel.DynType),
				cel.UnaryBinding(func(val ref.Val) ref.Val {
					list := val.(traits.Lister)
					var result []any
					it := list.Iterator()
					for it.HasNext() == types.True {
						item := it.Next()
						if sub, ok := item.(traits.Lister); ok {
							subIt := sub.Iterator()
							for subIt.HasNext() == types.True {
								result = append(result, refToNative(subIt.Next()))
							}
						} else {
							result = append(result, refToNative(item))
						}
					}
					return types.DefaultTypeAdapter.NativeToValue(result)
				}),
			),
		),
		// obj() — register fixed-arity overloads for 1–5 key-value pairs.
		// CEL does not support true variadic functions without macros, so we register
		// overloads for each supported arity. All overloads share the same binding
		// via objBinding.
		cel.Function("obj",
			cel.Overload("obj_2",
				[]*cel.Type{cel.DynType, cel.DynType},
				cel.DynType,
				cel.FunctionBinding(objBinding),
			),
			cel.Overload("obj_4",
				[]*cel.Type{cel.DynType, cel.DynType, cel.DynType, cel.DynType},
				cel.DynType,
				cel.FunctionBinding(objBinding),
			),
			cel.Overload("obj_6",
				[]*cel.Type{cel.DynType, cel.DynType, cel.DynType, cel.DynType, cel.DynType, cel.DynType},
				cel.DynType,
				cel.FunctionBinding(objBinding),
			),
			cel.Overload("obj_8",
				[]*cel.Type{cel.DynType, cel.DynType, cel.DynType, cel.DynType, cel.DynType, cel.DynType, cel.DynType, cel.DynType},
				cel.DynType,
				cel.FunctionBinding(objBinding),
			),
			cel.Overload("obj_10",
				[]*cel.Type{cel.DynType, cel.DynType, cel.DynType, cel.DynType, cel.DynType, cel.DynType, cel.DynType, cel.DynType, cel.DynType, cel.DynType},
				cel.DynType,
				cel.FunctionBinding(objBinding),
			),
		),
	}
}

func (l *collectionLib) ProgramOptions() []cel.ProgramOption {
	return nil
}

// objBinding is the shared implementation for all obj() fixed-arity overloads.
func objBinding(args ...ref.Val) ref.Val {
	if len(args)%2 != 0 {
		return types.NewErr("obj: requires even number of arguments (key-value pairs), got %d", len(args))
	}
	m := make(map[string]any, len(args)/2)
	for i := 0; i < len(args); i += 2 {
		key, ok := args[i].(types.String)
		if !ok {
			return types.NewErr("obj: key at position %d must be a string, got %s", i, args[i].Type())
		}
		m[string(key)] = refToNative(args[i+1])
	}
	return types.DefaultTypeAdapter.NativeToValue(m)
}

// ── JSON functions ────────────────────────────────────────────────────────────

func jsonFunctions() cel.EnvOption {
	return cel.Lib(&jsonLib{})
}

type jsonLib struct{}

func (l *jsonLib) CompileOptions() []cel.EnvOption {
	return []cel.EnvOption{
		cel.Function("jsonEncode",
			cel.Overload("jsonEncode_any",
				[]*cel.Type{cel.DynType},
				cel.StringType,
				cel.UnaryBinding(func(val ref.Val) ref.Val {
					native := refToNative(val)
					b, err := json.Marshal(native)
					if err != nil {
						return types.NewErr("jsonEncode: %v", err)
					}
					return types.String(string(b))
				}),
			),
		),
		cel.Function("jsonDecode",
			cel.Overload("jsonDecode_string",
				[]*cel.Type{cel.StringType},
				cel.DynType,
				cel.UnaryBinding(func(val ref.Val) ref.Val {
					s := string(val.(types.String))
					dec := json.NewDecoder(strings.NewReader(s))
					dec.UseNumber()
					var result any
					if err := dec.Decode(&result); err != nil {
						return types.NewErr("jsonDecode: %v", err)
					}
					return types.DefaultTypeAdapter.NativeToValue(normalizeJSONNumbers(result))
				}),
			),
		),
	}
}

func (l *jsonLib) ProgramOptions() []cel.ProgramOption {
	return nil
}

// ── Time functions ────────────────────────────────────────────────────────────

func timeFunctions() cel.EnvOption {
	return cel.Lib(&timeLib{})
}

type timeLib struct{}

func (l *timeLib) CompileOptions() []cel.EnvOption {
	return []cel.EnvOption{
		cel.Function("parseTimestamp",
			cel.Overload("parseTimestamp_string",
				[]*cel.Type{cel.StringType},
				cel.TimestampType,
				cel.UnaryBinding(func(val ref.Val) ref.Val {
					s := string(val.(types.String))
					layouts := []string{
						time.RFC3339,
						time.RFC3339Nano,
						"2006-01-02T15:04:05",
						"2006-01-02",
						"01/02/2006",
						"Jan 2, 2006",
					}
					for _, layout := range layouts {
						if t, err := time.Parse(layout, s); err == nil {
							return types.Timestamp{Time: t}
						}
					}
					return types.NewErr("parseTimestamp: unable to parse %q (tried RFC3339, ISO 8601 date, and common formats)", s)
				}),
			),
		),
		cel.Function("formatTimestamp",
			cel.Overload("formatTimestamp_timestamp_string",
				[]*cel.Type{cel.TimestampType, cel.StringType},
				cel.StringType,
				cel.BinaryBinding(func(lhs, rhs ref.Val) ref.Val {
					ts := lhs.(types.Timestamp)
					layout := string(rhs.(types.String))
					return types.String(ts.Time.Format(layout))
				}),
			),
		),
	}
}

func (l *timeLib) ProgramOptions() []cel.ProgramOption {
	return nil
}

// normalizeJSONNumbers walks a decoded JSON structure and converts json.Number
// values to int64 (if the number is an integer) or float64.
func normalizeJSONNumbers(v any) any {
	switch val := v.(type) {
	case json.Number:
		if i, err := val.Int64(); err == nil {
			return i
		}
		if f, err := val.Float64(); err == nil {
			return f
		}
		return val.String()
	case map[string]any:
		for k, v := range val {
			val[k] = normalizeJSONNumbers(v)
		}
		return val
	case []any:
		for i, v := range val {
			val[i] = normalizeJSONNumbers(v)
		}
		return val
	default:
		return v
	}
}
