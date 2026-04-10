package platforms

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/hermes-agent/hermes-agent-go/internal/gateway"
	"github.com/hermes-agent/hermes-agent-go/internal/tools"
	"github.com/slack-go/slack"
)

// --- Block Kit message support ---

// SendBlocks sends a Block Kit message to a Slack channel.
func (s *SlackAdapter) SendBlocks(chatID string, blocks []slack.Block, fallbackText string, threadTS string) (*gateway.SendResult, error) {
	if s.api == nil {
		return nil, fmt.Errorf("not connected")
	}

	opts := []slack.MsgOption{
		slack.MsgOptionBlocks(blocks...),
		slack.MsgOptionText(fallbackText, false),
	}
	if threadTS != "" {
		opts = append(opts, slack.MsgOptionTS(threadTS))
	}

	_, ts, err := s.api.PostMessage(chatID, opts...)
	if err != nil {
		return &gateway.SendResult{Success: false, Error: err.Error(), Retryable: true}, nil
	}
	return &gateway.SendResult{Success: true, MessageID: ts}, nil
}

// EditMessage updates an existing Slack message.
func (s *SlackAdapter) EditMessage(chatID, messageTS, newText string) error {
	if s.api == nil {
		return fmt.Errorf("not connected")
	}
	_, _, _, err := s.api.UpdateMessage(chatID, messageTS,
		slack.MsgOptionText(newText, false))
	if err != nil {
		return fmt.Errorf("update message: %w", err)
	}
	return nil
}

// EditMessageBlocks updates a message with Block Kit blocks.
func (s *SlackAdapter) EditMessageBlocks(chatID, messageTS string, blocks []slack.Block, fallbackText string) error {
	if s.api == nil {
		return fmt.Errorf("not connected")
	}
	_, _, _, err := s.api.UpdateMessage(chatID, messageTS,
		slack.MsgOptionBlocks(blocks...),
		slack.MsgOptionText(fallbackText, false))
	if err != nil {
		return fmt.Errorf("update message blocks: %w", err)
	}
	return nil
}

// AddReaction adds an emoji reaction to a message.
func (s *SlackAdapter) AddReaction(chatID, messageTS, emoji string) error {
	if s.api == nil {
		return fmt.Errorf("not connected")
	}
	return s.api.AddReaction(emoji, slack.ItemRef{
		Channel:   chatID,
		Timestamp: messageTS,
	})
}

// RemoveReaction removes an emoji reaction from a message.
func (s *SlackAdapter) RemoveReaction(chatID, messageTS, emoji string) error {
	if s.api == nil {
		return fmt.Errorf("not connected")
	}
	return s.api.RemoveReaction(emoji, slack.ItemRef{
		Channel:   chatID,
		Timestamp: messageTS,
	})
}

// --- Approval buttons (Block Kit interactive) ---

// SendApprovalPrompt sends a Block Kit message with approve/deny buttons.
// Integrates with the tools.GatewayApprovalQueue system.
func (s *SlackAdapter) SendApprovalPrompt(chatID, command, sessionKey, description, threadTS string) (*gateway.SendResult, error) {
	cmdPreview := command
	if len(cmdPreview) > 2900 {
		cmdPreview = cmdPreview[:2900] + "..."
	}

	blocks := []slack.Block{
		slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn",
				fmt.Sprintf(":warning: *Command Approval Required*\n```%s```\nReason: %s",
					cmdPreview, description),
				false, false),
			nil, nil),
		slack.NewActionBlock("hermes_approval_actions",
			slack.NewButtonBlockElement("hermes_approve_once", sessionKey,
				slack.NewTextBlockObject("plain_text", "Allow Once", false, false)).
				WithStyle(slack.StylePrimary),
			slack.NewButtonBlockElement("hermes_approve_session", sessionKey,
				slack.NewTextBlockObject("plain_text", "Allow Session", false, false)),
			slack.NewButtonBlockElement("hermes_approve_always", sessionKey,
				slack.NewTextBlockObject("plain_text", "Always Allow", false, false)),
			slack.NewButtonBlockElement("hermes_deny", sessionKey,
				slack.NewTextBlockObject("plain_text", "Deny", false, false)).
				WithStyle(slack.StyleDanger),
		),
	}

	return s.SendBlocks(chatID, blocks,
		fmt.Sprintf("⚠️ Command approval: %s", cmdPreview[:min(100, len(cmdPreview))]),
		threadTS)
}

// HandleApprovalAction processes an interactive button click for approvals.
// Returns true if the action was handled.
func HandleApprovalAction(actionID, sessionKey string) bool {
	var result tools.ApprovalResult

	switch actionID {
	case "hermes_approve_once":
		result = tools.ApprovalResult{Approved: true, Scope: tools.ApproveOnce}
	case "hermes_approve_session":
		result = tools.ApprovalResult{Approved: true, Scope: tools.ApproveSession}
	case "hermes_approve_always":
		result = tools.ApprovalResult{Approved: true, Scope: tools.ApprovePermanent}
	case "hermes_deny":
		result = tools.ApprovalResult{Approved: false, Scope: tools.ApproveDeny}
	default:
		return false
	}

	tools.GlobalGatewayApprovalQueue().Resolve(sessionKey, result)
	return true
}

// --- Markdown → Slack mrkdwn formatting ---

var (
	// Fenced code blocks: ```lang\n...\n```
	reFencedCode = regexp.MustCompile("(?s)```[a-zA-Z]*\n(.*?)```")
	// Inline code: `...`
	reInlineCode = regexp.MustCompile("`([^`]+)`")
	// Bold: **text** → *text*
	reBold = regexp.MustCompile(`\*\*(.+?)\*\*`)
	// Italic: _text_ (already Slack format)
	// Strikethrough: ~~text~~ → ~text~
	reStrike = regexp.MustCompile(`~~(.+?)~~`)
	// Links: [text](url) → <url|text>
	reLink = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	// Headers: # text → *text*
	reHeader = regexp.MustCompile(`(?m)^#{1,6}\s+(.+)$`)
)

// FormatSlackMessage converts Markdown to Slack mrkdwn format.
func FormatSlackMessage(md string) string {
	// Protect code blocks from other transformations.
	var codeBlocks []string
	md = reFencedCode.ReplaceAllStringFunc(md, func(match string) string {
		idx := len(codeBlocks)
		codeBlocks = append(codeBlocks, match) // keep as-is (Slack supports ```)
		return fmt.Sprintf("\x00CODE%d\x00", idx)
	})

	var inlineCode []string
	md = reInlineCode.ReplaceAllStringFunc(md, func(match string) string {
		idx := len(inlineCode)
		inlineCode = append(inlineCode, match)
		return fmt.Sprintf("\x00INLINE%d\x00", idx)
	})

	// Transform Markdown → mrkdwn.
	md = reBold.ReplaceAllString(md, "*$1*")
	md = reStrike.ReplaceAllString(md, "~$1~")
	md = reLink.ReplaceAllString(md, "<$2|$1>")
	md = reHeader.ReplaceAllString(md, "*$1*")

	// Restore protected regions.
	for i, code := range inlineCode {
		md = strings.Replace(md, fmt.Sprintf("\x00INLINE%d\x00", i), code, 1)
	}
	for i, code := range codeBlocks {
		md = strings.Replace(md, fmt.Sprintf("\x00CODE%d\x00", i), code, 1)
	}

	return md
}

// --- Interactive message handling ---

// HandleInteractiveCallback processes Slack interactive message callbacks.
// Called from handleEvent when an interactive_message or block_actions event arrives.
func (s *SlackAdapter) HandleInteractiveCallback(payload json.RawMessage) {
	var callback struct {
		Type    string `json:"type"`
		Actions []struct {
			ActionID string `json:"action_id"`
			Value    string `json:"value"`
		} `json:"actions"`
		Channel struct {
			ID string `json:"id"`
		} `json:"channel"`
		Message struct {
			TS string `json:"ts"`
		} `json:"message"`
	}

	if err := json.Unmarshal(payload, &callback); err != nil {
		return
	}

	for _, action := range callback.Actions {
		if HandleApprovalAction(action.ActionID, action.Value) {
			// Update the message to show the result.
			statusText := "✅ Approved"
			if action.ActionID == "hermes_deny" {
				statusText = "❌ Denied"
			}
			s.EditMessage(callback.Channel.ID, callback.Message.TS,
				fmt.Sprintf("%s (by user)", statusText))
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
