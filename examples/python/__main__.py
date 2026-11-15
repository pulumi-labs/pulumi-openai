import pulumi
import pulumi_openai as openai

support_agent = openai.Agent(
    "supportAgent",
    model="gpt-5.5",
    name="support-agent",
    instructions="You answer customer questions about orders.",
    metadata={"managed-by": "pulumi"},
    reasoning={"effort": openai.ReasoningEffort.LOW},
    tools=[{"type": openai.ToolType.WEB_SEARCH}],
)

daily_digest = openai.PeriodicAutomation(
    "dailyDigest",
    name="daily-digest",
    expression="0 9 * * 1-5",
    timezone="Europe/Amsterdam",
    target={
        "agent_id": support_agent.id,
        "environment": {"type": openai.EnvironmentType.NONE},
        "input": [
            {
                "content": [
                    {
                        "type": openai.InputContentType.INPUT_TEXT,
                        "text": "Summarize yesterday's open orders.",
                    }
                ],
            }
        ],
    },
)

pulumi.export("agentId", support_agent.id)
pulumi.export("automationStatus", daily_digest.status)
