package internal

import (
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	appVersion = "v1.0.0"
)

type uiCommand struct {
	cmdType string
	cmdArg  string
}

type UI struct {
	*ChatRoom
	TerminalApp *tview.Application
	MsgInputs   chan string
	CmdInputs   chan uiCommand

	peerBox    *tview.TextView
	messageBox *tview.TextView
	inputBox   *tview.InputField
}

func NewUI(cr *ChatRoom) *UI {
	app := tview.NewApplication()

	cmdchan := make(chan uiCommand)
	msgchan := make(chan string)

	titlebox := tview.NewTextView().
		SetText(fmt.Sprintf("PeerChat. A P2P Chat Application. %s", appVersion)).
		SetTextColor(tcell.ColorWhite).
		SetTextAlign(tview.AlignCenter)

	titlebox.
		SetBorder(true).
		SetBorderColor(tcell.ColorGreen)

	messagebox := tview.NewTextView().
		SetDynamicColors(true).
		SetChangedFunc(func() {
			app.Draw()
		})

	messagebox.
		SetBorder(true).
		SetBorderColor(tcell.ColorGreen).
		SetTitle(fmt.Sprintf("ChatRoom-%s", cr.RoomName)).
		SetTitleAlign(tview.AlignLeft).
		SetTitleColor(tcell.ColorWhite)

	usage := tview.NewTextView().
		SetDynamicColors(true).
		SetText(`[red]/quit[green] - quit the chat | [red]/room <roomname>[green] - change chat room | [red]/user <username>[green] - change user name | [red]/clear[green] - clear the chat`)

	usage.
		SetBorder(true).
		SetBorderColor(tcell.ColorGreen).
		SetTitle("Usage").
		SetTitleAlign(tview.AlignLeft).
		SetTitleColor(tcell.ColorWhite).
		SetBorderPadding(0, 0, 1, 0)

	peerbox := tview.NewTextView()

	peerbox.
		SetBorder(true).
		SetBorderColor(tcell.ColorGreen).
		SetTitle("Peers").
		SetTitleAlign(tview.AlignLeft).
		SetTitleColor(tcell.ColorWhite)

	input := tview.NewInputField().
		SetLabel(cr.UserName + " > ").
		SetLabelColor(tcell.ColorGreen).
		SetFieldWidth(0).
		SetFieldBackgroundColor(tcell.ColorBlack)

	input.SetBorder(true).
		SetBorderColor(tcell.ColorGreen).
		SetTitle("Input").
		SetTitleAlign(tview.AlignLeft).
		SetTitleColor(tcell.ColorWhite).
		SetBorderPadding(0, 0, 1, 0)

	input.SetDoneFunc(func(key tcell.Key) {
		if key != tcell.KeyEnter {
			return
		}

		line := input.GetText()
		if len(line) == 0 {
			return
		}

		if strings.HasPrefix(line, "/") {
			cmdparts := strings.Split(line, " ")
			if len(cmdparts) == 1 {
				cmdparts = append(cmdparts, "")
			}

			cmdchan <- uiCommand{cmdType: cmdparts[0], cmdArg: cmdparts[1]}

		} else {
			msgchan <- line
		}

		input.SetText("")
	})

	flex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(titlebox, 3, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexColumn).
			AddItem(messagebox, 0, 1, false).
			AddItem(peerbox, 20, 1, false),
			0, 8, false).
		AddItem(input, 3, 1, true).
		AddItem(usage, 3, 1, false)

	app.SetRoot(flex, true)

	return &UI{
		ChatRoom:    cr,
		TerminalApp: app,
		peerBox:     peerbox,
		messageBox:  messagebox,
		inputBox:    input,
		MsgInputs:   msgchan,
		CmdInputs:   cmdchan,
	}
}

func (ui *UI) Run() error {
	go ui.startEventHandler()

	defer ui.Close()
	return ui.TerminalApp.Run()
}

func (ui *UI) Close() {
	ui.cancel()
}

func (ui *UI) startEventHandler() {
	refreshticker := time.NewTicker(time.Second)
	defer refreshticker.Stop()

	for {
		select {

		case msg := <-ui.MsgInputs:
			ui.Outbound <- msg
			ui.displaySelfMessage(msg)
		case cmd := <-ui.CmdInputs:
			go ui.handleCommand(cmd)
		case msg := <-ui.Inbound:
			ui.displayChatMessage(msg)
		case log := <-ui.Logs:
			ui.displayLogMessage(log)
		case <-refreshticker.C:
			ui.syncPeerBox()
		case <-ui.ctx.Done():
			return
		}
	}
}

func (ui *UI) handleCommand(cmd uiCommand) {
	switch cmd.cmdType {
	case "/quit":
		ui.TerminalApp.Stop()
		return

	case "/clear":
		ui.messageBox.Clear()
	case "/room":
		if cmd.cmdArg == "" {
			ui.Logs <- chatlog{logPrefix: "badcmd", logMsg: "missing room name for command"}
		} else {
			ui.Logs <- chatlog{logPrefix: "roomchange", logMsg: fmt.Sprintf("joining new room '%s'", cmd.cmdArg)}
			oldchatroom := ui.ChatRoom
			newchatroom, err := NewChatRoom(ui.Host, ui.UserName, cmd.cmdArg)
			if err != nil {
				ui.Logs <- chatlog{logPrefix: "jumperr", logMsg: fmt.Sprintf("could not change chat room - %s", err)}
				return
			}

			ui.ChatRoom = newchatroom
			time.Sleep(time.Second * 1)

			oldchatroom.Exit()

			ui.messageBox.Clear()
			ui.messageBox.SetTitle(fmt.Sprintf("ChatRoom-%s", ui.ChatRoom.RoomName))
		}
	case "/user":
		if cmd.cmdArg == "" {
			ui.Logs <- chatlog{logPrefix: "badcmd", logMsg: "missing user name for command"}
		} else {
			ui.UpdateUser(cmd.cmdArg)
			ui.inputBox.SetLabel(ui.UserName + " > ")
		}
	default:
		ui.Logs <- chatlog{logPrefix: "badcmd", logMsg: fmt.Sprintf("unsupported command - %s", cmd.cmdType)}
	}
}

// displayChatMessage displays a message recieved from a peer
func (ui *UI) displayChatMessage(msg chatMessage) {
	prompt := fmt.Sprintf("[green]<%s>:[-]", msg.SenderName)
	fmt.Fprintf(ui.messageBox, "%s %s\n", prompt, msg.Message)
}

// displaySelfMessage displays a message recieved from self
func (ui *UI) displaySelfMessage(msg string) {
	prompt := fmt.Sprintf("[blue]<%s>:[-]", ui.UserName)
	fmt.Fprintf(ui.messageBox, "%s %s\n", prompt, msg)
}

// displayLogMessage displays a log message
func (ui *UI) displayLogMessage(log chatlog) {
	prompt := fmt.Sprintf("[yellow]<%s>:[-]", log.logPrefix)
	fmt.Fprintf(ui.messageBox, "%s %s\n", prompt, log.logMsg)
}

// syncPeerBox refreshes the list of peers
func (ui *UI) syncPeerBox() {
	peers := ui.PeerList()

	ui.peerBox.Lock()
	ui.peerBox.Clear()
	ui.peerBox.Unlock()

	for _, p := range peers {
		peerid := p.Pretty()
		peerid = peerid[len(peerid)-8:]
		fmt.Fprintln(ui.peerBox, peerid)
	}

	ui.TerminalApp.Draw()
}
