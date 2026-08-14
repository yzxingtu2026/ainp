package aitool

import (
	"bytes"
	"encoding/json"
	"fmt"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v5"
)

// ValidateArguments validates model-supplied arguments against the complete
// projected tool schema, including nested objects and collection bounds.
func ValidateArguments(schema, arguments json.RawMessage) error {
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("tool-input.json", bytes.NewReader(schema)); err != nil {
		return fmt.Errorf("invalid tool schema: %w", err)
	}
	contract, err := compiler.Compile("tool-input.json")
	if err != nil {
		return fmt.Errorf("invalid tool schema: %w", err)
	}
	var values any
	decoder := json.NewDecoder(bytes.NewReader(arguments))
	decoder.UseNumber()
	if err = decoder.Decode(&values); err != nil {
		return fmt.Errorf("invalid tool arguments: %w", err)
	}
	if err = contract.Validate(values); err != nil {
		return fmt.Errorf("tool arguments do not match schema: %w", err)
	}
	return nil
}
