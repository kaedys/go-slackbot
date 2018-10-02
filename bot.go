// Package slackbot hopes to ease development of Slack bots by adding helpful
// methods and a mux-router style interface to the github.com/nlopes/slack package.
//
// Incoming Slack RTM events are mapped to a handler in the following form:
// 	bot.Hear("(?i)how are you(.*)").MessageHandler(HowAreYouHandler)
//
// The package adds Reply and ReplyWithAttachments methods:
//	func HowAreYouHandler(ctx context.Context, bot *slackbot.Bot, evt *slack.MessageEvent) {
// 		bot.Reply(evt, "A bit tired. You get it? A bit?", slackbot.WithTyping)
//	}
//
//	func HowAreYouAttachmentsHandler(ctx context.Context, bot *slackbot.Bot, evt *slack.MessageEvent) {
// 		txt := "Beep Beep Boop is a ridiculously simple hosting platform for your Slackbots."
// 		attachment := slack.Attachment{
// 			Pretext:   "We bring bots to life. :sunglasses: :thumbsup:",
// 			Title:     "Host, deploy and share your bot in seconds.",
// 			TitleLink: "https://beepboophq.com/",
// 			Text:      txt,
// 			Fallback:  txt,
// 			ImageURL:  "https://storage.googleapis.com/beepboophq/_assets/bot-1.22f6fb.png",
// 			Color:     "#7CD197",
// 		}
//
//		attachments := []slack.Attachment{attachment}
//		bot.ReplyWithAttachments(evt, attachments, slackbot.WithTyping)
//	}
//
// The slackbot package exposes  github.com/nlopes/slack RTM and Client objects
// enabling a consumer to interact with the lower level package directly:
// 	func HowAreYouHandler(ctx context.Context, bot *slackbot.Bot, evt *slack.MessageEvent) {
// 		bot.RTM.NewOutgoingMessage("Hello", "#random")
// 	}
//
//
// Project home and samples: https://github.com/BeepBoopHQ/go-slackbot
package slackbot

import (
	"context"
	"fmt"
	"time"

	"github.com/nlopes/slack"
	log "github.com/sirupsen/logrus"
)

const maxTypingSleep = time.Millisecond * 2000

// New constructs a new Bot using the slackToken to authorize against the Slack service.
func New(slackToken string) *Bot {
	return &Bot{
		Client:                slack.New(slackToken),
		TypingDelayMultiplier: 0,
	}
}

type Bot struct {
	SimpleRouter
	botUserID             string        // Slack UserID of the bot UserID
	Client                *slack.Client // Slack API
	RTM                   *slack.RTM
	TypingDelayMultiplier float64 // Multiplier on typing delay.  Default 0 -> no delay.  1 -> 2ms per character, 5 -> 10ms per, 0.5 -> 1ms per. Max delay is 2000ms regardless.

	debugging bool
}

// Returns a copy of the bot with debugging enabled.  Intended to be daisychained with the New() constructor.
// Note that this is only a shallow copy, if used after Run() is called, race conditions may occur.
func (b *Bot) WithDebugging() *Bot {
	newB := *b
	newB.debugging = true
	return &newB
}

// Run listens for incoming slack RTM events, matching them to an appropriate handler. It will terminate when the
// provided channel is closed, or if it encounters an error during initial authentication.  Authentication is done
// synchronously, and a non-nil error will be returned if an authentication error is encounters.  Once authentication
// has succeeded, Run will create a new goroutine for the actual message handling, and thus does not need to be run
// in a goroutine itself.
func (b *Bot) Run(quitCh <-chan struct{}) error {
	b.RTM = b.Client.NewRTM()
	go b.RTM.ManageConnection()

auth:
	for {
		select {
		case msg := <-b.RTM.IncomingEvents:
			switch ev := msg.Data.(type) {
			case *slack.ConnectedEvent:
				b.debugf("[Slackbot] Connected: %+v", ev.Info.User)
				b.setBotID(ev.Info.User.ID)
				break auth

			case *slack.InvalidAuthEvent:
				return fmt.Errorf("authentication failed")

			default:
				// Ignore other events, including messages, until auth is successful
			}
		case <-quitCh:
			b.debugf("[Slackbot] Quit event received during authentication.")
			return fmt.Errorf("quit event received during authentication")
		}
	}

	go func() {
		for {
			select {
			case msg := <-b.RTM.IncomingEvents:
				ctx := addBotToContext(context.Background(), b)
				switch ev := msg.Data.(type) {
				case *slack.MessageEvent:
					ctx = addMessageToContext(ctx, ev)
					var match RouteMatch
					if matched, ctx := b.Match(ctx, &match); matched && match.Handler != nil {
						match.Handler(ctx)
					}

				case *slack.RTMError:
					log.WithError(ev).Error("[Slackbot] RTM Error.")

				default:
					// Ignore other events.
				}
			case <-quitCh:
				b.debugf("[Slackbot] Quit event received.")
				return
			}
		}
	}()

	return nil
}

// Reply replies to a message event with a simple message.
func (b *Bot) Reply(evt *slack.MessageEvent, msg string) {
	if b.TypingDelayMultiplier > 0 {
		b.TypeByMessage(evt, msg)
	}
	b.RTM.SendMessage(b.RTM.NewOutgoingMessage(msg, evt.Channel))
}

// ReplyWithAttachments replys to a message event with a Slack Attachments message.
func (b *Bot) ReplyWithAttachments(evt *slack.MessageEvent, msg string, attachments ...slack.Attachment) {
	params := slack.PostMessageParameters{
		AsUser:      true,
		Attachments: attachments,
	}

	b.Client.PostMessage(evt.Msg.Channel, msg, params)
}

// Type sends a typing event to indicate that the bot is "typing" or otherwise working.
func (b *Bot) Type(evt *slack.MessageEvent) {
	b.RTM.SendMessage(b.RTM.NewTypingMessage(evt.Channel))
}

// TypeByMessage sends a typing message and simulates delay (max 2000ms) based on message size.
func (b *Bot) TypeByMessage(evt *slack.MessageEvent, msg interface{}) {
	msgLen := msgLen(msg)

	sleepDuration := time.Duration(float64(time.Minute*time.Duration(msgLen)/30000) * (b.TypingDelayMultiplier))
	if sleepDuration > maxTypingSleep {
		sleepDuration = maxTypingSleep
	}

	b.Type(evt)
	time.Sleep(sleepDuration)
}

// Fetch the botUserID.
func (b *Bot) BotUserID() string {
	return b.botUserID
}

func (b *Bot) setBotID(ID string) {
	b.botUserID = ID
	b.SimpleRouter.SetBotID(ID)
}

func (b *Bot) debugf(format string, args ...interface{}) {
	if b.debugging {
		log.Debugf(format, args...)
	}
}

// msgLen gets length of message and attachment messages. Unsupported types return 0.
func msgLen(msg interface{}) (msgLen int) {
	switch m := msg.(type) {
	case string:
		msgLen = len(m)
	case []slack.Attachment:
		msgLen = len(fmt.Sprintf("%#v", m))
	}
	return
}
