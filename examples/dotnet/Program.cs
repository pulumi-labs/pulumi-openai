using System.Collections.Generic;
using Pulumi;
using OpenAI = Pulumi.Openai;

return await Deployment.RunAsync(() =>
{
    var supportAgent = new OpenAI.Agent("supportAgent", new()
    {
        Model = "gpt-5.5",
        Name = "support-agent",
        Instructions = "You answer customer questions about orders.",
        Metadata = { { "managed-by", "pulumi" } },
        Reasoning = new OpenAI.Inputs.AgentReasoningArgs
        {
            Effort = OpenAI.ReasoningEffort.Low,
        },
        Tools =
        {
            new OpenAI.Inputs.AgentToolArgs { Type = OpenAI.ToolType.WebSearch },
        },
    });

    var dailyDigest = new OpenAI.PeriodicAutomation("dailyDigest", new()
    {
        Name = "daily-digest",
        Expression = "0 9 * * 1-5",
        Timezone = "Europe/Amsterdam",
        Target = new OpenAI.Inputs.AutomationTargetArgs
        {
            AgentId = supportAgent.Id,
            Environment = new OpenAI.Inputs.EnvironmentConfigArgs
            {
                Type = OpenAI.EnvironmentType.None,
            },
            Input =
            {
                new OpenAI.Inputs.InputMessageArgs
                {
                    Content =
                    {
                        new OpenAI.Inputs.InputContentArgs
                        {
                            Type = OpenAI.InputContentType.InputText,
                            Text = "Summarize yesterday's open orders.",
                        },
                    },
                },
            },
        },
    });

    return new Dictionary<string, object?>
    {
        ["agentId"] = supportAgent.Id,
        ["automationStatus"] = dailyDigest.Status,
    };
});
