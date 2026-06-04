package eval

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/openai/openai-go"
)

func JudgeAnswer(ctx context.Context, gen Generator, question, context, expectedAnswer, generatedAnswer string) (float64, error) {
	prompt := fmt.Sprintf(`You are evaluating the correctness of a generated answer against a ground-truth answer and the retrieved context that was used to generate it.

Question: %s
Retrieved context: %s
Expected answer: %s
Generated answer: %s

Rate the generated answer on a scale from 0.0 to 1.0 where:
- 1.0 means perfectly correct and complete based on the context
- 0.0 means completely wrong or hallucinated from the context

Penalize answers that contradict the context or add information not present in it. Return only a float number without any additional text.`, question, context, expectedAnswer, generatedAnswer)

	completion, err := gen.Generate(ctx, openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage("You are a strict evaluator. Return only a float number between 0.0 and 1.0."),
			openai.UserMessage(prompt),
		},
		Temperature: openai.Float(0.0),
		MaxTokens:   openai.Int(16),
	})
	if err != nil {
		return 0, fmt.Errorf("judge generate: %w", err)
	}

	if len(completion.Choices) == 0 {
		return 0, fmt.Errorf("judge returned no choices")
	}

	scoreStr := strings.TrimSpace(completion.Choices[0].Message.Content)
	score, err := strconv.ParseFloat(scoreStr, 64)
	if err != nil {
		return 0, fmt.Errorf("judge returned non-numeric score: %q", scoreStr)
	}

	if score < 0 || score > 1 {
		return 0, fmt.Errorf("judge score out of range [0,1]: %f", score)
	}

	return score, nil
}
