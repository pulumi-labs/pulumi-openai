import * as openai from "@pulumi/openai";

const supportAgent = new openai.Agent("supportAgent", {
  model: "gpt-5.5",
  name: "support-agent",
  instructions: "You answer customer questions about orders.",
  metadata: { "managed-by": "pulumi" },
  reasoning: { effort: openai.ReasoningEffort.Low },
  tools: [{ type: openai.ToolType.WebSearch }],
});

const dailyDigest = new openai.PeriodicAutomation("dailyDigest", {
  name: "daily-digest",
  expression: "0 9 * * 1-5",
  timezone: "Europe/Amsterdam",
  target: {
    agentId: supportAgent.id,
    environment: { type: openai.EnvironmentType.None },
    input: [
      {
        content: [
          { type: openai.InputContentType.InputText, text: "Summarize yesterday's open orders." },
        ],
      },
    ],
  },
});

export const agentId = supportAgent.id;
export const automationStatus = dailyDigest.status;
