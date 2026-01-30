package fabric

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/zjrosen/perles/internal/orchestration/fabric/domain"
	"github.com/zjrosen/perles/internal/orchestration/fabric/repository"
)

// Service is the main facade for Fabric operations.
type Service struct {
	threads       repository.ThreadRepository
	dependencies  repository.DependencyRepository
	subscriptions repository.SubscriptionRepository
	acks          repository.AckRepository

	// Channel IDs for the fixed structure
	rootID     string
	systemID   string
	tasksID    string
	planningID string
	generalID  string
	observerID string

	// Event handler (optional)
	onEvent func(Event)
}

// NewService creates a new Fabric service.
func NewService(
	threads repository.ThreadRepository,
	deps repository.DependencyRepository,
	subs repository.SubscriptionRepository,
	acks repository.AckRepository,
) *Service {
	return &Service{
		threads:       threads,
		dependencies:  deps,
		subscriptions: subs,
		acks:          acks,
	}
}

// SetEventHandler sets the callback for fabric events.
func (s *Service) SetEventHandler(handler func(Event)) {
	s.onEvent = handler
}

// SubscriptionRepository returns the subscription repository for external use (e.g., FabricBroker).
func (s *Service) SubscriptionRepository() repository.SubscriptionRepository {
	return s.subscriptions
}

// emit publishes an event if a handler is registered.
func (s *Service) emit(event Event) {
	if s.onEvent != nil {
		s.onEvent(event)
	}
}

// InitSession creates the fixed channel structure for a new session.
func (s *Service) InitSession(createdBy string) error {
	channels := domain.FixedChannels()
	channelIDs := make(map[string]string)

	for _, ch := range channels {
		ch.CreatedBy = createdBy
		ch.CreatedAt = time.Now()

		created, err := s.threads.Create(ch)
		if err != nil {
			return fmt.Errorf("create channel %s: %w", ch.Slug, err)
		}

		channelIDs[ch.Slug] = created.ID
		s.emit(NewChannelCreatedEvent(created))
	}

	s.rootID = channelIDs[domain.SlugRoot]
	s.systemID = channelIDs[domain.SlugSystem]
	s.tasksID = channelIDs[domain.SlugTasks]
	s.planningID = channelIDs[domain.SlugPlanning]
	s.generalID = channelIDs[domain.SlugGeneral]
	s.observerID = channelIDs[domain.SlugObserver]

	// Create child_of dependencies for non-root channels
	for slug, id := range channelIDs {
		if slug == domain.SlugRoot {
			continue
		}
		dep := domain.NewDependency(id, s.rootID, domain.RelationChildOf)
		if err := s.dependencies.Add(dep); err != nil {
			return fmt.Errorf("add dependency for %s: %w", slug, err)
		}
	}

	// Auto-subscribe coordinator to #system with mode=all
	// This ensures coordinator gets notified when workers signal ready
	if _, err := s.subscriptions.Subscribe(s.systemID, createdBy, domain.ModeAll); err != nil {
		return fmt.Errorf("subscribe coordinator to system: %w", err)
	}

	return nil
}

// GetChannel returns a channel by slug.
func (s *Service) GetChannel(slug string) (*domain.Thread, error) {
	return s.threads.GetBySlug(slug)
}

// GetChannelID returns the ID for a channel slug.
func (s *Service) GetChannelID(slug string) string {
	switch slug {
	case domain.SlugRoot:
		return s.rootID
	case domain.SlugSystem:
		return s.systemID
	case domain.SlugTasks:
		return s.tasksID
	case domain.SlugPlanning:
		return s.planningID
	case domain.SlugGeneral:
		return s.generalID
	case domain.SlugObserver:
		return s.observerID
	default:
		return ""
	}
}

// GetChannelSlug returns the slug for a channel ID.
func (s *Service) GetChannelSlug(channelID string) string {
	switch channelID {
	case s.rootID:
		return domain.SlugRoot
	case s.systemID:
		return domain.SlugSystem
	case s.tasksID:
		return domain.SlugTasks
	case s.planningID:
		return domain.SlugPlanning
	case s.generalID:
		return domain.SlugGeneral
	case s.observerID:
		return domain.SlugObserver
	default:
		return ""
	}
}

// SendMessageInput contains parameters for sending a message.
type SendMessageInput struct {
	ChannelSlug string
	Content     string
	Kind        domain.MessageKind
	CreatedBy   string
	Mentions    []string
	Meta        map[string]string
}

// SendMessage posts a new message to a channel.
func (s *Service) SendMessage(input SendMessageInput) (*domain.Thread, error) {
	channelID := s.GetChannelID(input.ChannelSlug)
	if channelID == "" {
		return nil, fmt.Errorf("unknown channel: %s", input.ChannelSlug)
	}

	if input.Kind == "" {
		input.Kind = domain.KindInfo
	}

	// Parse mentions from content if not provided
	mentions := input.Mentions
	if len(mentions) == 0 {
		mentions = parseMentions(input.Content)
	}

	// Build initial participants: sender + all mentioned agents
	participants := make([]string, 0, 1+len(mentions))
	participants = append(participants, input.CreatedBy)
	for _, m := range mentions {
		if m != input.CreatedBy {
			participants = append(participants, m)
		}
	}

	msg := domain.Thread{
		Type:         domain.ThreadMessage,
		Content:      input.Content,
		Kind:         string(input.Kind),
		CreatedBy:    input.CreatedBy,
		CreatedAt:    time.Now(),
		Mentions:     mentions,
		Participants: participants,
		Meta:         input.Meta,
	}

	created, err := s.threads.Create(msg)
	if err != nil {
		return nil, fmt.Errorf("create message: %w", err)
	}

	dep := domain.NewDependency(created.ID, channelID, domain.RelationChildOf)
	if err := s.dependencies.Add(dep); err != nil {
		return nil, fmt.Errorf("add dependency: %w", err)
	}

	s.emit(NewMessagePostedEvent(created, channelID, input.ChannelSlug))

	return created, nil
}

// ReplyInput contains parameters for replying to a message.
type ReplyInput struct {
	MessageID string
	Content   string
	Kind      domain.MessageKind
	CreatedBy string
	Mentions  []string
	Meta      map[string]string
}

// Reply posts a reply to an existing message thread.
// TODO: Currently all replies are flattened to point to the root message (single-level threading).
// Future enhancement: support configurable nesting depth or true nested threading.
func (s *Service) Reply(input ReplyInput) (*domain.Thread, error) {
	parent, err := s.threads.Get(input.MessageID)
	if err != nil {
		return nil, fmt.Errorf("get parent message: %w", err)
	}

	if parent.Type != domain.ThreadMessage {
		return nil, fmt.Errorf("can only reply to messages, got %s", parent.Type)
	}

	// Find the root message of this thread (flatten all replies to single level)
	rootID := s.findThreadRoot(input.MessageID)
	if rootID == "" {
		rootID = input.MessageID // No parent found, this IS the root
	}

	// Get root for participant tracking
	root := parent
	if rootID != input.MessageID {
		root, err = s.threads.Get(rootID)
		if err != nil {
			return nil, fmt.Errorf("get root message: %w", err)
		}
	}

	if input.Kind == "" {
		input.Kind = domain.KindResponse
	}

	mentions := input.Mentions
	if len(mentions) == 0 {
		mentions = parseMentions(input.Content)
	}

	reply := domain.Thread{
		Type:      domain.ThreadMessage,
		Content:   input.Content,
		Kind:      string(input.Kind),
		CreatedBy: input.CreatedBy,
		CreatedAt: time.Now(),
		Mentions:  mentions,
		Meta:      input.Meta,
	}

	created, err := s.threads.Create(reply)
	if err != nil {
		return nil, fmt.Errorf("create reply: %w", err)
	}

	// Always point to root message for flat threading
	dep := domain.NewDependency(created.ID, rootID, domain.RelationReplyTo)
	if err := s.dependencies.Add(dep); err != nil {
		return nil, fmt.Errorf("add reply dependency: %w", err)
	}

	// Add replier as participant to root thread (so they see future replies)
	// Also add any newly mentioned agents
	root.AddParticipant(input.CreatedBy)
	root.AddParticipants(mentions...)
	// Update is best-effort for participant notifications; reply already created successfully
	_, _ = s.threads.Update(*root)

	// Find the channel this message belongs to
	channelID := s.findChannelForMessage(rootID)
	channelSlug := s.GetChannelSlug(channelID)

	// Pass root's participants so broker can notify them of the reply
	s.emit(NewReplyPostedEvent(created, channelID, channelSlug, rootID, root.Participants))

	return created, nil
}

// findThreadRoot traverses reply_to edges to find the root message of a thread.
// Returns the root message ID, or empty string if messageID has no parent.
func (s *Service) findThreadRoot(messageID string) string {
	relation := domain.RelationReplyTo
	current := messageID

	for {
		deps, err := s.dependencies.GetParents(current, &relation)
		if err != nil || len(deps) == 0 {
			// No parent - current is the root (or we started at root)
			if current == messageID {
				return "" // Started at root
			}
			return current
		}
		current = deps[0].DependsOnID
	}
}

// AttachArtifactInput contains parameters for attaching an artifact.
type AttachArtifactInput struct {
	TargetID  string // Channel or message ID
	Path      string // Absolute file path
	Name      string // Optional display name (defaults to basename)
	CreatedBy string
	Meta      map[string]string
}

// AttachArtifact attaches a file reference to a channel or message.
// The file is not copied - we store a reference to the absolute path.
func (s *Service) AttachArtifact(input AttachArtifactInput) (*domain.Thread, error) {
	// Validate file exists and get metadata
	info, err := osStatFunc(input.Path)
	if err != nil {
		return nil, fmt.Errorf("stat file: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("path is a directory: %s", input.Path)
	}

	// Compute SHA256
	sha256Hash, err := computeFileSHA256(input.Path)
	if err != nil {
		return nil, fmt.Errorf("compute sha256: %w", err)
	}

	// Determine display name
	name := input.Name
	if name == "" {
		name = filepath.Base(input.Path)
	}

	// Infer media type from extension
	mediaType := inferMediaType(input.Path)

	artifact := domain.Thread{
		Type:       domain.ThreadArtifact,
		Name:       name,
		MediaType:  mediaType,
		SizeBytes:  info.Size(),
		StorageURI: "file://" + input.Path,
		Sha256:     sha256Hash,
		CreatedBy:  input.CreatedBy,
		CreatedAt:  time.Now(),
		Meta:       input.Meta,
	}

	created, err := s.threads.Create(artifact)
	if err != nil {
		return nil, fmt.Errorf("create artifact: %w", err)
	}

	dep := domain.NewDependency(created.ID, input.TargetID, domain.RelationReferences)
	if err := s.dependencies.Add(dep); err != nil {
		return nil, fmt.Errorf("add artifact dependency: %w", err)
	}

	s.emit(NewArtifactAddedEvent(created, input.TargetID))

	return created, nil
}

// osStatFunc is a variable for testing.
var osStatFunc = os.Stat

// computeFileSHA256 computes the SHA256 hash of a file.
func computeFileSHA256(path string) (string, error) {
	f, err := os.Open(path) //nolint:gosec // path is validated by caller
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// inferMediaType returns a MIME type based on file extension.
func inferMediaType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return "text/x-go"
	case ".md":
		return "text/markdown"
	case ".json":
		return "application/json"
	case ".yaml", ".yml":
		return "application/x-yaml"
	case ".diff", ".patch":
		return "text/x-diff"
	case ".txt":
		return "text/plain"
	case ".sh":
		return "text/x-shellscript"
	case ".py":
		return "text/x-python"
	case ".js":
		return "text/javascript"
	case ".ts":
		return "text/typescript"
	case ".html":
		return "text/html"
	case ".css":
		return "text/css"
	case ".xml":
		return "application/xml"
	case ".log":
		return "text/plain"
	default:
		return "application/octet-stream"
	}
}

// GetThread retrieves a thread by ID.
func (s *Service) GetThread(id string) (*domain.Thread, error) {
	return s.threads.Get(id)
}

// GetReplies returns all replies to a message.
func (s *Service) GetReplies(messageID string) ([]domain.Thread, error) {
	relation := domain.RelationReplyTo
	deps, err := s.dependencies.GetChildren(messageID, &relation)
	if err != nil {
		return nil, err
	}

	replies := make([]domain.Thread, 0, len(deps))
	for _, dep := range deps {
		thread, err := s.threads.Get(dep.ThreadID)
		if err != nil {
			continue
		}
		replies = append(replies, *thread)
	}

	return replies, nil
}

// GetArtifacts returns all artifacts attached to a channel or message.
func (s *Service) GetArtifacts(targetID string) ([]domain.Thread, error) {
	relation := domain.RelationReferences
	deps, err := s.dependencies.GetChildren(targetID, &relation)
	if err != nil {
		return nil, err
	}

	artifacts := make([]domain.Thread, 0, len(deps))
	for _, dep := range deps {
		thread, err := s.threads.Get(dep.ThreadID)
		if err != nil {
			continue
		}
		if thread.Type == domain.ThreadArtifact {
			artifacts = append(artifacts, *thread)
		}
	}

	return artifacts, nil
}

// GetArtifactContent retrieves the content of an artifact by reading from disk.
func (s *Service) GetArtifactContent(artifactID string) ([]byte, error) {
	thread, err := s.threads.Get(artifactID)
	if err != nil {
		return nil, fmt.Errorf("get artifact: %w", err)
	}

	if thread.Type != domain.ThreadArtifact {
		return nil, fmt.Errorf("thread is not an artifact: %s", artifactID)
	}

	if thread.StorageURI == "" {
		return nil, fmt.Errorf("artifact has no storage URI: %s", artifactID)
	}

	// Parse file:// URI
	path := strings.TrimPrefix(thread.StorageURI, "file://")
	if path == thread.StorageURI {
		return nil, fmt.Errorf("unsupported storage URI scheme: %s", thread.StorageURI)
	}

	content, err := os.ReadFile(path) //nolint:gosec // path from trusted storage
	if err != nil {
		return nil, fmt.Errorf("read artifact file: %w", err)
	}

	return content, nil
}

// ListMessages returns messages in a channel.
func (s *Service) ListMessages(channelSlug string, limit int) ([]domain.Thread, error) {
	channelID := s.GetChannelID(channelSlug)
	if channelID == "" {
		return nil, fmt.Errorf("unknown channel: %s", channelSlug)
	}

	childOf := domain.RelationChildOf
	deps, err := s.dependencies.GetChildren(channelID, &childOf)
	if err != nil {
		return nil, err
	}

	messages := make([]domain.Thread, 0, len(deps))
	for _, dep := range deps {
		thread, err := s.threads.Get(dep.ThreadID)
		if err != nil {
			continue
		}
		if thread.Type == domain.ThreadMessage {
			messages = append(messages, *thread)
		}
	}

	// Sort by Seq
	for i := 0; i < len(messages); i++ {
		for j := i + 1; j < len(messages); j++ {
			if messages[i].Seq > messages[j].Seq {
				messages[i], messages[j] = messages[j], messages[i]
			}
		}
	}

	if limit > 0 && len(messages) > limit {
		messages = messages[:limit]
	}

	return messages, nil
}

// Ack marks messages as acknowledged by an agent.
func (s *Service) Ack(agentID string, messageIDs ...string) error {
	if err := s.acks.Ack(agentID, messageIDs...); err != nil {
		return err
	}
	s.emit(NewAckedEvent(agentID, messageIDs))
	return nil
}

// GetUnacked returns unacked message counts by channel for an agent.
func (s *Service) GetUnacked(agentID string) (map[string]repository.UnackedSummary, error) {
	return s.acks.GetUnacked(agentID)
}

// Subscribe subscribes an agent to a channel.
func (s *Service) Subscribe(channelSlug, agentID string, mode domain.SubscriptionMode) (*domain.Subscription, error) {
	channelID := s.GetChannelID(channelSlug)
	if channelID == "" {
		return nil, fmt.Errorf("unknown channel: %s", channelSlug)
	}
	sub, err := s.subscriptions.Subscribe(channelID, agentID, mode)
	if err != nil {
		return nil, err
	}
	s.emit(NewSubscribedEvent(sub, channelSlug))
	return sub, nil
}

// Unsubscribe removes an agent's subscription to a channel.
func (s *Service) Unsubscribe(channelSlug, agentID string) error {
	channelID := s.GetChannelID(channelSlug)
	if channelID == "" {
		return fmt.Errorf("unknown channel: %s", channelSlug)
	}
	if err := s.subscriptions.Unsubscribe(channelID, agentID); err != nil {
		return err
	}
	s.emit(NewUnsubscribedEvent(channelID, channelSlug, agentID))
	return nil
}

// GetSubscriptions returns all subscriptions for an agent.
func (s *Service) GetSubscriptions(agentID string) ([]domain.Subscription, error) {
	return s.subscriptions.ListForAgent(agentID)
}

// UnsubscribeAll removes all subscriptions for an agent.
// This is used to clean up when an agent (e.g., Observer) is stopped.
func (s *Service) UnsubscribeAll(agentID string) error {
	subs, err := s.subscriptions.ListForAgent(agentID)
	if err != nil {
		return fmt.Errorf("listing subscriptions for %s: %w", agentID, err)
	}

	for _, sub := range subs {
		if err := s.subscriptions.Unsubscribe(sub.ChannelID, agentID); err != nil {
			// Log but continue - best effort cleanup
			continue
		}
		channelSlug := s.GetChannelSlug(sub.ChannelID)
		s.emit(NewUnsubscribedEvent(sub.ChannelID, channelSlug, agentID))
	}

	return nil
}

// findChannelForMessage traverses child_of to find the channel.
func (s *Service) findChannelForMessage(messageID string) string {
	relation := domain.RelationChildOf
	deps, err := s.dependencies.GetParents(messageID, &relation)
	if err != nil || len(deps) == 0 {
		return ""
	}
	return deps[0].DependsOnID
}

// mentionPattern matches @agent-id or @AGENT.ID patterns.
var mentionPattern = regexp.MustCompile(`@([a-zA-Z0-9._-]+)`)

// parseMentions extracts @mentions from content.
// Mentions are normalized to lowercase to match process IDs (e.g., worker-1, coordinator).
func parseMentions(content string) []string {
	matches := mentionPattern.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	var mentions []string

	for _, match := range matches {
		if len(match) > 1 {
			mention := strings.ToLower(match[1])
			if !seen[mention] {
				seen[mention] = true
				mentions = append(mentions, mention)
			}
		}
	}

	return mentions
}
