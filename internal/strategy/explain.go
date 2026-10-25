package strategy

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/aalvaropc/upkeep/internal/model"
)

const (
	explainSystem = `You write one short note for the reviewer of a pull request about a change an automated agent made to it. In at most 45 words, in plain prose, say what was changed and why it was needed, using only the pull request title, the failure and the diff you are given. Do not use headings, lists or code blocks, and do not mention the agent.`

	maxExplainPatch   = 6000
	maxExplainFailure = 1200
)

var ErrUngrounded = errors.New("the explanation names things that are not in the change")

func (e Env) CanExplain() bool {
	if e.Models == nil {
		return false
	}
	_, _, err := e.Models.For(model.Nano)
	return err == nil
}

func (e Env) Explain(ctx context.Context, job *Job, failure, patch string) (string, error) {
	if !e.CanExplain() || strings.TrimSpace(patch) == "" {
		return "", nil
	}
	if len(patch) > maxExplainPatch {
		patch = patch[:maxExplainPatch] + "\n... the diff continues ..."
	}
	failure = strings.TrimSpace(failure)
	if len(failure) > maxExplainFailure {
		failure = failure[:maxExplainFailure] + "\n..."
	}
	var user strings.Builder
	fmt.Fprintf(&user, "Pull request: %s\n", job.PR.Title)
	if failure != "" {
		fmt.Fprintf(&user, "\nWhat was failing:\n%s\n", failure)
	}
	fmt.Fprintf(&user, "\nThe change:\n```diff\n%s\n```", strings.TrimRight(patch, "\n"))
	answer, err := e.complete(ctx, job, model.Nano, explainSystem, user.String(), false, 300)
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(strings.Trim(strings.TrimSpace(answer), "`"))
	if !Grounded(text, patch+"\n"+failure+"\n"+job.PR.Title) {
		return "", ErrUngrounded
	}
	return text, nil
}

var (
	quoted    = regexp.MustCompile("`([^`]+)`")
	camelCase = regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_]*[a-z][A-Z][A-Za-z0-9_]*\b|\b[A-Za-z]+\.[A-Za-z][A-Za-z0-9_.]*\b`)
	words     = regexp.MustCompile(`[A-Za-z][A-Za-z0-9_]*`)
)

func Grounded(text, corpus string) bool {
	for _, m := range quoted.FindAllStringSubmatch(text, -1) {
		if !strings.Contains(corpus, m[1]) {
			return false
		}
	}
	for _, token := range camelCase.FindAllString(text, -1) {
		if !strings.Contains(corpus, token) {
			return false
		}
	}
	start := true
	for _, loc := range words.FindAllStringIndex(text, -1) {
		word := text[loc[0]:loc[1]]
		first := start
		start = false
		if loc[0] > 0 {
			before := strings.TrimRight(text[:loc[0]], " \t\n")
			first = before == "" || strings.HasSuffix(before, ".") || strings.HasSuffix(before, ":")
		}
		if !first && word[0] >= 'A' && word[0] <= 'Z' && !strings.Contains(corpus, word) {
			return false
		}
	}
	return true
}
