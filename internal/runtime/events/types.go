package events

import domainevent "fkteams/internal/domain/event"

type EventType = domainevent.Type

const (
	EventAgentStart   = domainevent.TypeAgentStart
	EventAgentEnd     = domainevent.TypeAgentEnd
	EventTurnStart    = domainevent.TypeTurnStart
	EventTurnEnd      = domainevent.TypeTurnEnd
	EventMessageStart = domainevent.TypeMessageStart
	EventMessageDelta = domainevent.TypeMessageDelta
	EventMessageEnd   = domainevent.TypeMessageEnd
	EventToolStart    = domainevent.TypeToolStart
	EventToolUpdate   = domainevent.TypeToolUpdate
	EventToolEnd      = domainevent.TypeToolEnd
	EventAction       = domainevent.TypeAction
	EventUsage        = domainevent.TypeUsage
	EventError        = domainevent.TypeError
	EventMemberUpdate = domainevent.TypeMemberUpdate
)

type DeltaKind = domainevent.DeltaKind

const (
	DeltaOutput     = domainevent.DeltaOutput
	DeltaReasoning  = domainevent.DeltaReasoning
	DeltaToolArgs   = domainevent.DeltaToolArgs
	DeltaToolResult = domainevent.DeltaToolResult
)

type ActionType = domainevent.ActionType

const (
	ActionTransfer             = domainevent.ActionTransfer
	ActionInterrupted          = domainevent.ActionInterrupted
	ActionExit                 = domainevent.ActionExit
	ActionAskQuestions         = domainevent.ActionAskQuestions
	ActionAskResponse          = domainevent.ActionAskResponse
	ActionApprovalRequired     = domainevent.ActionApprovalRequired
	ActionApprovalDecision     = domainevent.ActionApprovalDecision
	ActionContextCompressStart = domainevent.ActionContextCompressStart
	ActionContextCompress      = domainevent.ActionContextCompress
)

type NotifyType = domainevent.NotifyType

const (
	NotifyProcessingStart  = domainevent.NotifyProcessingStart
	NotifyProcessingEnd    = domainevent.NotifyProcessingEnd
	NotifyUserMessage      = domainevent.NotifyUserMessage
	NotifyQueueUpdated     = domainevent.NotifyQueueUpdated
	NotifyCancelled        = domainevent.NotifyCancelled
	NotifyError            = domainevent.NotifyError
	NotifyAskQuestions     = domainevent.NotifyAskQuestions
	NotifyApprovalRequired = domainevent.NotifyApprovalRequired
	NotifyConnected        = domainevent.NotifyConnected
	NotifyPong             = domainevent.NotifyPong
	NotifyInvalidAPIKey    = domainevent.NotifyInvalidAPIKey
)

type Event = domainevent.Event
