package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/merefield/clai/internal/model"
	"github.com/merefield/clai/internal/provider"
)

func (a *Application) retryMissingCommand(ctx context.Context, kind string) (model.Reply, error) {
	feedback := "The previous reply did not provide a usable command. Revisit the original request and put a suitable action or diagnostic command in cmd."
	noCommand := "No command was proposed."
	if kind == "how_to" {
		feedback = "The previous reply did not provide a command example. Put the actual Bash example in cmd for the user to read. CLAI will display it as instructions."
		noCommand = "No command example was provided."
	}
	reply, err := a.retryCommandReply(ctx, kind, feedback+" If you cannot provide one, explain the blocker or ask for the missing detail in info. Return one CLAI JSON reply without calling tools.")
	if err != nil {
		return model.Reply{}, fmt.Errorf("retry missing command: %w", err)
	}
	if reply.Command == "" {
		if detail := strings.TrimSpace(reply.Info); detail != "" {
			noCommand += "\n\n" + detail
		}
		return model.Reply{Info: noCommand, Risk: model.RiskNone, Variables: []model.Variable{}}, nil
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
