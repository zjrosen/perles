package fabric

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zjrosen/perles/internal/orchestration/fabric/domain"
	"github.com/zjrosen/perles/internal/orchestration/fabric/repository"
)

func newTestService() *Service {
	threadRepo := repository.NewMemoryThreadRepository()
	depRepo := repository.NewMemoryDependencyRepository()
	subRepo := repository.NewMemorySubscriptionRepository()
	ackRepo := repository.NewMemoryAckRepository(depRepo, threadRepo, subRepo)

	return NewService(threadRepo, depRepo, subRepo, ackRepo)
}

func TestService_InitSession(t *testing.T) {
	svc := newTestService()

	var events []Event
	svc.SetEventHandler(func(e Event) {
		events = append(events, e)
	})

	err := svc.InitSession("coordinator")
	require.NoError(t, err)

	// Should have created 6 channels (root, system, tasks, planning, general, observer)
	require.Len(t, events, 6)
	for _, e := range events {
		require.Equal(t, EventChannelCreated, e.Type)
	}

	// Verify channel IDs are set
	require.NotEmpty(t, svc.GetChannelID(domain.SlugRoot))
	require.NotEmpty(t, svc.GetChannelID(domain.SlugSystem))
	require.NotEmpty(t, svc.GetChannelID(domain.SlugTasks))
	require.NotEmpty(t, svc.GetChannelID(domain.SlugPlanning))
	require.NotEmpty(t, svc.GetChannelID(domain.SlugGeneral))
	require.NotEmpty(t, svc.GetChannelID(domain.SlugObserver))

	// Verify coordinator is auto-subscribed to #system with mode=all
	subs, err := svc.GetSubscriptions("coordinator")
	require.NoError(t, err)
	require.Len(t, subs, 1, "coordinator should have 1 auto-subscription")
	require.Equal(t, svc.GetChannelID(domain.SlugSystem), subs[0].ChannelID)
	require.Equal(t, "coordinator", subs[0].AgentID)
	require.Equal(t, domain.ModeAll, subs[0].Mode)
}

func TestService_GetChannel(t *testing.T) {
	svc := newTestService()
	err := svc.InitSession("system")
	require.NoError(t, err)

	channel, err := svc.GetChannel(domain.SlugTasks)
	require.NoError(t, err)
	require.Equal(t, "Tasks", channel.Title)
	require.Equal(t, domain.ThreadChannel, channel.Type)
}

func TestService_SendMessage(t *testing.T) {
	svc := newTestService()
	err := svc.InitSession("system")
	require.NoError(t, err)

	var events []Event
	svc.SetEventHandler(func(e Event) {
		events = append(events, e)
	})

	msg, err := svc.SendMessage(SendMessageInput{
		ChannelSlug: domain.SlugTasks,
		Content:     "Task: Implement login @worker-1",
		CreatedBy:   "COORDINATOR",
	})
	require.NoError(t, err)
	require.NotEmpty(t, msg.ID)
	require.Equal(t, "Task: Implement login @worker-1", msg.Content)
	require.Equal(t, string(domain.KindInfo), msg.Kind)
	require.Contains(t, msg.Mentions, "worker-1")

	// Verify event
	require.Len(t, events, 1)
	require.Equal(t, EventMessagePosted, events[0].Type)
}

func TestService_Reply(t *testing.T) {
	svc := newTestService()
	err := svc.InitSession("system")
	require.NoError(t, err)

	msg, err := svc.SendMessage(SendMessageInput{
		ChannelSlug: domain.SlugTasks,
		Content:     "Task: Implement login",
		CreatedBy:   "COORDINATOR",
	})
	require.NoError(t, err)

	var events []Event
	svc.SetEventHandler(func(e Event) {
		events = append(events, e)
	})

	reply, err := svc.Reply(ReplyInput{
		MessageID: msg.ID,
		Content:   "Starting work @coordinator",
		CreatedBy: "WORKER.1",
	})
	require.NoError(t, err)
	require.NotEmpty(t, reply.ID)
	require.Equal(t, string(domain.KindResponse), reply.Kind)
	require.Contains(t, reply.Mentions, "coordinator")

	// Verify event
	require.Len(t, events, 1)
	require.Equal(t, EventReplyPosted, events[0].Type)
}

func TestService_GetReplies(t *testing.T) {
	svc := newTestService()
	err := svc.InitSession("system")
	require.NoError(t, err)

	msg, err := svc.SendMessage(SendMessageInput{
		ChannelSlug: domain.SlugTasks,
		Content:     "Task: Implement login",
		CreatedBy:   "COORDINATOR",
	})
	require.NoError(t, err)

	_, err = svc.Reply(ReplyInput{
		MessageID: msg.ID,
		Content:   "Starting work",
		CreatedBy: "WORKER.1",
	})
	require.NoError(t, err)

	_, err = svc.Reply(ReplyInput{
		MessageID: msg.ID,
		Content:   "Done!",
		CreatedBy: "WORKER.1",
	})
	require.NoError(t, err)

	replies, err := svc.GetReplies(msg.ID)
	require.NoError(t, err)
	require.Len(t, replies, 2)
}

func TestService_AttachArtifact(t *testing.T) {
	svc := newTestService()
	err := svc.InitSession("system")
	require.NoError(t, err)

	msg, err := svc.SendMessage(SendMessageInput{
		ChannelSlug: domain.SlugTasks,
		Content:     "Here's my diff",
		CreatedBy:   "WORKER.1",
	})
	require.NoError(t, err)

	// Create a temp file for the artifact
	tmpFile, err := os.CreateTemp("", "test-artifact-*.diff")
	require.NoError(t, err)
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	_, err = tmpFile.WriteString("--- a/file.go\n+++ b/file.go\n@@ -1,3 +1,4 @@")
	require.NoError(t, err)
	require.NoError(t, tmpFile.Close())

	artifact, err := svc.AttachArtifact(AttachArtifactInput{
		TargetID:  msg.ID,
		Path:      tmpFile.Name(),
		Name:      "changes.diff",
		CreatedBy: "WORKER.1",
	})
	require.NoError(t, err)
	require.NotEmpty(t, artifact.ID)
	require.Equal(t, "changes.diff", artifact.Name)
	require.Equal(t, domain.ThreadArtifact, artifact.Type)
	require.Equal(t, "text/x-diff", artifact.MediaType)
	require.Contains(t, artifact.StorageURI, tmpFile.Name())
	require.NotEmpty(t, artifact.Sha256)

	// Retrieve artifact
	artifacts, err := svc.GetArtifacts(msg.ID)
	require.NoError(t, err)
	require.Len(t, artifacts, 1)

	// Get content (reads from disk)
	content, err := svc.GetArtifactContent(artifact.ID)
	require.NoError(t, err)
	require.Contains(t, string(content), "file.go")
}

func TestService_ListMessages(t *testing.T) {
	svc := newTestService()
	err := svc.InitSession("system")
	require.NoError(t, err)

	for i := 0; i < 5; i++ {
		_, err := svc.SendMessage(SendMessageInput{
			ChannelSlug: domain.SlugTasks,
			Content:     "Message",
			CreatedBy:   "COORDINATOR",
		})
		require.NoError(t, err)
	}

	messages, err := svc.ListMessages(domain.SlugTasks, 0)
	require.NoError(t, err)
	require.Len(t, messages, 5)

	// With limit
	limited, err := svc.ListMessages(domain.SlugTasks, 3)
	require.NoError(t, err)
	require.Len(t, limited, 3)
}

func TestService_AckAndUnacked(t *testing.T) {
	svc := newTestService()
	err := svc.InitSession("system")
	require.NoError(t, err)

	// Subscribe WORKER.1 to tasks channel so they can see messages
	_, err = svc.Subscribe(domain.SlugTasks, "WORKER.1", domain.ModeAll)
	require.NoError(t, err)

	msg1, err := svc.SendMessage(SendMessageInput{
		ChannelSlug: domain.SlugTasks,
		Content:     "Message 1",
		CreatedBy:   "COORDINATOR",
	})
	require.NoError(t, err)

	msg2, err := svc.SendMessage(SendMessageInput{
		ChannelSlug: domain.SlugTasks,
		Content:     "Message 2",
		CreatedBy:   "COORDINATOR",
	})
	require.NoError(t, err)

	// Check unacked
	unacked, err := svc.GetUnacked("WORKER.1")
	require.NoError(t, err)
	tasksID := svc.GetChannelID(domain.SlugTasks)
	require.Equal(t, 2, unacked[tasksID].Count)

	// Ack one message
	err = svc.Ack("WORKER.1", msg1.ID)
	require.NoError(t, err)

	unacked, err = svc.GetUnacked("WORKER.1")
	require.NoError(t, err)
	require.Equal(t, 1, unacked[tasksID].Count)
	require.Contains(t, unacked[tasksID].ThreadIDs, msg2.ID)
}

func TestService_Subscriptions(t *testing.T) {
	svc := newTestService()
	err := svc.InitSession("system")
	require.NoError(t, err)

	sub, err := svc.Subscribe(domain.SlugTasks, "WORKER.1", domain.ModeAll)
	require.NoError(t, err)
	require.Equal(t, domain.ModeAll, sub.Mode)

	subs, err := svc.GetSubscriptions("WORKER.1")
	require.NoError(t, err)
	require.Len(t, subs, 1)

	err = svc.Unsubscribe(domain.SlugTasks, "WORKER.1")
	require.NoError(t, err)

	subs, err = svc.GetSubscriptions("WORKER.1")
	require.NoError(t, err)
	require.Len(t, subs, 0)
}

func TestService_UnsubscribeAll(t *testing.T) {
	svc := newTestService()
	err := svc.InitSession("system")
	require.NoError(t, err)

	// Subscribe observer to multiple channels
	_, err = svc.Subscribe(domain.SlugSystem, "OBSERVER", domain.ModeAll)
	require.NoError(t, err)
	_, err = svc.Subscribe(domain.SlugTasks, "OBSERVER", domain.ModeAll)
	require.NoError(t, err)
	_, err = svc.Subscribe(domain.SlugPlanning, "OBSERVER", domain.ModeAll)
	require.NoError(t, err)
	_, err = svc.Subscribe(domain.SlugGeneral, "OBSERVER", domain.ModeAll)
	require.NoError(t, err)

	// Verify subscriptions exist
	subs, err := svc.GetSubscriptions("OBSERVER")
	require.NoError(t, err)
	require.Len(t, subs, 4)

	// Unsubscribe from all
	err = svc.UnsubscribeAll("OBSERVER")
	require.NoError(t, err)

	// Verify all subscriptions removed
	subs, err = svc.GetSubscriptions("OBSERVER")
	require.NoError(t, err)
	require.Len(t, subs, 0)
}

func TestService_UnsubscribeAll_NoSubscriptions(t *testing.T) {
	svc := newTestService()
	err := svc.InitSession("system")
	require.NoError(t, err)

	// UnsubscribeAll on agent with no subscriptions should be no-op
	err = svc.UnsubscribeAll("NONEXISTENT")
	require.NoError(t, err)
}

func TestParseMentions(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{
			name:    "single mention",
			content: "Hello @worker-1",
			want:    []string{"worker-1"},
		},
		{
			name:    "multiple mentions",
			content: "@coordinator please review @worker-2",
			want:    []string{"coordinator", "worker-2"},
		},
		{
			name:    "uppercase mention normalized",
			content: "Hi @WORKER.1",
			want:    []string{"worker.1"},
		},
		{
			name:    "duplicate mentions",
			content: "@worker-1 hello @worker-1",
			want:    []string{"worker-1"},
		},
		{
			name:    "no mentions",
			content: "Hello world",
			want:    nil,
		},
		{
			name:    "mention with underscore",
			content: "Hi @worker_one",
			want:    []string{"worker_one"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseMentions(tt.content)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestService_ParticipantTracking(t *testing.T) {
	svc := newTestService()
	err := svc.InitSession("system")
	require.NoError(t, err)

	// Coordinator sends message mentioning 3 workers
	// Note: parseMentions extracts lowercase from content, so we use explicit Mentions
	msg, err := svc.SendMessage(SendMessageInput{
		ChannelSlug: domain.SlugPlanning,
		Content:     "Let's discuss the plan",
		CreatedBy:   "COORDINATOR",
		Mentions:    []string{"WORKER.1", "WORKER.2", "WORKER.3"},
	})
	require.NoError(t, err)

	// Initial message should have sender + all mentioned as participants
	require.ElementsMatch(t, []string{"COORDINATOR", "WORKER.1", "WORKER.2", "WORKER.3"}, msg.Participants)

	// Worker.1 replies (no mentions)
	var replyEvent Event
	svc.SetEventHandler(func(e Event) {
		if e.Type == EventReplyPosted {
			replyEvent = e
		}
	})

	_, err = svc.Reply(ReplyInput{
		MessageID: msg.ID,
		Content:   "I think we should start with the API",
		CreatedBy: "WORKER.1",
	})
	require.NoError(t, err)

	// Reply event should include all parent participants for broker notification
	require.ElementsMatch(t, []string{"COORDINATOR", "WORKER.1", "WORKER.2", "WORKER.3"}, replyEvent.Participants)

	// Fetch updated parent thread - Worker.1 should still be in participants
	updated, err := svc.GetThread(msg.ID)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"COORDINATOR", "WORKER.1", "WORKER.2", "WORKER.3"}, updated.Participants)

	// Worker.4 replies (new participant via reply)
	_, err = svc.Reply(ReplyInput{
		MessageID: msg.ID,
		Content:   "I agree with Worker.1",
		CreatedBy: "WORKER.4",
	})
	require.NoError(t, err)

	// Now Worker.4 should be added as participant
	updated, err = svc.GetThread(msg.ID)
	require.NoError(t, err)
	require.Contains(t, updated.Participants, "WORKER.4")
}

func TestService_ReplyFlattensToRoot(t *testing.T) {
	svc := newTestService()
	err := svc.InitSession("system")
	require.NoError(t, err)

	// Create root message
	root, err := svc.SendMessage(SendMessageInput{
		ChannelSlug: domain.SlugTasks,
		Content:     "Root task message",
		CreatedBy:   "COORDINATOR",
	})
	require.NoError(t, err)

	// First reply to root
	reply1, err := svc.Reply(ReplyInput{
		MessageID: root.ID,
		Content:   "First reply",
		CreatedBy: "WORKER.1",
	})
	require.NoError(t, err)

	// Second reply tries to reply to reply1 (nested)
	reply2, err := svc.Reply(ReplyInput{
		MessageID: reply1.ID, // Replying to reply, not root
		Content:   "Second reply (should flatten)",
		CreatedBy: "WORKER.2",
	})
	require.NoError(t, err)

	// Third reply tries to reply to reply2 (even deeper)
	reply3, err := svc.Reply(ReplyInput{
		MessageID: reply2.ID, // Replying to nested reply
		Content:   "Third reply (should also flatten)",
		CreatedBy: "WORKER.3",
	})
	require.NoError(t, err)

	// All replies should be direct children of root
	replies, err := svc.GetReplies(root.ID)
	require.NoError(t, err)
	require.Len(t, replies, 3, "all 3 replies should be flattened to root")

	// Verify reply IDs are present
	replyIDs := make(map[string]bool)
	for _, r := range replies {
		replyIDs[r.ID] = true
	}
	require.True(t, replyIDs[reply1.ID], "reply1 should be under root")
	require.True(t, replyIDs[reply2.ID], "reply2 should be under root (flattened)")
	require.True(t, replyIDs[reply3.ID], "reply3 should be under root (flattened)")

	// Verify nested messages have no children (they were flattened up)
	reply1Children, err := svc.GetReplies(reply1.ID)
	require.NoError(t, err)
	require.Len(t, reply1Children, 0, "reply1 should have no children - flattened to root")

	reply2Children, err := svc.GetReplies(reply2.ID)
	require.NoError(t, err)
	require.Len(t, reply2Children, 0, "reply2 should have no children - flattened to root")
}

func TestService_ReplyFlattening_ParticipantsOnRoot(t *testing.T) {
	svc := newTestService()
	err := svc.InitSession("system")
	require.NoError(t, err)

	// Create root message
	root, err := svc.SendMessage(SendMessageInput{
		ChannelSlug: domain.SlugGeneral,
		Content:     "Discussion thread",
		CreatedBy:   "COORDINATOR",
	})
	require.NoError(t, err)

	// First reply
	reply1, err := svc.Reply(ReplyInput{
		MessageID: root.ID,
		Content:   "Reply from worker-1",
		CreatedBy: "WORKER.1",
	})
	require.NoError(t, err)

	// Nested reply (to reply1, but should flatten to root)
	_, err = svc.Reply(ReplyInput{
		MessageID: reply1.ID,
		Content:   "Reply from worker-2 @worker-3",
		CreatedBy: "WORKER.2",
	})
	require.NoError(t, err)

	// All participants should be tracked on root, not on intermediate replies
	rootThread, err := svc.GetThread(root.ID)
	require.NoError(t, err)

	require.Contains(t, rootThread.Participants, "COORDINATOR", "creator should be participant")
	require.Contains(t, rootThread.Participants, "WORKER.1", "worker-1 should be participant")
	require.Contains(t, rootThread.Participants, "WORKER.2", "worker-2 should be participant (flattened)")
	require.Contains(t, rootThread.Participants, "worker-3", "mentioned worker-3 should be participant")

	// Intermediate reply should NOT have worker-2 as participant (tracking is on root)
	reply1Thread, err := svc.GetThread(reply1.ID)
	require.NoError(t, err)
	require.NotContains(t, reply1Thread.Participants, "WORKER.2", "worker-2 should not be on reply1, only on root")
}

func TestService_findThreadRoot(t *testing.T) {
	svc := newTestService()
	err := svc.InitSession("system")
	require.NoError(t, err)

	// Create a message (this is a root - has no reply_to parent)
	root, err := svc.SendMessage(SendMessageInput{
		ChannelSlug: domain.SlugTasks,
		Content:     "Root message",
		CreatedBy:   "COORDINATOR",
	})
	require.NoError(t, err)

	// findThreadRoot on root should return empty (it IS the root)
	foundRoot := svc.findThreadRoot(root.ID)
	require.Empty(t, foundRoot, "root message should return empty from findThreadRoot")

	// Create a reply
	reply1, err := svc.Reply(ReplyInput{
		MessageID: root.ID,
		Content:   "First reply",
		CreatedBy: "WORKER.1",
	})
	require.NoError(t, err)

	// findThreadRoot on reply1 should return root.ID
	foundRoot = svc.findThreadRoot(reply1.ID)
	require.Equal(t, root.ID, foundRoot, "reply1 should find root as its thread root")

	// Create another reply (will be flattened to root)
	reply2, err := svc.Reply(ReplyInput{
		MessageID: reply1.ID, // Tries to reply to reply1
		Content:   "Second reply",
		CreatedBy: "WORKER.2",
	})
	require.NoError(t, err)

	// findThreadRoot on reply2 should also return root.ID (flattened)
	foundRoot = svc.findThreadRoot(reply2.ID)
	require.Equal(t, root.ID, foundRoot, "reply2 should find root as its thread root (flattened)")
}

func TestEventStruct_HasChannelSlug(t *testing.T) {
	// Verify the Event struct has the ChannelSlug field by creating an event and setting it
	event := Event{
		Type:        EventMessagePosted,
		ChannelSlug: "tasks",
	}
	require.Equal(t, "tasks", event.ChannelSlug)
}

func TestEmitMessagePosted_PopulatesChannelSlug(t *testing.T) {
	svc := newTestService()
	err := svc.InitSession("system")
	require.NoError(t, err)

	var capturedEvent Event
	svc.SetEventHandler(func(e Event) {
		if e.Type == EventMessagePosted {
			capturedEvent = e
		}
	})

	_, err = svc.SendMessage(SendMessageInput{
		ChannelSlug: domain.SlugTasks,
		Content:     "Test message",
		CreatedBy:   "COORDINATOR",
	})
	require.NoError(t, err)

	require.Equal(t, EventMessagePosted, capturedEvent.Type)
	require.Equal(t, domain.SlugTasks, capturedEvent.ChannelSlug)
}

func TestEmitReplyPosted_PopulatesChannelSlug(t *testing.T) {
	svc := newTestService()
	err := svc.InitSession("system")
	require.NoError(t, err)

	msg, err := svc.SendMessage(SendMessageInput{
		ChannelSlug: domain.SlugPlanning,
		Content:     "Original message",
		CreatedBy:   "COORDINATOR",
	})
	require.NoError(t, err)

	var capturedEvent Event
	svc.SetEventHandler(func(e Event) {
		if e.Type == EventReplyPosted {
			capturedEvent = e
		}
	})

	_, err = svc.Reply(ReplyInput{
		MessageID: msg.ID,
		Content:   "Reply message",
		CreatedBy: "WORKER.1",
	})
	require.NoError(t, err)

	require.Equal(t, EventReplyPosted, capturedEvent.Type)
	require.Equal(t, domain.SlugPlanning, capturedEvent.ChannelSlug)
}

func TestChannelSlug_AllChannelTypes(t *testing.T) {
	// Test that all 4 user-facing channel types populate ChannelSlug correctly
	channels := []string{domain.SlugTasks, domain.SlugPlanning, domain.SlugGeneral, domain.SlugSystem}

	for _, channelSlug := range channels {
		t.Run(channelSlug, func(t *testing.T) {
			svc := newTestService()
			err := svc.InitSession("system")
			require.NoError(t, err)

			var capturedEvent Event
			svc.SetEventHandler(func(e Event) {
				if e.Type == EventMessagePosted {
					capturedEvent = e
				}
			})

			_, err = svc.SendMessage(SendMessageInput{
				ChannelSlug: channelSlug,
				Content:     "Test message for " + channelSlug,
				CreatedBy:   "COORDINATOR",
			})
			require.NoError(t, err)

			require.Equal(t, EventMessagePosted, capturedEvent.Type)
			require.Equal(t, channelSlug, capturedEvent.ChannelSlug, "ChannelSlug should be populated for %s", channelSlug)
		})
	}
}
