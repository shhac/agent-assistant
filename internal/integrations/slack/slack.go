// Package slack provides an owner-only DM channel using Slack Socket Mode.
package slack

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	slackapi "github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

type Config struct {
	BotTokenEnv string
	AppTokenEnv string
	OwnerUserID string
}
type Message struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	Channel  string `json:"channel"`
	ThreadTS string `json:"thread_ts"`
}

// Inbox must durably claim before acknowledgement. The daemon records completion
// after handling and surfaces unresolved claims after a crash instead of replaying
// potentially effectful owner requests automatically.
type Inbox interface {
	Claim(context.Context, string) (bool, error)
}
type Handler func(context.Context, Message) (string, error)
type Client struct {
	cfg    Config
	inbox  Inbox
	api    *slackapi.Client
	socket *socketmode.Client
}

func New(cfg Config, inbox Inbox) (*Client, error) {
	if cfg.OwnerUserID == "" || cfg.BotTokenEnv == "" || cfg.AppTokenEnv == "" {
		return nil, errors.New("Slack owner user ID and token environment references are required")
	}
	if inbox == nil {
		return nil, errors.New("Slack requires a durable event inbox")
	}
	bot, app := os.Getenv(cfg.BotTokenEnv), os.Getenv(cfg.AppTokenEnv)
	if bot == "" || app == "" {
		return nil, errors.New("Slack bot or app token environment variable is not set")
	}
	// SDK debug output includes protocol payloads; never enable it for private DMs.
	api := slackapi.New(bot, slackapi.OptionAppLevelToken(app), slackapi.OptionLog(log.New(io.Discard, "", 0)))
	socket := socketmode.New(api, socketmode.OptionLog(log.New(io.Discard, "", 0)))
	return &Client{cfg: cfg, inbox: inbox, api: api, socket: socket}, nil
}

// ParseOwnerMessage is a pure allowlist gate. Channel messages, other users,
// bots, edits, shared-channel messages and malformed envelopes cannot invoke the PA.
func ParseOwnerMessage(owner string, event slackevents.EventsAPIEvent) (Message, bool) {
	msg, ok := event.InnerEvent.Data.(*slackevents.MessageEvent)
	if !ok || owner == "" || msg.User != owner || msg.ChannelType != "im" || msg.Channel == "" || msg.SubType != "" || msg.BotID != "" || event.IsExtSharedChannel || strings.TrimSpace(msg.Text) == "" {
		return Message{}, false
	}
	callback, ok := event.Data.(*slackevents.EventsAPICallbackEvent)
	if !ok || callback.EventID == "" {
		return Message{}, false
	}
	thread := msg.ThreadTimeStamp
	if thread == "" {
		thread = msg.TimeStamp
	}
	if thread == "" {
		return Message{}, false
	}
	return Message{ID: callback.EventID, Text: msg.Text, Channel: msg.Channel, ThreadTS: thread}, true
}

func (c *Client) Run(ctx context.Context, handler Handler) error {
	if handler == nil {
		return errors.New("Slack handler is required")
	}
	ctx, cancel := context.WithCancel(ctx)
	var loops sync.WaitGroup
	defer func() { cancel(); loops.Wait() }()
	runErr := make(chan error, 1)
	loops.Add(1)
	go func() { defer loops.Done(); runErr <- c.socket.RunContext(ctx) }()
	queue := make(chan Message, 64)
	handlerErr := make(chan error, 1)
	loops.Add(1)
	go func() {
		defer loops.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case m := <-queue:
				response, err := handler(ctx, m)
				if err != nil {
					response = "I couldn't complete that request. Its recorded state is available in the dashboard; I have not automatically repeated it."
				}
				if response != "" {
					if replyErr := c.reply(ctx, m, response); replyErr != nil {
						select {
						case handlerErr <- replyErr:
						default:
						}
						return
					}
				}
			}
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-handlerErr:
			return errors.New("Slack reply delivery is uncertain; inspect the recorded conversation before resending")
		case err := <-runErr:
			if err != nil {
				return errors.New("Slack Socket Mode stopped; check connectivity and app credentials")
			}
			return nil
		case event := <-c.socket.Events:
			if event.Type == socketmode.EventTypeInvalidAuth {
				return errors.New("Slack rejected app credentials")
			}
			if event.Type != socketmode.EventTypeEventsAPI {
				if event.Request != nil {
					c.ack(ctx, event.Request.EnvelopeID)
				}
				continue
			}
			payload, ok := event.Data.(slackevents.EventsAPIEvent)
			if !ok {
				if event.Request != nil {
					c.ack(ctx, event.Request.EnvelopeID)
				}
				continue
			}
			msg, allowed := ParseOwnerMessage(c.cfg.OwnerUserID, payload)
			if !allowed {
				if event.Request != nil {
					c.ack(ctx, event.Request.EnvelopeID)
				}
				continue
			}
			// Leave a saturated inbox unacknowledged so Slack can retry. Never durably
			// claim a request that cannot be queued in this process.
			if len(queue) == cap(queue) {
				continue
			}
			fresh, err := c.inbox.Claim(ctx, msg.ID)
			if err != nil {
				return errors.New("cannot persist Slack event receipt")
			}
			if event.Request != nil {
				c.ack(ctx, event.Request.EnvelopeID)
			}
			if fresh {
				queue <- msg
			}
		}
	}
}
func (c *Client) ack(ctx context.Context, id string) {
	ackCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_ = c.socket.AckCtx(ackCtx, id, nil)
}

func (c *Client) reply(ctx context.Context, msg Message, text string) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	_, _, err := c.api.PostMessageContext(ctx, msg.Channel, slackapi.MsgOptionText(text, false), slackapi.MsgOptionTS(msg.ThreadTS), slackapi.MsgOptionDisableLinkUnfurl(), slackapi.MsgOptionDisableMediaUnfurl())
	if err != nil {
		return errors.New("Slack message delivery failed or is uncertain")
	}
	return nil
}

// Notify sends only to the configured owner. The caller decides when a meaningful
// update warrants notification and records an outbox entry before sending.
func (c *Client) Notify(ctx context.Context, text string) error {
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("Slack notification is empty")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	ch, _, _, err := c.api.OpenConversationContext(ctx, &slackapi.OpenConversationParameters{Users: []string{c.cfg.OwnerUserID}})
	if err != nil {
		return errors.New("cannot open Slack owner DM")
	}
	return c.reply(ctx, Message{Channel: ch.ID}, text)
}
