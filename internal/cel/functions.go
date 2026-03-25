package cel

import (
	"strings"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
)

// customFunctions returns all custom CEL function options for the Mantle environment.
func customFunctions() []cel.EnvOption {
	return []cel.EnvOption{
		stringFunctions(),
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
