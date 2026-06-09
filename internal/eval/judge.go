package eval

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/shared/constant"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/generator"
)

type judgeResponse struct {
	Score     float64 `json:"score"`
	Reasoning string  `json:"reasoning"`
}

const prompt = `You are evaluating the correctness of a generated answer against a ground-truth answer and the retrieved context that was used to generate it.

Question: %s
Retrieved context: %s
Expected answer: %s
Generated answer: %s

Rate the generated answer on a scale from 0.0 to 1.0 where:
- 1.0 means perfectly correct and complete based on the context
- 0.0 means completely wrong or hallucinated from the context

Penalize answers that contradict the context or add information not present in it.`

func JudgeAnswer(ctx context.Context, gen generator.Generator, question, context, expectedAnswer, generatedAnswer string) (float64, error) {
	prompt := fmt.Sprintf(prompt, question, context, expectedAnswer, generatedAnswer)

	completion, err := gen.Generate(ctx, openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage("You are a strict evaluator. Always respond in JSON."),
			openai.UserMessage(prompt),
		},
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &openai.ResponseFormatJSONSchemaParam{
				Type: constant.JSONSchema("json_schema"),
				JSONSchema: openai.ResponseFormatJSONSchemaJSONSchemaParam{
					Name:        "judge_score",
					Strict:      openai.Bool(true),
					Description: openai.String("Score the correctness of a generated answer"),
					Schema: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"score": map[string]any{
								"type":        "number",
								"description": "Correctness score from 0.0 to 1.0",
							},
							"reasoning": map[string]any{
								"type":        "string",
								"description": "Brief explanation for the score",
							},
						},
						"required":             []string{"score", "reasoning"},
						"additionalProperties": false,
					},
				},
			},
		},
		Temperature: openai.Float(0.0),
		MaxTokens:   openai.Int(128),
	})
	if err != nil {
		return 0, fmt.Errorf("judge generate: %w", err)
	}

	if len(completion.Choices) == 0 {
		return 0, fmt.Errorf("judge returned no choices")
	}

	var resp judgeResponse
	if err := json.Unmarshal([]byte(completion.Choices[0].Message.Content), &resp); err != nil {
		return 0, fmt.Errorf("judge returned invalid JSON: %w", err)
	}

	if resp.Score < 0 || resp.Score > 1 {
		return 0, fmt.Errorf("judge score out of range [0,1]: %f", resp.Score)
	}

	return resp.Score, nil
}
