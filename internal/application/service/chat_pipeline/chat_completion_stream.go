package chatpipeline

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/modelcontext"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
)

// EmptyTruncatedAnswerFallback explains an output-budget exhaustion with no answer text.
const EmptyTruncatedAnswerFallback = "Sorry, this answer hit the model's per-response output limit " +
	"before any text was produced. Try narrowing the question, or raise max_completion_tokens."

// IsLengthFinishReason reports whether a provider ended a response because its
// completion-token budget was exhausted.
func IsLengthFinishReason(reason string) bool {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "length", "max_tokens", "max_output_tokens":
		return true
	default:
		return false
	}
}

// PluginChatCompletionStream implements streaming chat completion functionality
// as a plugin that can be registered to EventManager
type PluginChatCompletionStream struct {
	modelService interfaces.ModelService // Interface for model operations
}

// NewPluginChatCompletionStream creates a new PluginChatCompletionStream instance
// and registers it with the EventManager
func NewPluginChatCompletionStream(eventManager *EventManager,
	modelService interfaces.ModelService,
) *PluginChatCompletionStream {
	res := &PluginChatCompletionStream{
		modelService: modelService,
	}
	eventManager.Register(res)
	return res
}

// ActivationEvents returns the event types this plugin handles
func (p *PluginChatCompletionStream) ActivationEvents() []types.EventType {
	return []types.EventType{types.CHAT_COMPLETION_STREAM}
}

// OnEvent handles streaming chat completion events
// It prepares the chat model, messages, and initiates streaming response
func (p *PluginChatCompletionStream) OnEvent(ctx context.Context,
	eventType types.EventType, chatManage *types.ChatManage, next func() *PluginError,
) *PluginError {
	pipelineInfo(ctx, "Stream", "input", map[string]interface{}{
		"session_id":     chatManage.SessionID,
		"user_question":  chatManage.UserContent,
		"history_rounds": len(chatManage.History),
		"chat_model":     chatManage.ChatModelID,
	})

	// Prepare chat model and options
	chatModel, opt, err := prepareChatModel(ctx, p.modelService, chatManage)
	if err != nil {
		return ErrGetChatModel.WithError(err)
	}

	// Prepare base messages without history

	chatMessages, modelContext := prepareMessagesWithModelContext(ctx, chatManage)
	chatMessages = modelContext.EncodeMessages(chatMessages)
	reportModelContextLeaks(ctx, "Stream", modelContext, chatMessages)
	ctx = withPromptCacheMetadata(ctx, chatModel, chatMessages, opt, "knowledge_qa")
	pipelineInfo(ctx, "Stream", "messages_ready", map[string]interface{}{
		"message_count": len(chatMessages),
		"system_prompt": chatMessages[0].Content,
	})
	pipelineInfo(ctx, "Stream", "user_message", map[string]interface{}{
		"content": chatMessages[len(chatMessages)-1].Content,
	})
	// EventBus is required for event-driven streaming
	if chatManage.EventBus == nil {
		pipelineError(ctx, "Stream", "eventbus_missing", map[string]interface{}{
			"session_id": chatManage.SessionID,
		})
		return ErrModelCall.WithError(errors.New("EventBus is required for streaming"))
	}
	eventBus := chatManage.EventBus

	pipelineInfo(ctx, "Stream", "eventbus_ready", map[string]interface{}{
		"session_id": chatManage.SessionID,
	})

	// Initiate streaming chat model call with independent context
	pipelineInfo(ctx, "Stream", "model_call", map[string]interface{}{
		"chat_model": chatManage.ChatModelID,
	})
	responseChan, err := chatModel.ChatStream(ctx, chatMessages, opt)
	if err != nil {
		pipelineError(ctx, "Stream", "model_call", map[string]interface{}{
			"chat_model": chatManage.ChatModelID,
			"error":      err.Error(),
		})
		return ErrModelCall.WithError(err)
	}
	if responseChan == nil {
		pipelineError(ctx, "Stream", "model_call", map[string]interface{}{
			"chat_model": chatManage.ChatModelID,
			"error":      "nil_channel",
		})
		return ErrModelCall.WithError(errors.New("chat stream returned nil channel"))
	}

	pipelineInfo(ctx, "Stream", "model_started", map[string]interface{}{
		"session_id": chatManage.SessionID,
	})

	// Start goroutine to consume channel and emit events directly.
	// reasoning_content is routed to EventAgentThought (SSE response_type=thinking)
	// and plain answer text to EventAgentFinalAnswer, matching the Agent pipeline.
	// The goroutine monitors ctx.Done() to avoid leaking when the context is cancelled
	// and the upstream channel is not closed promptly.
	go func() {
		answerDecoder := modelContext.StreamDecoder()
		thinkingDecoder := modelContext.StreamDecoder()
		thinkingID := fmt.Sprintf("%s-thinking", uuid.New().String()[:8])
		answerID := fmt.Sprintf("%s-answer", uuid.New().String()[:8])
		thinkingOpen := false
		answerCompleted := false
		answerProduced := false
		var answerBuf strings.Builder

		closeThinking := func() {
			if !thinkingOpen {
				return
			}
			eventBus.Emit(ctx, types.Event{
				ID:        thinkingID,
				Type:      types.EventType(event.EventAgentThought),
				SessionID: chatManage.SessionID,
				Data: event.AgentThoughtData{
					Done: true,
				},
			})
			thinkingOpen = false
		}

		// flushDecoders drains any handle suffix the stream decoders held back to
		// bridge references split across provider chunks. Both the normal close
		// and the cancellation path must call this, otherwise a resource
		// reference in flight at teardown is silently dropped (and never
		// persisted, since the assistant message is saved from these events).
		flushDecoders := func() {
			thinkingTail := thinkingDecoder.Flush()
			if thinkingTail != "" {
				_ = eventBus.Emit(ctx, types.Event{
					ID:        thinkingID,
					Type:      types.EventType(event.EventAgentThought),
					SessionID: chatManage.SessionID,
					Data:      event.AgentThoughtData{Content: thinkingTail},
				})
			}
			answerBuf.WriteString(answerDecoder.Flush())
			if answerBuf.Len() > 0 {
				_ = eventBus.Emit(ctx, types.Event{
					ID:        answerID,
					Type:      types.EventType(event.EventAgentFinalAnswer),
					SessionID: chatManage.SessionID,
					Data:      event.AgentFinalAnswerData{Content: answerBuf.String()},
				})
				answerBuf.Reset()
			}
		}

		for {
			select {
			case <-ctx.Done():
				flushDecoders()
				closeThinking()
				pipelineInfo(ctx, "Stream", "context_cancelled", map[string]interface{}{
					"session_id": chatManage.SessionID,
				})
				return

			case response, ok := <-responseChan:
				if !ok {
					flushDecoders()
					closeThinking()
					pipelineInfo(ctx, "Stream", "channel_close", map[string]interface{}{
						"session_id": chatManage.SessionID,
					})
					return
				}

				if response.ResponseType == types.ResponseTypeError {
					pipelineError(ctx, "Stream", "stream_error", map[string]interface{}{
						"session_id": chatManage.SessionID,
						"error":      response.Content,
					})
					eventBus.Emit(ctx, types.Event{
						ID:        fmt.Sprintf("%s-error", uuid.New().String()[:8]),
						Type:      types.EventType(event.EventError),
						SessionID: chatManage.SessionID,
						Data: event.ErrorData{
							Error:     response.Content,
							Stage:     "chat_completion_stream",
							SessionID: chatManage.SessionID,
						},
					})
					continue
				}

				if response.ResponseType == types.ResponseTypeThinking {
					response.Content = thinkingDecoder.Feed(response.Content)
					if response.Done {
						response.Content += thinkingDecoder.Flush()
					}
					if response.Content != "" {
						thinkingOpen = true
						eventBus.Emit(ctx, types.Event{
							ID:        thinkingID,
							Type:      types.EventType(event.EventAgentThought),
							SessionID: chatManage.SessionID,
							Data: event.AgentThoughtData{
								Content: response.Content,
								Done:    false,
							},
						})
					}
					if response.Done {
						closeThinking()
					}
					continue
				}

				if response.ResponseType == types.ResponseTypeAnswer {
					// Providers can emit a completion once for finish_reason and again
					// for their EOF sentinel. A final answer is a terminal event for a
					// single stream, so forwarding a later duplicate would put an answer
					// after the session's complete event.
					if answerCompleted {
						continue
					}
					answerBuf.WriteString(answerDecoder.Feed(response.Content))
					if !response.Done {
						continue
					}
					answerBuf.WriteString(answerDecoder.Flush())
					answerCompleted = true
					response.Content = answerBuf.String()
					if strings.TrimSpace(response.Content) != "" {
						answerProduced = true
					}
					truncated := IsLengthFinishReason(response.FinishReason)
					if truncated && !answerProduced {
						response.Content = EmptyTruncatedAnswerFallback
						answerProduced = true
					}
					response.Content = p.validateAndRepair(ctx, modelContext, chatModel, chatMessages, opt, response.Content)
					answerBuf.Reset()
					closeThinking()
					eventBus.Emit(ctx, types.Event{
						ID:        answerID,
						Type:      types.EventType(event.EventAgentFinalAnswer),
						SessionID: chatManage.SessionID,
						Data: event.AgentFinalAnswerData{
							Content:   response.Content,
							Done:      true,
							Truncated: truncated,
						},
					})
				}
			}
		}
	}()

	return next()
}

// validateAndRepair checks the completed answer against the original bodies of
// the chunks it cites and asks the model once to fix citations that do not
// support their sentence. The repaired answer replaces the draft only when the
// number of unsupported citations strictly decreases.
func (p *PluginChatCompletionStream) validateAndRepair(
	ctx context.Context,
	registry *modelcontext.Registry,
	chatModel chat.Chat,
	messages []chat.Message,
	opt *chat.ChatOptions,
	answer string,
) string {
	if registry == nil || strings.TrimSpace(answer) == "" {
		return answer
	}
	violations := registry.ValidateAnswer(answer)
	if len(violations) == 0 {
		return answer
	}
	var details strings.Builder
	for _, violation := range violations {
		body, _ := registry.ChunkText(violation.ChunkID)
		body = strings.Join(strings.Fields(body), " ")
		if runes := []rune(body); len(runes) > 600 {
			body = string(runes[:600])
		}
		fmt.Fprintf(&details, "- line: %s\n  fact missing from every cited chunk: %s\n  cited chunk body: %s\n",
			violation.Line, violation.Fact, body)
	}
	repairMessages := append([]chat.Message{}, messages...)
	repairMessages = append(repairMessages,
		chat.Message{Role: "assistant", Content: answer},
		chat.Message{
			Role: "user",
			Content: "One or more cited chunks do not contain a number or clause of the line they annotate; another retrieved chunk does. " +
				"Rewrite only those lines so they cite the chunk whose body actually contains the value, keeping the value and the sentence. " +
				"Do not delete numbers, sentences or citations, and do not touch lines that are already correct. Output only the corrected answer.\n\n" + details.String(),
		},
	)
	repairCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	response, err := chatModel.Chat(repairCtx, repairMessages, opt)
	if err != nil || response == nil || strings.TrimSpace(response.Content) == "" {
		pipelineWarn(ctx, "Stream", "citation_repair_failed", map[string]interface{}{
			"violations": len(violations),
		})
		return answer
	}
	decoder := registry.StreamDecoder()
	fixed := decoder.Feed(response.Content) + decoder.Flush()
	after := len(registry.ValidateAnswer(fixed))
	if after >= len(violations) || modelcontext.FactLineCount(fixed) < modelcontext.FactLineCount(answer) {
		pipelineInfo(ctx, "Stream", "citation_repair_rejected", map[string]interface{}{
			"before": len(violations),
			"after":  after,
		})
		return answer
	}
	pipelineInfo(ctx, "Stream", "citation_repair_applied", map[string]interface{}{
		"before": len(violations),
		"after":  after,
	})
	return fixed
}
