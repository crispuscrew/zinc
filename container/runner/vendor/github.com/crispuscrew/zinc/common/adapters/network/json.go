package network

import (
	"encoding/json"
	"fmt"
	"strings"
)

func uniqueKeys(decoder *json.Decoder) error { return scanJSON(decoder, 0) }

func scanJSON(decoder *json.Decoder, depth int) error {
	if depth > 64 {
		return fmt.Errorf("manifest nesting exceeds 64 levels")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, compound := token.(json.Delim)
	if !compound {
		return nil
	}
	if delimiter != '{' && delimiter != '[' {
		return fmt.Errorf("unexpected JSON delimiter")
	}
	seen := map[string]bool{}
	for decoder.More() {
		if delimiter == '{' {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, valid := key.(string)
			if !valid {
				return fmt.Errorf("invalid JSON key")
			}
			canonical := strings.ToLower(name)
			if seen[canonical] {
				return fmt.Errorf("duplicate manifest key %q", name)
			}
			seen[canonical] = true
		}
		if err := scanJSON(decoder, depth+1); err != nil {
			return err
		}
	}
	_, err = decoder.Token()
	return err
}
