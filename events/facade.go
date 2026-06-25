package events

import runtimeevents "fkteams/internal/runtime/events"

type FriendlyError = runtimeevents.FriendlyError
type Emitter = runtimeevents.Emitter
type MessageEvent = runtimeevents.MessageEvent
type ToolEvent = runtimeevents.ToolEvent

var WithCallback = runtimeevents.WithCallback
var WithNonInteractive = runtimeevents.WithNonInteractive
var IsNonInteractive = runtimeevents.IsNonInteractive
var NormalizeEvent = runtimeevents.NormalizeEvent
var IsMemberEvent = runtimeevents.IsMemberEvent
var DispatchEvent = runtimeevents.DispatchEvent
var Dispatch = runtimeevents.Dispatch
var IsInternalToolName = runtimeevents.IsInternalToolName
var IsInternalContinueContent = runtimeevents.IsInternalContinueContent
var NewEmitter = runtimeevents.NewEmitter
var AgentStart = runtimeevents.AgentStart
var AgentEnd = runtimeevents.AgentEnd
var AgentError = runtimeevents.AgentError
var TurnStart = runtimeevents.TurnStart
var TurnEnd = runtimeevents.TurnEnd
var MessageStart = runtimeevents.MessageStart
var MessageDelta = runtimeevents.MessageDelta
var MessageEnd = runtimeevents.MessageEnd
var ToolStart = runtimeevents.ToolStart
var ToolUpdate = runtimeevents.ToolUpdate
var ToolEnd = runtimeevents.ToolEnd
var Action = runtimeevents.Action
var Error = runtimeevents.Error
var Usage = runtimeevents.Usage
var UserMessagePair = runtimeevents.UserMessagePair
var TurnID = runtimeevents.TurnID
var ToolCallsFromEvent = runtimeevents.ToolCallsFromEvent
var ToolCallRefAt = runtimeevents.ToolCallRefAt
var ValidateEventContract = runtimeevents.ValidateEventContract
var NormalizeFriendlyError = runtimeevents.NormalizeFriendlyError
