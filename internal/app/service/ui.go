package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/Flicster/peerchat/internal/app/model"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	appVersion = "v1.0.0"
)

type uiCommand struct {
	Type string
	Arg  string
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

	messagebox := tview.NewTextView()
	messagebox.
		SetDynamicColors(true).
		SetScrollable(true).
		SetChangedFunc(func() {
			app.Draw()
			messagebox.ScrollToEnd()
		}).
		SetBorder(true).
		SetBorderColor(tcell.ColorGreen).
		SetTitle(fmt.Sprintf("ChatRoom-%s", cr.RoomName)).
		SetTitleAlign(tview.AlignLeft).
		SetTitleColor(tcell.ColorWhite).
		SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			switch event.Key() {
			case tcell.KeyUp:
				row, _ := messagebox.GetScrollOffset()
				if row > 0 {
					messagebox.ScrollTo(row-1, 0)
				}
				return nil
			case tcell.KeyDown:
				row, _ := messagebox.GetScrollOffset()
				messagebox.ScrollTo(row+1, 0)
				return nil
			case tcell.KeyHome:
				messagebox.ScrollToBeginning()
				return nil
			case tcell.KeyEnd:
				messagebox.ScrollToEnd()
				return nil
			case tcell.KeyPgUp:
				row, _ := messagebox.GetScrollOffset()
				if row > 10 {
					messagebox.ScrollTo(row-10, 0)
				} else {
					messagebox.ScrollToBeginning()
				}
				return nil
			case tcell.KeyPgDn:
				row, _ := messagebox.GetScrollOffset()
				messagebox.ScrollTo(row+10, 0)
				return nil
			default:
				return event
			}
		})

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

	peerbox := tview.NewTextView().
		SetChangedFunc(func() {
			app.Draw()
		})
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

			cmdchan <- uiCommand{Type: cmdparts[0], Arg: cmdparts[1]}

		} else {
			msgchan <- line
		}

		input.SetText("")
	})

	flex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(titlebox, 3, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexColumn).
			AddItem(messagebox, 0, 1, false). // Убедитесь, что messagebox не в фокусе по умолчанию
			AddItem(peerbox, 20, 1, false),
			0, 8, false).
		AddItem(input, 3, 1, true).
		AddItem(usage, 3, 1, false)

	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyTab {
			if input.HasFocus() {
				app.SetFocus(messagebox)
			} else {
				app.SetFocus(input)
			}
			return nil
		}
		return event
	})

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
	ui.displayHistory()
	go ui.start()

	defer ui.Close()
	return ui.TerminalApp.Run()
}

func (ui *UI) Close() {
	ui.ChatRoom.cancel()
}

func (ui *UI) start() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case msg := <-ui.MsgInputs:
			m := model.ChatMessage{
				Message:    msg,
				SenderID:   ui.ChatRoom.peerId.Pretty(),
				SenderName: ui.ChatRoom.UserName,
				CreatedAt:  time.Now(),
			}
			ui.Outbound <- m
		case cmd := <-ui.CmdInputs:
			ui.handleCommand(cmd)
		case msg := <-ui.ChatRoom.Inbound:
			ui.displayMessage(msg)
		case log := <-ui.ChatRoom.Logs:
			ui.displayLogMessage(log)
		case <-ticker.C:
			ui.syncPeerBox()
		case <-ui.ChatRoom.ctx.Done():
			return
		}
	}
}

func (ui *UI) handleCommand(cmd uiCommand) {
	switch cmd.Type {
	case "/quit":
		ui.TerminalApp.Stop()
		return
	case "/clear":
		ui.messageBox.Clear()
	case "/room":
		if cmd.Arg == "" {
			ui.Logs <- chatlog{logPrefix: "system", logMsg: "missing room name for command"}
			return
		} else if cmd.Arg == ui.RoomName {
			ui.Logs <- chatlog{logPrefix: "system", logMsg: "you are currently in the room"}
			return
		} else {
			ui.Logs <- chatlog{logPrefix: "system", logMsg: fmt.Sprintf("joining new room '%s'", cmd.Arg)}
			go ui.changeRoom(cmd.Arg)
		}
	case "/user":
		if cmd.Arg == "" {
			ui.Logs <- chatlog{logPrefix: "system", logMsg: "missing user name for command"}
		} else {
			ui.UpdateUser(cmd.Arg)
			ui.inputBox.SetLabel(ui.UserName + " > ")
		}
	default:
		ui.Logs <- chatlog{logPrefix: "system", logMsg: fmt.Sprintf("unsupported command - %s", cmd.Type)}
	}
}

func (ui *UI) displayMessage(msg model.ChatMessage) {
	if msg.SenderName == ui.ChatRoom.UserName {
		ui.displayOwnerMessage(msg)
	} else {
		ui.displayUserMessage(msg)
	}
}

// displayChatMessage displays a message recieved from a peer
func (ui *UI) displayUserMessage(msg model.ChatMessage) {
	prompt := fmt.Sprintf("[lightslategrey]%s[-] [green]<%s>:[-]", msg.CreatedAt.Format(time.TimeOnly), msg.SenderName)
	fmt.Fprintf(ui.messageBox, "%s %s\n", prompt, msg.Message)
}

// displaySelfMessage displays a message recieved from self
func (ui *UI) displayOwnerMessage(msg model.ChatMessage) {
	prompt := fmt.Sprintf("[lightslategrey]%s[-] [blue]<%s>:[-]", msg.CreatedAt.Format(time.TimeOnly), ui.UserName)
	fmt.Fprintf(ui.messageBox, "%s %s\n", prompt, msg.Message)
}

// displayLogMessage displays a log message
func (ui *UI) displayLogMessage(log chatlog) {
	prompt := fmt.Sprintf("[lightslategrey]%s[-] [yellow]<%s>:[-]", time.Now().Format(time.TimeOnly), log.logPrefix)
	fmt.Fprintf(ui.messageBox, "%s %s\n", prompt, log.logMsg)
}

// syncPeerBox refreshes the list of peers
func (ui *UI) syncPeerBox() {
	peers := ui.PeerList()

	ui.peerBox.Lock()
	ui.peerBox.Clear()
	ui.peerBox.Unlock()

	for _, p := range peers {
		peerId := p.Pretty()
		if len(peerId) > 8 {
			peerId = peerId[len(peerId)-8:]
		}
		fmt.Fprintln(ui.peerBox, peerId)
	}
}

func (ui *UI) changeRoom(roomName string) {
	newChatRoom, err := NewChatRoom(ui.Host, ui.UserName, roomName)
	if err != nil {
		ui.Logs <- chatlog{logPrefix: "system", logMsg: fmt.Sprintf("could not change chat room - %s", err)}
		return
	}
	ui.ChatRoom.Exit()

	ui.bindChatRoom(newChatRoom)

	ui.messageBox.Clear()
	ui.messageBox.SetTitle(fmt.Sprintf("ChatRoom-%s", ui.ChatRoom.RoomName))
	ui.displayHistory()
}

func (ui *UI) bindChatRoom(cr *ChatRoom) {
	ui.ChatRoom = cr
	go ui.start()
}

func (ui *UI) displayHistory() {
	for _, msg := range ui.ChatRoom.History {
		if msg.SenderName == ui.ChatRoom.UserName {
			ui.displayOwnerMessage(msg)
		} else {
			ui.displayUserMessage(msg)
		}
	}
}
