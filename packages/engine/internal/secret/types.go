package secret

import (
	"fmt"
	"sort"
	"strings"
)

// FieldDef defines a credential field with its validation rules.
type FieldDef struct {
	Name     string
	Required bool
}

// CredentialType defines the schema for a credential type.
type CredentialType struct {
	Name              string
	Fields            []FieldDef
	RequireAtLeastOne []string // at least one of these field names must be non-empty
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
	if len(ct.RequireAtLeastOne) > 0 {
		anySet := false
		for _, name := range ct.RequireAtLeastOne {
			if v, ok := data[name]; ok && v != "" {
				anySet = true
				break
			}
		}
		if !anySet {
			return fmt.Errorf("credential type %q requires at least one of: %s",
				ct.Name, strings.Join(ct.RequireAtLeastOne, ", "))
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
	"aws": {
		Name: "aws",
		Fields: []FieldDef{
			{Name: "access_key_id", Required: true},
			{Name: "secret_access_key", Required: true},
			{Name: "region", Required: false},
			{Name: "session_token", Required: false},
		},
	},
	"docker": {
		Name: "docker",
		Fields: []FieldDef{
			{Name: "host", Required: false},
			{Name: "ca_cert", Required: false},
			{Name: "client_cert", Required: false},
			{Name: "client_key", Required: false},
		},
	},
	"git": {
		Name: "git",
		Fields: []FieldDef{
			{Name: "token", Required: false},
			{Name: "ssh_key", Required: false},
			{Name: "username", Required: false},
		},
		// At-least-one validator below guarantees we have auth material.
		RequireAtLeastOne: []string{"token", "ssh_key"},
	},
}

// GetType returns the credential type definition, or an error if unknown.
func GetType(name string) (*CredentialType, error) {
	ct, ok := builtinTypes[name]
	if !ok {
		available := make([]string, 0, len(builtinTypes))
		for k := range builtinTypes {
			available = append(available, k)
		}
		sort.Strings(available)
		return nil, fmt.Errorf("unknown credential type %q (available: %s)", name, strings.Join(available, ", "))
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
