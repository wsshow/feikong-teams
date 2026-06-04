package cli

import (
	"fkteams/eventlog"
	"fkteams/eventview"
	"fkteams/fkevent"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/pterm/pterm"
)

// QueryView 是查询执行器的输出端口。执行器只负责生命周期，终端如何展示由 View 决定。
type QueryView interface {
	Start(input string)
	EventCallback(recorder *eventlog.HistoryRecorder) func(fkevent.Event) error
	Flush()
	Interrupted()
	Error(error)
	Done(time.Duration)
	CancelRequested()
	AutoReject()
}

// TerminalQueryView 保留传统非交互输出方式，交互 TUI runtime 不再使用它。
type TerminalQueryView struct {
	spinner     *pterm.SpinnerPrinter
	stopSpinner func()
}

func NewTerminalQueryView() *TerminalQueryView {
	return &TerminalQueryView{}
}

func (v *TerminalQueryView) Start(input string) {
	fmt.Println()
	spinner, _ := pterm.DefaultSpinner.Start("思考中...")
	v.spinner = spinner
	v.stopSpinner = sync.OnceFunc(func() { spinner.Stop() })
}

func (v *TerminalQueryView) EventCallback(recorder *eventlog.HistoryRecorder) func(fkevent.Event) error {
	return func(event fkevent.Event) error {
		if v.stopSpinner != nil {
			v.stopSpinner()
		}
		recorder.RecordEvent(event)
		eventview.PrintEvent(event)
		return nil
	}
}

func (v *TerminalQueryView) Flush() {
	eventview.FlushPrintEvent()
}

func (v *TerminalQueryView) Interrupted() {
	if v.stopSpinner != nil {
		v.stopSpinner()
	}
	pterm.Warning.Println("查询已中断")
}

func (v *TerminalQueryView) Error(err error) {
	if v.stopSpinner != nil {
		v.stopSpinner()
	}
	log.Printf("执行出错: %v", err)
}

func (v *TerminalQueryView) Done(elapsed time.Duration) {
	if v.stopSpinner != nil {
		v.stopSpinner()
	}
	fmt.Printf("\n\033[1;32m✓ 完成\033[0m \033[90m(%s)\033[0m\n", elapsed)
}

func (v *TerminalQueryView) CancelRequested() {
	fmt.Printf("\n\n")
	pterm.Info.Println("正在中断查询...")
}

func (v *TerminalQueryView) AutoReject() {
	pterm.Warning.Println("非交互模式，自动拒绝危险命令")
}

type callbackQueryView struct {
	callbackBuilder func(*eventlog.HistoryRecorder) func(fkevent.Event) error
}

func (v callbackQueryView) Start(input string) {}

func (v callbackQueryView) EventCallback(recorder *eventlog.HistoryRecorder) func(fkevent.Event) error {
	if v.callbackBuilder == nil {
		return func(event fkevent.Event) error {
			recorder.RecordEvent(event)
			return nil
		}
	}
	return v.callbackBuilder(recorder)
}

func (v callbackQueryView) Flush() {}

func (v callbackQueryView) Interrupted() {}

func (v callbackQueryView) Error(err error) {
	log.Printf("执行出错: %v", err)
}

func (v callbackQueryView) Done(elapsed time.Duration) {}

func (v callbackQueryView) CancelRequested() {}

func (v callbackQueryView) AutoReject() {}
