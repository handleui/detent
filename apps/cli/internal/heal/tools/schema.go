package tools

// Property defines a single property in a JSON schema.
type Property struct {
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
	Default     any      `json:"default,omitempty"`
}

// SchemaBuilder helps construct JSON schemas for tool inputs.
type SchemaBuilder struct {
	properties map[string]any
	required   []string
}

// NewSchema creates a new schema builder.
func NewSchema() *SchemaBuilder {
	return &SchemaBuilder{
		properties: make(map[string]any),
		required:   []string{},
	}
}

// AddString adds a required string property.
func (s *SchemaBuilder) AddString(name, description string) *SchemaBuilder {
	s.properties[name] = Property{
		Type:        "string",
		Description: description,
	}
	s.required = append(s.required, name)
	return s
}

// AddOptionalString adds an optional string property.
func (s *SchemaBuilder) AddOptionalString(name, description string) *SchemaBuilder {
	s.properties[name] = Property{
		Type:        "string",
		Description: description,
	}
	return s
}

// AddInteger adds a required integer property.
func (s *SchemaBuilder) AddInteger(name, description string) *SchemaBuilder {
	s.properties[name] = Property{
		Type:        "integer",
		Description: description,
	}
	s.required = append(s.required, name)
	return s
}

// AddOptionalInteger adds an optional integer property with a default value.
func (s *SchemaBuilder) AddOptionalInteger(name, description string, defaultVal int) *SchemaBuilder {
	s.properties[name] = Property{
		Type:        "integer",
		Description: description,
		Default:     defaultVal,
	}
	return s
}

// AddEnum adds a required enum property.
func (s *SchemaBuilder) AddEnum(name, description string, values []string) *SchemaBuilder {
	s.properties[name] = Property{
		Type:        "string",
		Description: description,
		Enum:        values,
	}
	s.required = append(s.required, name)
	return s
}

// AddOptionalEnum adds an optional enum property.
func (s *SchemaBuilder) AddOptionalEnum(name, description string, values []string) *SchemaBuilder {
	s.properties[name] = Property{
		Type:        "string",
		Description: description,
		Enum:        values,
	}
	return s
}

// Build returns the schema as a map for the Anthropic SDK.
func (s *SchemaBuilder) Build() map[string]any {
	schema := map[string]any{
		"type":       "object",
		"properties": s.properties,
	}
	if len(s.required) > 0 {
		schema["required"] = s.required
	}
	return schema
}
