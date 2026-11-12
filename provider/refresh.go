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
	"fmt"
)

// refreshSpec describes how a resource's inputs are refreshed from the API.
//
// Paths are lists of API field names, with "*" standing for any list index.
type refreshSpec struct {
	// freeForm paths hold objects with user-defined keys, where every remote key matters.
	freeForm [][]string
	// writeOnly paths hold values the API does not return, such as credentials. The
	// user's value is kept as is.
	writeOnly [][]string
}

// refreshInputs computes the inputs of a resource from the API's representation of it.
//
// The API returns resolved values: every default is filled in. Adopting them wholesale
// would report a change for each input the user left unset. So when prior inputs exist,
// only the properties the user set are refreshed. With no prior inputs (an import), the
// whole remote configuration is adopted.
func refreshInputs[I any](spec refreshSpec, prior I, hasPrior bool, remoteBody []byte) (I, error) {
	var inputs I
	var remote map[string]any
	if err := json.Unmarshal(remoteBody, &remote); err != nil {
		return inputs, fmt.Errorf("decoding response: %w", err)
	}

	refreshed := remote
	if hasPrior {
		refreshed, _ = spec.project(toAPI(prior), remote, nil).(map[string]any)
	}
	if err := fromAPI(refreshed, &inputs); err != nil {
		return inputs, fmt.Errorf("decoding response: %w", err)
	}
	return inputs, nil
}

// project returns the remote value restricted to the shape of the prior value: objects
// keep only the keys present in prior, recursively. A key the remote no longer has is
// dropped, which surfaces as a change.
func (spec refreshSpec) project(prior, remote any, path []string) any {
	switch prior := prior.(type) {
	case map[string]any:
		remote, ok := remote.(map[string]any)
		if !ok {
			return remote
		}
		if matchesAny(spec.freeForm, path) {
			return remote
		}
		result := map[string]any{}
		for key, priorValue := range prior {
			keyPath := append(path[:len(path):len(path)], key)
			if matchesAny(spec.writeOnly, keyPath) {
				result[key] = priorValue
				continue
			}
			if remoteValue, ok := remote[key]; ok && remoteValue != nil {
				result[key] = spec.project(priorValue, remoteValue, keyPath)
			}
		}
		return result
	case []any:
		remote, ok := remote.([]any)
		if !ok || len(remote) != len(prior) {
			return remote
		}
		result := make([]any, len(prior))
		for i := range prior {
			result[i] = spec.project(prior[i], remote[i], append(path[:len(path):len(path)], "*"))
		}
		return result
	default:
		return remote
	}
}

func matchesAny(patterns [][]string, path []string) bool {
	for _, pattern := range patterns {
		if len(pattern) != len(path) {
			continue
		}
		match := true
		for i := range pattern {
			if pattern[i] != path[i] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
