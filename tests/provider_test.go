// Copyright 2025, Pulumi Corporation.
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

package tests

import (
	"context"
	"testing"

	"github.com/blang/semver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/integration"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/property"

	openai "github.com/pulumi-labs/pulumi-openai/provider"
)

func TestAgentPreview(t *testing.T) {
	t.Parallel()

	prov := provider(t)

	// A preview must succeed without credentials and without calling the API.
	response, err := prov.Create(p.CreateRequest{
		Urn: urn("Agent"),
		Properties: property.NewMap(map[string]property.Value{
			"model": property.New("gpt-test"),
			"name":  property.New("my-agent"),
		}),
		DryRun: true,
	})

	require.NoError(t, err)
	assert.Equal(t, "my-agent", response.Properties.Get("name").AsString())
}

// urn is a helper function to build an urn for running integration tests.
func urn(typ string) resource.URN {
	return resource.NewURN("stack", "proj", "",
		tokens.Type("openai:index:"+typ), "name")
}

// Create a test server.
func provider(t *testing.T) integration.Server {
	s, err := integration.NewServer(
		context.Background(),
		openai.Name,
		semver.MustParse("1.0.0"),
		integration.WithProvider(openai.Provider()),
	)
	require.NoError(t, err)
	return s
}
