package slackbot

import (
	"context"

	"github.com/nlopes/slack"
)

// key is unexported so other packages cannot access these keys directly or by mimicking their values.
// This ensures that messages and bots can only be added to or retrieved from the context via these functions.
type key int

const (
	bot_context_key key = iota
	message_context_key
)

func BotFromContext(ctx context.Context) *Bot {
	if result, ok := ctx.Value(bot_context_key).(*Bot); ok {
		return result
	}
	return nil
}

func addBotToContext(ctx context.Context, bot *Bot) context.Context {
	return context.WithValue(ctx, bot_context_key, bot)
}

func MessageFromContext(ctx context.Context) *slack.MessageEvent {
	if result, ok := ctx.Value(message_context_key).(*slack.MessageEvent); ok {
		return result
	}
	return nil
}

func addMessageToContext(ctx context.Context, msg *slack.MessageEvent) context.Context {
	return context.WithValue(ctx, message_context_key, msg)
}
