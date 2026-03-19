package secret

import "fmt"

// FieldDef defines a credential field with its validation rules.
type FieldDef struct {
	Name     string
	Required bool
}

// CredentialType defines the schema for a credential type.
type CredentialType struct {
	Name   string
	Fields []FieldDef
}

// Validate checks that all required fields are present in the data map.
func (ct *CredentialType) Validate(data map[string]string) error {
	for _, f := range ct.Fields {
		if f.Required {
			v, ok := data[f.Name]
			if !ok || v == "" {
				return fmt.Errorf("field %q is required for credential type %q", f.Name, ct.Name)
			}
		}
	}
	return nil
}

// FieldNames returns the list of field names for this type.
func (ct *CredentialType) FieldNames() []string {
	names := make([]string, len(ct.Fields))
	for i, f := range ct.Fields {
		names[i] = f.Name
	}
	return names
}

// built-in credential types.
var builtinTypes = map[string]*CredentialType{
	"generic": {
		Name: "generic",
		Fields: []FieldDef{
			{Name: "key", Required: true},
		},
	},
	"bearer": {
		Name: "bearer",
		Fields: []FieldDef{
			{Name: "token", Required: true},
		},
	},
	"openai": {
		Name: "openai",
		Fields: []FieldDef{
			{Name: "api_key", Required: true},
			{Name: "org_id", Required: false},
		},
	},
	"basic": {
		Name: "basic",
		Fields: []FieldDef{
			{Name: "username", Required: true},
			{Name: "password", Required: true},
		},
	},
}

// GetType returns the credential type definition, or an error if unknown.
func GetType(name string) (*CredentialType, error) {
	ct, ok := builtinTypes[name]
	if !ok {
		return nil, fmt.Errorf("unknown credential type %q (available: generic, bearer, openai, basic)", name)
	}
	return ct, nil
}

// ListTypes returns all available credential type names.
func ListTypes() []string {
	names := make([]string, 0, len(builtinTypes))
	for name := range builtinTypes {
		names = append(names, name)
	}
	return names
}
