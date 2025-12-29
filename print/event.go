package print

import (
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

// Event 打印智能体事件信息
func Event(event *adk.AgentEvent) {
	fmt.Printf("name: %s\npath: %s", event.AgentName, event.RunPath)
	if event.Output != nil && event.Output.MessageOutput != nil {
		if m := event.Output.MessageOutput.Message; m != nil {
			if len(m.Content) > 0 {
				if m.Role == schema.Tool {
					fmt.Printf("\ntool response: %s", m.Content)
				} else {
					fmt.Printf("\nanswer: %s", m.Content)
				}
			}
			if len(m.ToolCalls) > 0 {
				for _, tc := range m.ToolCalls {
					fmt.Printf("\ntool name: %s", tc.Function.Name)
					fmt.Printf("\narguments: %s", tc.Function.Arguments)
				}
			}
		} else if s := event.Output.MessageOutput.MessageStream; s != nil {
			toolMap := map[int][]*schema.Message{}
			var contentStart bool
			charNumOfOneRow := 0
			maxCharNumOfOneRow := 120
			for {
				chunk, err := s.Recv()
				if err != nil {
					if err == io.EOF {
						break
					}
					fmt.Printf("error: %v", err)
					return
				}
				if chunk.Content != "" {
					if !contentStart {
						contentStart = true
						if chunk.Role == schema.Tool {
							fmt.Printf("\ntool response: ")
						} else {
							fmt.Printf("\nanswer: ")
						}
					}

					charNumOfOneRow += len(chunk.Content)
					if strings.Contains(chunk.Content, "\n") {
						charNumOfOneRow = 0
					} else if charNumOfOneRow >= maxCharNumOfOneRow {
						fmt.Printf("\n")
						charNumOfOneRow = 0
					}
					fmt.Printf("%v", chunk.Content)
				}

				if len(chunk.ToolCalls) > 0 {
					for _, tc := range chunk.ToolCalls {
						index := tc.Index
						if index == nil {
							log.Fatalf("index is nil")
						}
						// 长参数时的流式输出
						fmt.Print(tc.Function.Arguments)
						toolMap[*index] = append(toolMap[*index], &schema.Message{
							Role: chunk.Role,
							ToolCalls: []schema.ToolCall{
								{
									ID:    tc.ID,
									Type:  tc.Type,
									Index: tc.Index,
									Function: schema.FunctionCall{
										Name:      tc.Function.Name,
										Arguments: tc.Function.Arguments,
									},
								},
							},
						})
					}
				}
			}

			for _, msgs := range toolMap {
				m, err := schema.ConcatMessages(msgs)
				if err != nil {
					log.Fatalf("ConcatMessage failed: %v", err)
					return
				}
				fmt.Printf("\ntool name: %s", m.ToolCalls[0].Function.Name)
				fmt.Printf("\narguments: %s", m.ToolCalls[0].Function.Arguments)
			}
		}
	}
	if event.Action != nil {
		if event.Action.TransferToAgent != nil {
			fmt.Printf("\naction: transfer to %v", event.Action.TransferToAgent.DestAgentName)
		}
		if event.Action.Interrupted != nil {
			for _, ic := range event.Action.Interrupted.InterruptContexts {
				str, ok := ic.Info.(fmt.Stringer)
				if ok {
					fmt.Printf("\n%s", str.String())
				} else {
					fmt.Printf("\n%v", ic.Info)
				}
			}
		}
		if event.Action.Exit {
			fmt.Printf("\naction: exit")
		}
	}
	if event.Err != nil {
		fmt.Printf("\nerror: %v", event.Err)
	}
	fmt.Println()
	fmt.Println()
}
