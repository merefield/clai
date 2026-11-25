package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/merefield/clai/internal/model"
	"github.com/merefield/clai/internal/provider"
)

func (a *Application) retryMissingCommand(ctx context.Context, kind string) (model.Reply, error) {
	reply, err := a.retryCommandReply(ctx, kind, "The previous reply did not provide a usable command. Revisit the original request and put a suitable action or diagnostic command in cmd. If you cannot provide one, explain the blocker or ask for the missing detail in info. Return one CLAI JSON reply without calling tools.")
	if err != nil {
		return model.Reply{}, fmt.Errorf("no command was proposed: retry failed: %w", err)
	}
	if reply.Command == "" {
		info := "No command was proposed."
		if detail := strings.TrimSpace(reply.Info); detail != "" {
			info += "\n\n" + detail
		}
		return model.Reply{Info: info, Risk: model.RiskNone, Variables: []model.Variable{}}, nil
	}
	return reply, nil
}

func (a *Application) retryCommandReply(ctx context.Context, kind, feedback string) (model.Reply, error) {
	messages := append(a.messages(kind), model.TextMessage("user", feedback))
	response, err := a.Client.Complete(ctx, provider.Request{Messages: messages})
	if err != nil {
		return model.Reply{}, err
	}
	if len(response.ToolCalls) > 0 || response.FinishReason == "tool_calls" {
		return model.Reply{}, fmt.Errorf("provider requested a tool during command retry")
	}
	return ParseReply(response.Text)
}
