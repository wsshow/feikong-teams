// Package engine 保留旧入口名称，实际执行内核位于 internal/runtime/turn。
package engine

import "fkteams/internal/runtime/turn"

type EventHandler = turn.EventHandler
type StartHandler = turn.StartHandler
type FinishHandler = turn.FinishHandler
type ContextHook = turn.ContextHook
type TurnInput = turn.TurnInput
type HistorySink = turn.HistorySink
type InterruptHandler = turn.InterruptHandler
type InterruptInfoHandler = turn.InterruptInfoHandler
type Session = turn.Session

var NewSession = turn.NewSession
var FixedDecisionHandler = turn.FixedDecisionHandler
var ChannelHandler = turn.ChannelHandler
var ChannelTargetHandler = turn.ChannelTargetHandler
var CallbackHandler = turn.CallbackHandler
var InfoHandler = turn.InfoHandler
