package slackbot

import (
	"context"
	"regexp"
)

type Route struct {
	handler      Handler
	err          error
	matchers     []Matcher
	subrouter    Router
	preprocessor Preprocessor
	botUserID    string
	talkToSelf   bool // if set, the bot can reply to its own messages
}

func (r *Route) setBotID(botID string) {
	r.botUserID = botID
	for _, matcher := range r.matchers {
		matcher.SetBotID(botID)
	}
}

// RouteMatch stores information about a matched route.
type RouteMatch struct {
	Route   *Route
	Handler Handler
}

func (r *Route) Match(ctx context.Context, match *RouteMatch) (bool, context.Context) {
	if r.err != nil {
		return false, ctx
	}

	if r.handler == nil {
		return false, ctx
	}

	if ev := MessageFromContext(ctx); ev != nil && !r.talkToSelf && r.botUserID == ev.User {
		return false, ctx
	}

	if r.preprocessor != nil {
		ctx = r.preprocessor(ctx)
	}
	for _, m := range r.matchers {
		var matched bool
		if matched, ctx = m.Match(ctx); !matched {
			return false, ctx
		}
	}

	// if this route contains a subrouter, invoke the subrouter match
	if r.subrouter != nil {
		return r.subrouter.Match(ctx, match)
	}

	match.Route = r
	match.Handler = r.handler
	return true, ctx
}

func (r *Route) TalkToSelf() *Route {
	r.talkToSelf = true
	return r
}

func (r *Route) NoTalkToSelf() *Route {
	r.talkToSelf = false
	return r
}

// Hear adds a matcher for the message text
func (r *Route) Hear(regex string) *Route {
	r.addRegexpMatcher(regex)
	return r
}

func (r *Route) Messages(types ...MessageType) *Route {
	r.addTypesMatcher(types...)
	return r
}

// Handler sets a handler for the route.
func (r *Route) Handler(handler Handler) error {
	if r.err != nil {
		return r.err
	}
	r.handler = handler
	return nil
}

func (r *Route) MessageHandler(fn MessageHandler) error {
	return r.Handler(func(ctx context.Context) {
		bot := BotFromContext(ctx)
		msg := MessageFromContext(ctx)
		fn(ctx, bot, msg)
	})
}

func (r *Route) Preprocess(fn Preprocessor) *Route {
	r.preprocessor = fn
	return r
}

func (r *Route) Subrouter() Router {
	r.subrouter = &SimpleRouter{err: r.err}
	return r.subrouter
}

// addMatcher adds a matcher to the route.
func (r *Route) AddMatcher(m Matcher) *Route {
	r.matchers = append(r.matchers, m)
	return r
}

func (r *Route) Err() error {
	return r.err
}

// ============================================================================
// Regex Type Matcher
// ============================================================================

type RegexpMatcher struct {
	regex     *regexp.Regexp
	botUserID string
}

func (rm *RegexpMatcher) Match(ctx context.Context) (bool, context.Context) {
	msg := MessageFromContext(ctx)
	// A message may be received via a direct mention. For simplicity sake, strip out any potention direct mentions first
	text := StripDirectMention(msg.Text)
	// Now match the stripped text against the regular expression
	matched := rm.regex.MatchString(text)
	return matched, ctx
}

func (rm *RegexpMatcher) SetBotID(botID string) {
	rm.botUserID = botID
}

// addRegexpMatcher adds a host or path matcher and builder to a route.
func (r *Route) addRegexpMatcher(regex string) {
	re, err := regexp.Compile(regex)
	if err != nil {
		r.err = err
	}

	r.AddMatcher(&RegexpMatcher{regex: re})
}

// ============================================================================
// Message Type Matcher
// ============================================================================

type TypesMatcher struct {
	types     []MessageType
	botUserID string
}

func (tm *TypesMatcher) Match(ctx context.Context) (bool, context.Context) {
	msg := MessageFromContext(ctx)
	for _, t := range tm.types {
		switch t {
		case DirectMessage:
			if IsDirectMessage(msg) {
				return true, ctx
			}
		case DirectMention:
			if IsDirectMention(msg, tm.botUserID) {
				return true, ctx
			}
		}
	}
	return false, ctx
}

func (tm *TypesMatcher) SetBotID(botID string) {
	tm.botUserID = botID
}

// addRegexpMatcher adds a host or path matcher and builder to a route.
func (r *Route) addTypesMatcher(types ...MessageType) {
	r.AddMatcher(&TypesMatcher{types: types, botUserID: r.botUserID})
}
