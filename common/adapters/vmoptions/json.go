package vmoptions

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"

	domain "github.com/crispuscrew/zinc/common/domain/vmoptions"
)

func encode(config domain.Config) ([]byte, error) {
	data, err := json.MarshalIndent(config, "", "  ")
	if len(data) >= maxBytes {
		return nil, fmt.Errorf("VM options exceed %d bytes", maxBytes)
	}
	return append(data, '\n'), err
}

func decode(data []byte) (domain.Config, error) {
	var config domain.Config
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := checkObject(decoder, reflect.TypeOf(config), 0); err != nil {
		return config, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return config, fmt.Errorf("VM options: trailing JSON data")
	}
	decoder = json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return config, err
	}
	return config, nil
}

// Check names exactly: encoding/json normally accepts case-insensitive aliases,
// including duplicate keys with different case, and null for scalar fields.
func checkObject(decoder *json.Decoder, shape reflect.Type, depth int) error {
	if depth > 16 {
		return fmt.Errorf("VM options: excessive JSON nesting")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if shape.Kind() == reflect.Struct {
		if token != json.Delim('{') {
			return fmt.Errorf("VM options: expected JSON object")
		}
		fields := map[string]reflect.Type{}
		for index := 0; index < shape.NumField(); index++ {
			field := shape.Field(index)
			fields[strings.Split(field.Tag.Get("json"), ",")[0]] = field.Type
		}
		seen := map[string]bool{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			field, known := fields[name]
			if !ok || !known || seen[name] {
				return fmt.Errorf("VM options: unknown or duplicate key %q", key)
			}
			seen[name] = true
			if err := checkObject(decoder, field, depth+1); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	}
	if shape.Kind() == reflect.Slice {
		if token == nil {
			return nil
		}
		if token != json.Delim('[') {
			return fmt.Errorf("VM options: expected JSON array")
		}
		for decoder.More() {
			if err := checkObject(decoder, shape.Elem(), depth+1); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	}
	if token == nil {
		return fmt.Errorf("VM options: null scalar is not allowed")
	}
	if _, nested := token.(json.Delim); nested {
		return fmt.Errorf("VM options: expected scalar")
	}
	return nil
}
