package events

import domainevent "fkteams/internal/domain/event"

type EventType = domainevent.Type

const (
	EventAgentStarted       = domainevent.TypeAgentStarted
	EventAgentCompleted     = domainevent.TypeAgentCompleted
	EventTurnStarted        = domainevent.TypeTurnStarted
	EventTurnCompleted      = domainevent.TypeTurnCompleted
	EventTurnFailed         = domainevent.TypeTurnFailed
	EventTurnCancelled      = domainevent.TypeTurnCancelled
	EventUserMessage        = domainevent.TypeUserMessage
	EventAssistantStarted   = domainevent.TypeAssistantStarted
	EventAssistantReasoning = domainevent.TypeAssistantReasoning
	EventAssistantText      = domainevent.TypeAssistantText
	EventAssistantCompleted = domainevent.TypeAssistantCompleted
	EventToolCallStarted    = domainevent.TypeToolCallStarted
	EventToolCallArguments  = domainevent.TypeToolCallArguments
	EventToolCallResult     = domainevent.TypeToolCallResult
	EventToolCallCompleted  = domainevent.TypeToolCallCompleted
	EventToolCallFailed     = domainevent.TypeToolCallFailed
	EventAskRequested       = domainevent.TypeAskRequested
	EventAskAnswered        = domainevent.TypeAskAnswered
	EventApprovalRequested  = domainevent.TypeApprovalRequested
	EventApprovalAnswered   = domainevent.TypeApprovalAnswered
	EventMemberStarted      = domainevent.TypeMemberStarted
	EventMemberCompleted    = domainevent.TypeMemberCompleted
	EventQueueUpdated       = domainevent.TypeQueueUpdated
	EventSystemNotice       = domainevent.TypeSystemNotice
	EventUsageReported      = domainevent.TypeUsageReported
	EventError              = domainevent.TypeError
)

type DeltaKind = domainevent.DeltaKind

const (
	DeltaOutput     = domainevent.DeltaOutput
	DeltaReasoning  = domainevent.DeltaReasoning
	DeltaToolArgs   = domainevent.DeltaToolArgs
	DeltaToolResult = domainevent.DeltaToolResult
)

type NotifyType = domainevent.NotifyType

const (
	NotifyProcessingStart  = domainevent.NotifyProcessingStart
	NotifyProcessingEnd    = domainevent.NotifyProcessingEnd
	NotifyUserMessage      = domainevent.NotifyUserMessage
	NotifyQueueUpdated     = domainevent.NotifyQueueUpdated
	NotifyCancelled        = domainevent.NotifyCancelled
	NotifyError            = domainevent.NotifyError
	NotifyApprovalRequired = domainevent.NotifyApprovalRequired
	NotifyConnected        = domainevent.NotifyConnected
	NotifyPong             = domainevent.NotifyPong
	NotifyInvalidAPIKey    = domainevent.NotifyInvalidAPIKey
)

type Event = domainevent.Event
type AskPayload = domainevent.AskPayload
type ApprovalPayload = domainevent.ApprovalPayload
type UsagePayload = domainevent.UsagePayload
type NoticePayload = domainevent.NoticePayload
