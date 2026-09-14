package slack

import (
	"github.com/slack-go/slack/slackevents"
	"testing"
)

func event() slackevents.EventsAPIEvent {
	return slackevents.EventsAPIEvent{Data: &slackevents.EventsAPICallbackEvent{EventID: "event-one"}, InnerEvent: slackevents.EventsAPIInnerEvent{Data: &slackevents.MessageEvent{User: "owner-one", ChannelType: "im", Channel: "dm-one", Text: "What needs a decision?", TimeStamp: "100.1"}}}
}
func TestOwnerDMIsAccepted(t *testing.T) {
	msg, ok := ParseOwnerMessage("owner-one", event())
	if !ok || msg.ID != "event-one" || msg.ThreadTS != "100.1" {
		t.Fatalf("owner DM lost: %#v", msg)
	}
}
func TestOwnerGateRejectsOtherSources(t *testing.T) {
	mutations := []func(*slackevents.EventsAPIEvent){func(e *slackevents.EventsAPIEvent) { e.InnerEvent.Data.(*slackevents.MessageEvent).User = "other-user" }, func(e *slackevents.EventsAPIEvent) {
		e.InnerEvent.Data.(*slackevents.MessageEvent).ChannelType = "channel"
	}, func(e *slackevents.EventsAPIEvent) { e.InnerEvent.Data.(*slackevents.MessageEvent).BotID = "bot" }, func(e *slackevents.EventsAPIEvent) {
		e.InnerEvent.Data.(*slackevents.MessageEvent).SubType = "message_changed"
	}, func(e *slackevents.EventsAPIEvent) { e.IsExtSharedChannel = true }, func(e *slackevents.EventsAPIEvent) { e.Data = nil }, func(e *slackevents.EventsAPIEvent) { e.InnerEvent.Data.(*slackevents.MessageEvent).Text = " " }}
	for i, mutate := range mutations {
		e := event()
		mutate(&e)
		if _, ok := ParseOwnerMessage("owner-one", e); ok {
			t.Errorf("untrusted event %d accepted", i)
		}
	}
}
func TestThreadRepliesStayInThread(t *testing.T) {
	e := event()
	e.InnerEvent.Data.(*slackevents.MessageEvent).ThreadTimeStamp = "99.1"
	m, ok := ParseOwnerMessage("owner-one", e)
	if !ok || m.ThreadTS != "99.1" {
		t.Fatal("thread lost")
	}
}
