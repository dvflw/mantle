package cel

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
)

// customFunctions returns all custom CEL function options for the Mantle environment.
func customFunctions() []cel.EnvOption {
	return []cel.EnvOption{
		stringFunctions(),
		typeFunctions(),
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
