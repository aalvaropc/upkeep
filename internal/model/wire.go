package model

import (
	"encoding/json"
	"errors"
	"fmt"
)

type wireToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function wireToolFunction `json:"function"`
}

type wireToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type wireMessage struct {
	Role       string         `json:"role"`
	Content    *string        `json:"content"`
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type wireTool struct {
	Type     string       `json:"type"`
	Function wireFunction `json:"function"`
}

type wireFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type wireTemplateKwargs struct {
	EnableThinking bool `json:"enable_thinking"`
}

type wireRequest struct {
	Model              string              `json:"model"`
	Messages           []wireMessage       `json:"messages"`
	Tools              []wireTool          `json:"tools,omitempty"`
	MaxTokens          int                 `json:"max_tokens,omitempty"`
	Temperature        *float64            `json:"temperature,omitempty"`
	ChatTemplateKwargs *wireTemplateKwargs `json:"chat_template_kwargs,omitempty"`
}

type wireUsage struct {
	PromptTokens        int `json:"prompt_tokens"`
	CompletionTokens    int `json:"completion_tokens"`
	PromptTokensDetails *struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
	CompletionTokensDetails *struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
}

type wireResponseMessage struct {
	Role             string         `json:"role"`
	Content          *string        `json:"content"`
	ReasoningContent *string        `json:"reasoning_content"`
	Reasoning        *string        `json:"reasoning"`
	ToolCalls        []wireToolCall `json:"tool_calls"`
}

type wireChoice struct {
	Message      wireResponseMessage `json:"message"`
	FinishReason string              `json:"finish_reason"`
}

type wireResponse struct {
	Choices []wireChoice `json:"choices"`
	Usage   wireUsage    `json:"usage"`
}

func toWire(req Request) wireRequest {
	w := wireRequest{
		Model:       req.Model,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		Messages:    make([]wireMessage, 0, len(req.Messages)),
	}
	if req.Reasoning != nil && !*req.Reasoning {
		w.ChatTemplateKwargs = &wireTemplateKwargs{EnableThinking: false}
	}
	for _, m := range req.Messages {
		wm := wireMessage{Role: m.Role, ToolCallID: m.ToolCallID}
		if m.Content != "" || len(m.ToolCalls) == 0 {
			content := m.Content
			wm.Content = &content
		}
		for _, tc := range m.ToolCalls {
			wm.ToolCalls = append(wm.ToolCalls, wireToolCall{
				ID:       tc.ID,
				Type:     "function",
				Function: wireToolFunction{Name: tc.Name, Arguments: tc.Arguments},
			})
		}
		w.Messages = append(w.Messages, wm)
	}
	for _, t := range req.Tools {
		params := t.Parameters
		if len(params) == 0 {
			params = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		w.Tools = append(w.Tools, wireTool{
			Type:     "function",
			Function: wireFunction{Name: t.Name, Description: t.Description, Parameters: params},
		})
	}
	return w
}

func fromWire(w wireResponse) (Response, error) {
	if len(w.Choices) == 0 {
		return Response{}, errors.New("the response has no choices")
	}
	choice := w.Choices[0]
	msg := Message{Role: choice.Message.Role}
	if msg.Role == "" {
		msg.Role = "assistant"
	}
	if choice.Message.Content != nil {
		msg.Content = *choice.Message.Content
	}
	switch {
	case choice.Message.ReasoningContent != nil:
		msg.Reasoning = *choice.Message.ReasoningContent
	case choice.Message.Reasoning != nil:
		msg.Reasoning = *choice.Message.Reasoning
	}
	for i, tc := range choice.Message.ToolCalls {
		if tc.Function.Name == "" {
			return Response{}, fmt.Errorf("tool call %d has no function name", i)
		}
		id := tc.ID
		if id == "" {
			id = fmt.Sprintf("call_%d", i)
		}
		msg.ToolCalls = append(msg.ToolCalls, ToolCall{ID: id, Name: tc.Function.Name, Arguments: tc.Function.Arguments})
	}
	usage := Usage{PromptTokens: w.Usage.PromptTokens, CompletionTokens: w.Usage.CompletionTokens}
	if w.Usage.PromptTokensDetails != nil {
		usage.CachedPromptTokens = w.Usage.PromptTokensDetails.CachedTokens
	}
	if w.Usage.CompletionTokensDetails != nil {
		usage.ReasoningTokens = w.Usage.CompletionTokensDetails.ReasoningTokens
	}
	return Response{Message: msg, Usage: usage, FinishReason: choice.FinishReason}, nil
}
