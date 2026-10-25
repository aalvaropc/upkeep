package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

type Role string

const (
	Ultra Role = "ultra"
	Super Role = "super"
	Nano  Role = "nano"
)

type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	Reasoning  string     `json:"reasoning_content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type Request struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Tools       []Tool    `json:"tools,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature *float64  `json:"temperature,omitempty"`
	Reasoning   *bool     `json:"reasoning,omitempty"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	ReasoningTokens  int `json:"reasoning_tokens"`
}

func (u Usage) Total() int { return u.PromptTokens + u.CompletionTokens }

type Response struct {
	Message      Message       `json:"message"`
	Usage        Usage         `json:"usage"`
	Latency      time.Duration `json:"latency"`
	FinishReason string        `json:"finish_reason"`
}

type Model interface {
	Complete(ctx context.Context, req Request) (Response, error)
}

type route struct {
	name  string
	model Model
}

type Router struct {
	mu     sync.RWMutex
	routes map[Role]route
}

func NewRouter() *Router {
	return &Router{routes: make(map[Role]route)}
}

func (r *Router) Set(role Role, name string, m Model) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.routes[role] = route{name: name, model: m}
}

func (r *Router) For(role Role) (string, Model, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if rt, ok := r.routes[role]; ok {
		return rt.name, rt.model, nil
	}
	if role == Ultra {
		if rt, ok := r.routes[Super]; ok {
			return rt.name, rt.model, nil
		}
	}
	return "", nil, fmt.Errorf("no model configured for the %s role", role)
}

func (r *Router) Largest() (Role, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, role := range []Role{Ultra, Super, Nano} {
		if rt, ok := r.routes[role]; ok {
			return role, rt.name, nil
		}
	}
	return "", "", errors.New("no model configured")
}
