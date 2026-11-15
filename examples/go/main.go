package main

import (
	openai "github.com/pulumi-labs/pulumi-openai/sdk/go/pulumi-openai"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		supportAgent, err := openai.NewAgent(ctx, "supportAgent", &openai.AgentArgs{
			Model:        pulumi.String("gpt-5.5"),
			Name:         pulumi.String("support-agent"),
			Instructions: pulumi.String("You answer customer questions about orders."),
			Metadata:     pulumi.StringMap{"managed-by": pulumi.String("pulumi")},
			Reasoning: &openai.AgentReasoningArgs{
				Effort: openai.ReasoningEffortLow,
			},
			Tools: openai.AgentToolArray{
				&openai.AgentToolArgs{Type: openai.ToolTypeWebSearch},
			},
		})
		if err != nil {
			return err
		}

		dailyDigest, err := openai.NewPeriodicAutomation(ctx, "dailyDigest", &openai.PeriodicAutomationArgs{
			Name:       pulumi.String("daily-digest"),
			Expression: pulumi.String("0 9 * * 1-5"),
			Timezone:   pulumi.String("Europe/Amsterdam"),
			Target: &openai.AutomationTargetArgs{
				AgentId: supportAgent.ID(),
				Environment: &openai.EnvironmentConfigArgs{
					Type: openai.EnvironmentTypeNone,
				},
				Input: openai.InputMessageArray{
					&openai.InputMessageArgs{
						Content: openai.InputContentArray{
							&openai.InputContentArgs{
								Type: openai.InputContentTypeInputText,
								Text: pulumi.String("Summarize yesterday's open orders."),
							},
						},
					},
				},
			},
		})
		if err != nil {
			return err
		}

		ctx.Export("agentId", supportAgent.ID())
		ctx.Export("automationStatus", dailyDigest.Status)
		return nil
	})
}
