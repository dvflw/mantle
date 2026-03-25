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
	}
}

func (l *stringLib) ProgramOptions() []cel.ProgramOption {
	return nil
}
