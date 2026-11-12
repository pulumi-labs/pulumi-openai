// Copyright 2026, Pulumi Corporation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package provider

import (
	"encoding/json"
	"reflect"
	"strings"
	"unicode"
)

// This file converts between Pulumi input structs and OpenAI API JSON.
//
// An API field is named after the snake_case form of the Pulumi property: `serverUrl` is
// sent as `server_url`. Only struct fields are renamed. The keys of maps are user data
// (metadata, JSON Schemas, headers) and pass through untouched.

// toAPI converts an input struct to its API JSON object. Unset optional fields (nil
// pointers, empty slices and empty maps) are left out.
func toAPI(inputs any) map[string]any {
	object, _ := encodeAPI(reflect.ValueOf(inputs)).(map[string]any)
	return object
}

// encodeAPI returns nil for a value that should be left out.
func encodeAPI(v reflect.Value) any {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			return nil
		}
		return encodeAPI(v.Elem())
	case reflect.Struct:
		object := map[string]any{}
		for i := range v.NumField() {
			name, ok := apiName(v.Type().Field(i))
			if !ok {
				continue
			}
			if value := encodeAPI(v.Field(i)); value != nil {
				object[name] = value
			}
		}
		return object
	case reflect.Slice:
		if v.Len() == 0 {
			return nil
		}
		list := make([]any, v.Len())
		for i := range list {
			list[i] = encodeAPI(v.Index(i))
		}
		return list
	case reflect.Map:
		if v.Len() == 0 {
			return nil
		}
		object := map[string]any{}
		for _, key := range v.MapKeys() {
			object[key.String()] = encodeAPI(v.MapIndex(key))
		}
		return object
	case reflect.String:
		return v.String()
	default:
		return v.Interface()
	}
}

// fromAPI decodes an API JSON object into an input struct, ignoring fields the struct
// does not declare.
func fromAPI(object map[string]any, inputs any) error {
	// Rename API fields to Go field names, then let encoding/json do the typed decoding.
	renamed := renameAPI(object, reflect.TypeOf(inputs))
	encoded, err := json.Marshal(renamed)
	if err != nil {
		return err
	}
	return json.Unmarshal(encoded, inputs)
}

func renameAPI(value any, t reflect.Type) any {
	switch t.Kind() {
	case reflect.Pointer:
		return renameAPI(value, t.Elem())
	case reflect.Struct:
		object, ok := value.(map[string]any)
		if !ok {
			return value
		}
		renamed := map[string]any{}
		for i := range t.NumField() {
			field := t.Field(i)
			name, ok := apiName(field)
			if !ok {
				continue
			}
			if fieldValue, ok := object[name]; ok && fieldValue != nil {
				renamed[field.Name] = renameAPI(fieldValue, field.Type)
			}
		}
		return renamed
	case reflect.Slice:
		list, ok := value.([]any)
		if !ok {
			return value
		}
		renamed := make([]any, len(list))
		for i := range list {
			renamed[i] = renameAPI(list[i], t.Elem())
		}
		return renamed
	default:
		return value
	}
}

// pulumiName returns the Pulumi property name of a struct field, or "" if it has none.
func pulumiName(field reflect.StructField) string {
	name, _, _ := strings.Cut(field.Tag.Get("pulumi"), ",")
	return name
}

// apiName returns the API name of a struct field, derived from its `pulumi` tag.
func apiName(field reflect.StructField) (string, bool) {
	name := pulumiName(field)
	if name == "" {
		return "", false
	}
	var snake strings.Builder
	for _, r := range name {
		if unicode.IsUpper(r) {
			snake.WriteByte('_')
		}
		snake.WriteRune(unicode.ToLower(r))
	}
	return snake.String(), true
}
