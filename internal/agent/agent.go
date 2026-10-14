package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Songxwn/maya-auto/internal/newapi"
	"github.com/Songxwn/maya-auto/internal/tools"
)

const systemPrompt = `你是 Maya Auto，负责操作已经打开的 Autodesk Maya 2024 或更高版本。你只能通过给定工具改变场景。
规则：
- 修改前先调用 maya_status 或 maya_list，不要假设对象已经存在。
- 建模、材质、摄影机、粒子、布料、力场、逐帧解算、烘焙、灯光和渲染都使用对应工具。
- 做完后用简短中文说明实际改了什么。
- 工具报错时最多根据错误调整并重试一次。
- 不要声称执行了没有工具结果支持的操作。`

type Event struct {
	Kind   string `json:"kind"`
	Text   string `json:"text,omitempty"`
	Name   string `json:"name,omitempty"`
	Args   any    `json:"args,omitempty"`
	Result any    `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

type Runner func(name string, args map[string]any, wait time.Duration) (json.RawMessage, error)

func Run(ctx context.Context, client *newapi.Client, model, prompt string, run Runner, emit func(Event)) error {
	model = strings.TrimSpace(model)
	prompt = strings.TrimSpace(prompt)
	if model == "" {
		return fmt.Errorf("未选择 NewAPI 模型")
	}
	if prompt == "" {
		return fmt.Errorf("指令为空")
	}
	if client == nil || client.BaseURL == "" || client.APIKey == "" {
		return fmt.Errorf("请先保存 NewAPI 地址和密钥")
	}
	messages := []newapi.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: newapi.FlexString(prompt)},
	}
	toolDefs := tools.OpenAI()
	for round := 0; round < 8; round++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		emit(Event{Kind: "status", Text: fmt.Sprintf("正在请求模型，第 %d 轮", round+1)})
		roundCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
		result, err := client.Chat(roundCtx, model, messages, toolDefs)
		cancel()
		if err != nil {
			return err
		}
		assistant := newapi.ChatMessage{Role: "assistant", Content: newapi.FlexString(result.Content), ToolCalls: result.ToolCalls}
		messages = append(messages, assistant)
		if strings.TrimSpace(result.Content) != "" {
			emit(Event{Kind: "assistant", Text: result.Content})
		}
		if len(result.ToolCalls) == 0 {
			emit(Event{Kind: "done", Text: result.UsageText})
			return nil
		}
		for _, call := range result.ToolCalls {
			args, err := newapi.ParseToolArgs(call.Function.Arguments)
			if err != nil {
				emit(Event{Kind: "tool_error", Name: call.Function.Name, Error: err.Error()})
				messages = append(messages, newapi.ChatMessage{Role: "tool", ToolCallID: call.ID, Name: call.Function.Name, Content: newapi.FlexString("参数不是 JSON 对象: " + err.Error())})
				continue
			}
			if _, ok := tools.Find(call.Function.Name); !ok {
				emit(Event{Kind: "tool_error", Name: call.Function.Name, Error: "未知工具"})
				messages = append(messages, newapi.ChatMessage{Role: "tool", ToolCallID: call.ID, Name: call.Function.Name, Content: "未知工具"})
				continue
			}
			emit(Event{Kind: "tool_call", Name: call.Function.Name, Args: args})
			raw, callErr := run(call.Function.Name, args, tools.Timeout(call.Function.Name))
			content := string(raw)
			if callErr != nil {
				content = callErr.Error()
				emit(Event{Kind: "tool_error", Name: call.Function.Name, Error: content})
			} else {
				var parsed any
				if json.Unmarshal(raw, &parsed) == nil {
					emit(Event{Kind: "tool_result", Name: call.Function.Name, Result: parsed})
				} else {
					emit(Event{Kind: "tool_result", Name: call.Function.Name, Result: content})
				}
			}
			if len(content) > 12000 {
				content = content[:12000] + "...truncated"
			}
			messages = append(messages, newapi.ChatMessage{Role: "tool", ToolCallID: call.ID, Name: call.Function.Name, Content: newapi.FlexString(content)})
		}
	}
	emit(Event{Kind: "done", Text: "已达到工具调用轮数上限"})
	return nil
}
