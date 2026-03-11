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
	stateCh <-chan JoyconState
	side    JoyconSide
}

type joyconPacketMsg struct {
	source string
	state  JoyconState
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
	lastUpdated time.Time
	closed      bool
	side        JoyconSide
	state       JoyconState
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
			side:  source.side,
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
		state.playerNo = msg.state.PlayerNo
		if msg.state.Side != UnknownSide {
			state.side = msg.state.Side
		}
		state.state = msg.state
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
	for _, key := range m.order {
		state := m.sources[key]
		role := classifyRenderRole(state.side, len(m.order))
		cards = append(cards, m.renderControllerCard(state, role))
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
		mutedStyle.Render(fmt.Sprintf("Player: %d", state.playerNo)),
		mutedStyle.Render(controllerStatus(state)),
		"",
		titleStyle.Render("Buttons"),
		renderButtonPanel(state, role, accent),
		"",
		titleStyle.Render("State"),
		renderStateSummary(state.state),
		"",
		titleStyle.Render("Latest packet"),
		renderPacketPreview(state.state.Raw),
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
		case input, ok := <-source.stateCh:
			if !ok {
				program.Send(joyconChannelClosedMsg{source: source.name})
				return
			}

			program.Send(joyconPacketMsg{
				source: source.name,
				state:  input,
				at:     time.Now(),
			})
		}
	}
}

func classifyRenderRole(side JoyconSide, total int) joyconRenderRole {
	switch side {
	case LeftSide:
		return joyconRoleLeft
	case RightSide:
		return joyconRoleRight
	case UnknownSide:
		if total == 1 {
			return joyconRoleSingle
		}
	}
	return joyconRoleSingle
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
			buttonSpec{label: "L", button: ButtonL},
			buttonSpec{label: "ZL", button: ButtonZL},
			buttonSpec{label: "-", button: ButtonMinus},
			buttonSpec{label: "CAP", button: ButtonCapture},
			buttonSpec{label: "UP", button: ButtonUp},
			buttonSpec{label: "LT", button: ButtonLeft},
			buttonSpec{label: "DN", button: ButtonDown},
			buttonSpec{label: "RT", button: ButtonRight},
			buttonSpec{label: "SL", button: ButtonSL},
			buttonSpec{label: "STK", button: ButtonStick},
			buttonSpec{label: "SR", button: ButtonSR},
		)
	case joyconRoleRight:
		return renderButtonStrip(accent, state,
			buttonSpec{label: "R", button: ButtonR},
			buttonSpec{label: "ZR", button: ButtonZR},
			buttonSpec{label: "+", button: ButtonPlus},
			buttonSpec{label: "X", button: ButtonX},
			buttonSpec{label: "Y", button: ButtonY},
			buttonSpec{label: "A", button: ButtonA},
			buttonSpec{label: "B", button: ButtonB},
			buttonSpec{label: "SL", button: ButtonSL},
			buttonSpec{label: "STK", button: ButtonStick},
			buttonSpec{label: "SR", button: ButtonSR},
			buttonSpec{label: "HOME", button: ButtonHome},
			buttonSpec{label: "CHAT", button: ButtonGameChat},
		)
	default:
		return renderButtonStrip(accent, state,
			buttonSpec{label: "L", button: ButtonL},
			buttonSpec{label: "ZL", button: ButtonZL},
			buttonSpec{label: "R", button: ButtonR},
			buttonSpec{label: "ZR", button: ButtonZR},
			buttonSpec{label: "UP", button: ButtonUp},
			buttonSpec{label: "LT", button: ButtonLeft},
			buttonSpec{label: "DN", button: ButtonDown},
			buttonSpec{label: "RT", button: ButtonRight},
			buttonSpec{label: "X", button: ButtonX},
			buttonSpec{label: "Y", button: ButtonY},
			buttonSpec{label: "A", button: ButtonA},
			buttonSpec{label: "B", button: ButtonB},
			buttonSpec{label: "-", button: ButtonMinus},
			buttonSpec{label: "STK", button: ButtonStick},
			buttonSpec{label: "+", button: ButtonPlus},
			buttonSpec{label: "HOME", button: ButtonHome},
			buttonSpec{label: "CHAT", button: ButtonGameChat},
		)
	}
}

type buttonSpec struct {
	label  string
	button Button
}

func renderButtonStrip(accent lipgloss.Color, state joyconCardState, buttons ...buttonSpec) string {
	row := make([]string, 0, len(buttons))
	for _, button := range buttons {
		row = append(row, buttonChip(button.label, state.state.Buttons[button.button], accent))
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

func renderStateSummary(state JoyconState) string {
	lines := []string{
		renderMetricRow("Temp",
			fmt.Sprintf("%5.2f C", state.Temperature),
		),
		renderMetricRow("Batt",
			fmt.Sprintf("%5.2f V", state.Voltage),
			fmt.Sprintf("%5.2f A", state.Ampere),
		),

		renderMetricRow("Stick",
			fmt.Sprintf("X:%6d", state.Stick.X),
			fmt.Sprintf("Y:%6d", state.Stick.Y),
		),

		renderMetricRow("Gyro",
			fmt.Sprintf("X:%6.2f", state.Gyro[0]),
			fmt.Sprintf("Y:%6.2f", state.Gyro[1]),
			fmt.Sprintf("Z:%6.2f deg/s", state.Gyro[2]),
		),
		renderMetricRow("Accel",
			fmt.Sprintf("X:%6.2f", state.Accel[0]),
			fmt.Sprintf("Y:%6.2f", state.Accel[1]),
			fmt.Sprintf("Z:%6.2f G", state.Accel[2]),
		),
		renderMetricRow("Mouse",
			fmt.Sprintf("X:%6d", state.Mouse.MouseX),
			fmt.Sprintf("Y:%6d", state.Mouse.MouseY),
			fmt.Sprintf("D:%6d", state.Mouse.Distance),
		),
	}
	return strings.Join(lines, "\n")
}

func renderMetricRow(label string, values ...string) string {
	labelStyle := lipgloss.NewStyle().
		Width(6).
		Bold(true).
		Foreground(lipgloss.Color("243"))

	return labelStyle.Render(label) + " " + strings.Join(values, "  ")
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

func buttonLabel(button Button) string {
	switch button {
	case ButtonUp:
		return "UP"
	case ButtonDown:
		return "DOWN"
	case ButtonLeft:
		return "LEFT"
	case ButtonRight:
		return "RIGHT"
	case ButtonA:
		return "A"
	case ButtonB:
		return "B"
	case ButtonX:
		return "X"
	case ButtonY:
		return "Y"
	case ButtonL:
		return "L"
	case ButtonR:
		return "R"
	case ButtonZL:
		return "ZL"
	case ButtonZR:
		return "ZR"
	case ButtonSL:
		return "SL"
	case ButtonSR:
		return "SR"
	case ButtonPlus:
		return "+"
	case ButtonMinus:
		return "-"
	case ButtonHome:
		return "HOME"
	case ButtonCapture:
		return "CAPTURE"
	case ButtonStick:
		return "STICK"
	case ButtonGameChat:
		return "CHAT"
	default:
		return "UNKNOWN"
	}
}
