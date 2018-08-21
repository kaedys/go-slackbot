package slackbot

import (
	"context"

	"github.com/nlopes/slack"
)

type MessageType int

const (
	DirectMessage MessageType = iota
	DirectMention
)

type Handler func(context.Context)
type MessageHandler func(ctx context.Context, bot *Bot, msg *slack.MessageEvent)
type Preprocessor func(context.Context) context.Context

// Matcher type for matching message routes
type Matcher interface {
	Match(context.Context) (bool, context.Context)
	SetBotID(botID string)
}
