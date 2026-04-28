package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// responseMsg carries a completed agent response.
type responseMsg struct{ text string }

// streamDeltaMsg carries a streaming text delta.
type streamDeltaMsg struct{ text string }

// toolProgressMsg indicates tool execution progress.
type toolProgressMsg struct{ tool string }

// Model is the main Bubble Tea model for the Hermes TUI.
type Model struct {
	textarea    textarea.Model
	viewport    viewport.Model
	spinner     spinner.Model
	history     []chatEntry
	streaming   strings.Builder
	isStreaming bool
	isThinking  bool
	width       int
	height      int
	ready       bool
	sendFn      func(string) // callback to send user input to agent
}

type chatEntry struct {
	role    string // "user", "assistant", "tool"
	content string
}

var (
	userStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	assistantStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	toolStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Italic(true)
	statusStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
)

// New creates a new TUI model.
func New(sendFn func(string)) Model {
	ta := textarea.New()
	ta.Placeholder = "Type a message... (Ctrl+D to send, Ctrl+C to quit)"
	ta.Focus()
	ta.CharLimit = 0
	ta.SetHeight(3)

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	return Model{
		textarea: ta,
		spinner:  sp,
		sendFn:   sendFn,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, m.spinner.Tick)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyCtrlD:
			text := strings.TrimSpace(m.textarea.Value())
			if text != "" {
				m.history = append(m.history, chatEntry{role: "user", content: text})
				m.textarea.Reset()
				m.isThinking = true
				if m.sendFn != nil {
					go m.sendFn(text)
				}
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height-6)
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - 6
		}
		m.textarea.SetWidth(msg.Width)
		m.updateViewport()

	case responseMsg:
		m.isStreaming = false
		m.isThinking = false
		content := msg.text
		if m.streaming.Len() > 0 {
			content = m.streaming.String()
			m.streaming.Reset()
		}
		m.history = append(m.history, chatEntry{role: "assistant", content: content})
		m.updateViewport()

	case streamDeltaMsg:
		m.isStreaming = true
		m.isThinking = false
		m.streaming.WriteString(msg.text)
		m.updateViewport()

	case toolProgressMsg:
		m.isThinking = true

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *Model) updateViewport() {
	var sb strings.Builder
	for _, entry := range m.history {
		switch entry.role {
		case "user":
			sb.WriteString(userStyle.Render("You: "))
			sb.WriteString(entry.content)
		case "assistant":
			sb.WriteString(assistantStyle.Render("Hermes: "))
			sb.WriteString(entry.content)
		case "tool":
			sb.WriteString(toolStyle.Render(entry.content))
		}
		sb.WriteString("\n\n")
	}
	if m.streaming.Len() > 0 {
		sb.WriteString(assistantStyle.Render("Hermes: "))
		sb.WriteString(m.streaming.String())
		sb.WriteString("▌\n")
	}
	m.viewport.SetContent(sb.String())
	m.viewport.GotoBottom()
}

func (m Model) View() string {
	if !m.ready {
		return "Initializing..."
	}

	var status string
	if m.isThinking {
		status = statusStyle.Render(fmt.Sprintf(" %s Thinking...", m.spinner.View()))
	} else if m.isStreaming {
		status = statusStyle.Render(fmt.Sprintf(" %s Streaming...", m.spinner.View()))
	}

	return fmt.Sprintf("%s\n%s\n%s",
		m.viewport.View(),
		status,
		m.textarea.View(),
	)
}

// PushResponse sends a completed response to the TUI (thread-safe via tea.Cmd).
func PushResponse(p *tea.Program, text string) {
	p.Send(responseMsg{text: text})
}

// PushStreamDelta sends a streaming delta to the TUI.
func PushStreamDelta(p *tea.Program, text string) {
	p.Send(streamDeltaMsg{text: text})
}

// PushToolProgress notifies the TUI of tool execution.
func PushToolProgress(p *tea.Program, tool string) {
	p.Send(toolProgressMsg{tool: tool})
}
