//go:build dotnet || all
// +build dotnet all

package examples

import (
	"testing"

	"github.com/pulumi/providertest/pulumitest"
	"github.com/pulumi/providertest/pulumitest/opttest"
)

func TestDotnet(t *testing.T) {
	pt := pulumitest.NewPulumiTest(t, "dotnet",
		opttest.DotNetReference("Pulumi.Openai", "../sdk/dotnet"),
		opttest.AttachProviderServer("openai", providerFactory),
	)

	pt.Preview(t)
	pt.Up(t)
	pt.Destroy(t)
}
