package messaging

import (
	"fmt"
	"strconv"
	"strings"
)

const TERMINATOR string = ":"

type MessageType int

const (
	MessageCommand MessageType = iota
	MessageText
	MessageCorrupted
)

type Command int

const (
	CommandLobbyExit = iota
	CommandLobbyReady
	CommandLobbyUnready
	CommandLobbyGameStarting
	CommandLobbyState

	CommandChoice

	CommandGameState

	CommandNil
)

//var commandList = []int{CommandLobbyExit, CommandLobbyReady, CommandLobbyUnready, CommandNil}

type Message struct {
	Type    MessageType
	Cmd     Command
	Content string
}

func (msg *Message) Parse() []byte {

	s := ""

	/*switch msg.Type {
	case MessageCommand:
		s = fmt.Sprintf("%d%s%d%s%s", msg.Type, TERMINATOR, msg.Cmd, TERMINATOR, msg.Content)
	case MessageText:
		s = fmt.Sprintf("%d%s%s", msg.Type, TERMINATOR, msg.Content)
	case MessageCorrupted:
		s = fmt.Sprintf("%d%sCORRUPTED", MessageCorrupted, TERMINATOR)
		fmt.Println("CORRUPTED MESSAGE (Parse())")
	}*/

	s = fmt.Sprintf("%d%s%d%s%s", msg.Type, TERMINATOR, msg.Cmd, TERMINATOR, msg.Content)

	return []byte(s)
}

func CreateTextMessage(s string) *Message {
	return &Message{
		Type:    MessageText,
		Cmd:     CommandNil,
		Content: s,
	}
}

func CreateCommandMessage(cmd Command, s string) *Message {
	return &Message{
		Type:    MessageCommand,
		Cmd:     cmd,
		Content: s,
	}
}

func ToMessage(b []byte) Message {
	s := string(b)
	parts := strings.Split(s, TERMINATOR)
	var mType MessageType = MessageCorrupted
	var cmd Command = CommandNil
	var content string = ""
	if len(parts) == 3 || len(parts) == 2 {
		switch parts[0] {
		case "0":
			mType = MessageCommand
			c, err := strconv.Atoi(parts[1])
			if err != nil {
				mType = MessageCorrupted
				break
			}
			cmd = Command(c)
		case "1":
			mType = MessageText
		default:
			mType = MessageCorrupted
			cmd = CommandNil
		}

		if len(parts) == 3 {
			content = parts[2]
		} else {
			content = ""
		}
	}

	if mType == MessageCorrupted {
		fmt.Println("CORRUPTED MESSAGE (ToMessage()):", s)
	}

	return Message{
		mType,
		cmd,
		content,
	}

}

/*func toCommand(s string) Command {

	i, err := strconv.Atoi(s)
	if err != nil {
		return CommandNil
	}

	if i < 0 || i >= len(commandList) {
		return CommandNil
	}

	return Command(i)

}*/
