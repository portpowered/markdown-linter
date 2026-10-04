package contract

import (
	"encoding/json"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// validateJSON never loads a schema URL: every resource comes from the installed registry.
func validateJSON(schema map[string]any, value any) error {
	normalize := func(v any) (any, error) {
		data, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		var raw any
		err = json.Unmarshal(data, &raw)
		return raw, err
	}
	document, err := normalize(schema)
	if err != nil {
		return err
	}
	instance, err := normalize(value)
	if err != nil {
		return err
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("urn:marklint:inline-schema", document); err != nil {
		return err
	}
	compiled, err := compiler.Compile("urn:marklint:inline-schema")
	if err != nil {
		return fmt.Errorf("installed schema: %w", err)
	}
	return compiled.Validate(instance)
}
