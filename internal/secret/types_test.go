package secret

import "testing"

func TestCredentialType_Validate(t *testing.T) {
	ct, err := GetType("openai")
	if err != nil {
		t.Fatalf("GetType() error: %v", err)
	}

	// Valid.
	err = ct.Validate(map[string]string{"api_key": "sk-123"})
	if err != nil {
		t.Errorf("Validate() with api_key should pass, got: %v", err)
	}

	// Valid with optional field.
	err = ct.Validate(map[string]string{"api_key": "sk-123", "org_id": "org-1"})
	if err != nil {
		t.Errorf("Validate() with all fields should pass, got: %v", err)
	}

	// Missing required field.
	err = ct.Validate(map[string]string{"org_id": "org-1"})
	if err == nil {
		t.Error("Validate() without api_key should fail")
	}

	// Empty required field.
	err = ct.Validate(map[string]string{"api_key": ""})
	if err == nil {
		t.Error("Validate() with empty api_key should fail")
	}
}

func TestGetType_Unknown(t *testing.T) {
	_, err := GetType("nonexistent")
	if err == nil {
		t.Error("GetType() with unknown type should fail")
	}
}

func TestGetType_AllBuiltins(t *testing.T) {
	for _, name := range []string{"generic", "bearer", "openai", "basic"} {
		ct, err := GetType(name)
		if err != nil {
			t.Errorf("GetType(%q) error: %v", name, err)
		}
		if ct.Name != name {
			t.Errorf("type name = %q, want %q", ct.Name, name)
		}
	}
}

func TestCredentialType_Docker(t *testing.T) {
	ct, err := GetType("docker")
	if err != nil {
		t.Fatalf("GetType('docker'): %v", err)
	}
	if ct.Name != "docker" {
		t.Errorf("name = %q, want 'docker'", ct.Name)
	}

	// All fields are optional — empty data should be valid.
	if err := ct.Validate(map[string]string{}); err != nil {
		t.Errorf("empty data should be valid: %v", err)
	}

	// Full TLS config should also be valid.
	if err := ct.Validate(map[string]string{
		"host":        "tcp://docker:2376",
		"ca_cert":     "ca-data",
		"client_cert": "cert-data",
		"client_key":  "key-data",
	}); err != nil {
		t.Errorf("full TLS data should be valid: %v", err)
	}
}
