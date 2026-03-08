package joy2mac

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type joyconInputSource struct {
	name    string
	title   string
	inputCh <-chan InputData
}

type joyconPacketMsg struct {
	source string
	data   InputData
	at     time.Time
}

type joyconChannelClosedMsg struct {
	source string
}

type joyconTickMsg time.Time

type joyconRenderRole string

const (
	joyconRoleSingle joyconRenderRole = "single"
	joyconRoleLeft   joyconRenderRole = "left"
	joyconRoleRight  joyconRenderRole = "right"
)

type joyconCardState struct {
	title       string
	playerNo    int
	packetCount int
	payloadSize int
	lastPacket  []byte
	lastUpdated time.Time
	closed      bool
}

type joyconViewerModel struct {
	sources map[string]joyconCardState
	order   []string
	width   int
	height  int
}

func newJoyconViewerModel(inputSources []joyconInputSource) joyconViewerModel {
	sources := make(map[string]joyconCardState, len(inputSources))
	order := make([]string, 0, len(inputSources))

	for _, source := range inputSources {
		order = append(order, source.name)
		sources[source.name] = joyconCardState{
			title: source.title,
		}
	}

	return joyconViewerModel{
		sources: sources,
		order:   order,
	}
}

func (m joyconViewerModel) Init() tea.Cmd {
	return joyconTickCmd()
}

func (m joyconViewerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case joyconPacketMsg:
		state := m.sources[msg.source]
		state.playerNo = msg.data.playerNo
		state.packetCount++
		state.payloadSize = len(msg.data.data)
		state.lastPacket = msg.data.data
		state.lastUpdated = msg.at
		state.closed = false
		m.sources[msg.source] = state
	case joyconChannelClosedMsg:
		state := m.sources[msg.source]
		state.closed = true
		m.sources[msg.source] = state
	case joyconTickMsg:
		return m, joyconTickCmd()
	}

	return m, nil
}

func (m joyconViewerModel) View() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205"))

	subtitleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241"))

	appStyle := lipgloss.NewStyle().
		Padding(1, 2)

	header := titleStyle.Render("Joy2Mac viewer")
	help := subtitleStyle.Render("Press q to quit")

	cards := make([]string, 0, len(m.order))
	for i, key := range m.order {
		role := classifyRenderRole(i, len(m.order))
		cards = append(cards, m.renderControllerCard(m.sources[key], role))
	}

	body := strings.Join(cards, "\n\n")
	if len(cards) > 1 {
		body = lipgloss.JoinHorizontal(lipgloss.Top, cards...)
	}

	return appStyle.Render(header + "\n" + help + "\n\n" + body)
}

func (m joyconViewerModel) renderControllerCard(state joyconCardState, role joyconRenderRole) string {
	accent := accentColor(role)

	cardStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accent).
		Padding(0, 1).
		Width(max(42, (m.width-10)/max(1, len(m.order))))

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(accent)

	mutedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("243"))

	lines := []string{
		titleStyle.Render(state.title),
		fmt.Sprintf("Player: %d", state.playerNo),
		fmt.Sprintf("Packets: %d", state.packetCount),
		fmt.Sprintf("Payload: %d bytes", state.payloadSize),
		mutedStyle.Render(controllerStatus(state)),
		"",
		titleStyle.Render("Latest packet"),
		renderPacketPreview(state.lastPacket),
		"",
		titleStyle.Render("Buttons"),
		renderButtonPanel(state, role, accent),
	}

	return cardStyle.Render(strings.Join(lines, "\n"))
}

func joyconTickCmd() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(t time.Time) tea.Msg {
		return joyconTickMsg(t)
	})
}

func runJoyconTUI(ctx context.Context, inputSources []joyconInputSource) {
	model := newJoyconViewerModel(inputSources)
	program := tea.NewProgram(model, tea.WithAltScreen())

	for _, source := range inputSources {
		go forwardJoyconInput(ctx, program, source)
	}

	go func() {
		<-ctx.Done()
		program.Send(tea.Quit())
	}()

	if _, err := program.Run(); err != nil {
		fmt.Printf("failed to run Joy-Con TUI: %v\n", err)
	}
}

func forwardJoyconInput(ctx context.Context, program *tea.Program, source joyconInputSource) {
	for {
		select {
		case <-ctx.Done():
			return
		case input, ok := <-source.inputCh:
			if !ok {
				program.Send(joyconChannelClosedMsg{source: source.name})
				return
			}

			cloned := append([]byte(nil), input.data...)
			input.data = cloned

			program.Send(joyconPacketMsg{
				source: source.name,
				data:   input,
				at:     time.Now(),
			})
		}
	}
}

func renderPacketPreview(packet []byte) string {
	if len(packet) == 0 {
		return "(no data yet)"
	}

	const bytesPerLine = 12

	lines := make([]string, 0, (len(packet)+bytesPerLine-1)/bytesPerLine)
	for start := 0; start < len(packet); start += bytesPerLine {
		end := min(len(packet), start+bytesPerLine)
		encoded := hex.EncodeToString(packet[start:end])
		chunks := make([]string, 0, (len(encoded)+1)/2)
		for i := 0; i < len(encoded); i += 2 {
			chunks = append(chunks, encoded[i:i+2])
		}
		lines = append(lines, strings.Join(chunks, " "))
	}

	return strings.Join(lines, "\n")
}

func classifyRenderRole(index int, total int) joyconRenderRole {
	if total == 1 {
		return joyconRoleSingle
	}
	if index == 0 {
		return joyconRoleLeft
	}
	return joyconRoleRight
}

func controllerStatus(state joyconCardState) string {
	switch {
	case state.closed:
		return "channel closed"
	case state.lastUpdated.IsZero():
		return "waiting for input"
	default:
		return fmt.Sprintf("status: %s", activityLabel(state)) + " | last packet " + fmt.Sprintf("%d", time.Since(state.lastUpdated).Microseconds()) + " μs ago"
	}
}

func activityLabel(state joyconCardState) string {
	switch {
	case state.closed:
		return "closed"
	case state.lastUpdated.IsZero():
		return "idle"
	case time.Since(state.lastUpdated) <= time.Second:
		return "active"
	default:
		return "idle"
	}
}

func accentColor(role joyconRenderRole) lipgloss.Color {
	switch role {
	case joyconRoleLeft:
		return lipgloss.Color("39")
	case joyconRoleRight:
		return lipgloss.Color("204")
	default:
		return lipgloss.Color("63")
	}
}

func renderButtonPanel(state joyconCardState, role joyconRenderRole, accent lipgloss.Color) string {
	switch role {
	case joyconRoleLeft:
		return renderButtonStrip(accent, state,
			buttonSpec{label: "L", bit: 0},
			buttonSpec{label: "ZL", bit: 1},
			buttonSpec{label: "-", bit: 6},
			buttonSpec{label: "CAP", bit: 7},
			buttonSpec{label: "UP", bit: 2},
			buttonSpec{label: "LT", bit: 3},
			buttonSpec{label: "DN", bit: 4},
			buttonSpec{label: "RT", bit: 5},
			buttonSpec{label: "SL", bit: 8},
			buttonSpec{label: "STK", bit: 10},
			buttonSpec{label: "SR", bit: 9},
		)
	case joyconRoleRight:
		return renderButtonStrip(accent, state,
			buttonSpec{label: "R", bit: 0},
			buttonSpec{label: "ZR", bit: 1},
			buttonSpec{label: "+", bit: 6},
			buttonSpec{label: "HOME", bit: 7},
			buttonSpec{label: "X", bit: 2},
			buttonSpec{label: "Y", bit: 3},
			buttonSpec{label: "A", bit: 4},
			buttonSpec{label: "B", bit: 5},
			buttonSpec{label: "SL", bit: 8},
			buttonSpec{label: "STK", bit: 10},
			buttonSpec{label: "SR", bit: 9},
		)
	default:
		return renderButtonStrip(accent, state,
			buttonSpec{label: "L", bit: 0},
			buttonSpec{label: "ZL", bit: 1},
			buttonSpec{label: "R", bit: 2},
			buttonSpec{label: "ZR", bit: 3},
			buttonSpec{label: "UP", bit: 4},
			buttonSpec{label: "LT", bit: 5},
			buttonSpec{label: "DN", bit: 6},
			buttonSpec{label: "RT", bit: 7},
			buttonSpec{label: "X", bit: 8},
			buttonSpec{label: "Y", bit: 9},
			buttonSpec{label: "A", bit: 10},
			buttonSpec{label: "B", bit: 11},
			buttonSpec{label: "-", bit: 12},
			buttonSpec{label: "STK", bit: 13},
			buttonSpec{label: "+", bit: 14},
			buttonSpec{label: "HOME", bit: 15},
		)
	}
}

type buttonSpec struct {
	label string
	bit   int
}

func renderButtonStrip(accent lipgloss.Color, state joyconCardState, buttons ...buttonSpec) string {
	row := make([]string, 0, len(buttons))
	for _, button := range buttons {
		row = append(row, buttonChip(button.label, packetBit(state.lastPacket, button.bit), accent))
	}
	return lipgloss.JoinHorizontal(lipgloss.Center, row...)
}

func buttonChip(label string, active bool, accent lipgloss.Color) string {
	style := lipgloss.NewStyle().
		Width(max(4, len(label)+2)).
		Align(lipgloss.Center).
		Bold(true).
		Padding(0, 1)

	if active {
		style = style.
			Background(accent).
			Foreground(lipgloss.Color("0"))
	} else {
		style = style.
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("248"))
	}

	return style.Render(label)
}

func packetBit(packet []byte, bit int) bool {
	byteIndex := bit / 8
	bitIndex := bit % 8

	if byteIndex < 0 || byteIndex >= len(packet) {
		return false
	}

	return packet[byteIndex]&(1<<bitIndex) != 0
}
