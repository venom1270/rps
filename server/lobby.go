package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/venom1270/RPS/game"
	"github.com/venom1270/RPS/messaging"
)

type Player struct {
	clientId string
	ready    bool
}

// subscriber represents a subscriber.
// Messages are sent on the msgs channel and if the client
// cannot keep up with the messages, closeSlow is called.
type subscriber struct {
	id     int
	player *Player
	msgs   chan []byte

	readCmdCh chan int
	readMsgCh chan []byte
	readErrCh chan error

	closeSlow func()
	c         *websocket.Conn
}

type Lobby struct {
	id         string
	players    []Player
	maxPlayers int
	state      string
	game       *game.Game
	server     *gameServer

	// Websocket stuff
	subscriberMessageBuffer int
	subscriberIdCount       int
	logf                    func(f string, v ...interface{})
	subscribersMu           sync.Mutex
	subscribers             []*subscriber

	// Async game mutex
	inputMutex sync.Mutex
}

func (l Lobby) String() string {
	return fmt.Sprintf("%s,%d,%d,%s", l.id, len(l.players), l.maxPlayers, l.state)
}

func (l *Lobby) exitLobby(clientId string) bool {

	log.Println("Exit lobby accepted:", clientId)

	ok := false

	for i, v := range l.players {
		if v.clientId == clientId {
			l.players = append(l.players[:i], l.players[i+1:]...)
			ok = true
			break
		}
	}

	if !ok {
		log.Printf("Player %s not found in lobby %s", clientId, l.id)
	}

	for i, v := range l.subscribers {
		if v.player.clientId == clientId {
			l.subscribers[i].c.Close(websocket.StatusGoingAway, "Lobby exit on request")
			// Sometimes a read error gets logged - this is probably because a ead operation is running somewhere
			log.Printf("Connection with player %s closed!", clientId)
			l.sendLobbyState()
			l.publish(messaging.CreateTextMessage("EXIT " + clientId).Parse())
			return true
		}
	}

	return false
}

func (l *Lobby) ready(clientId string) bool {
	for ip, p := range l.players {
		if p.clientId == clientId {
			l.players[ip].ready = true
			log.Println("Ready for clientId", clientId, "success!")
			l.sendLobbyState()
			go l.checkStartGame()
			return true
		}
	}
	return false
}

func (l *Lobby) unready(clientId string) bool {
	for ip, p := range l.players {
		if p.clientId == clientId {
			l.players[ip].ready = false
			log.Println("Unready for clientId", clientId, "success!")
			l.sendLobbyState()
			return true
		}
	}
	return false
}

// Sends lobby state to all connected clients (every change etc...)
func (l *Lobby) sendLobbyState() {
	log.Printf("Sending lobby state...")
	l.publish(messaging.CreateCommandMessage(messaging.CommandLobbyState, l.getState()).Parse())
}

func (l *Lobby) getState() string {
	// Format: lobby#playerName[string]_ready[0/1];...
	stateStr := l.id + "#"
	for _, p := range l.players {
		rStr := "0"
		if p.ready {
			rStr = "1"
		}
		stateStr += p.clientId + "_" + rStr + ";"
	}
	if stateStr[len(stateStr)-1] == ';' {
		stateStr = stateStr[0 : len(stateStr)-1]
	}

	return stateStr
}

// subscribe subscribes the given WebSocket to all broadcast messages.
// It creates a subscriber with a buffered msgs chan to give some room to slower
// connections and then registers the subscriber. It then listens for all messages
// and writes them to the WebSocket. If the context is cancelled or
// an error occurs, it returns and deletes the subscription.
//
// It uses CloseRead to keep reading from the connection to process control
// messages and cancel the context if the connection drops.
func (l *Lobby) subscribe(w http.ResponseWriter, r *http.Request, player *Player) error {
	var mu sync.Mutex
	var c *websocket.Conn
	var closed bool
	s := &subscriber{
		id:        l.subscriberIdCount,
		player:    player,
		msgs:      make(chan []byte, l.subscriberMessageBuffer),
		readCmdCh: make(chan int, l.subscriberMessageBuffer),
		readMsgCh: make(chan []byte, l.subscriberMessageBuffer),
		readErrCh: make(chan error, l.subscriberMessageBuffer),
		closeSlow: func() {
			mu.Lock()
			defer mu.Unlock()
			closed = true
			if c != nil {
				c.Close(websocket.StatusPolicyViolation, "connection too slow to keep up with messages")
			}
		},
	}

	l.subscriberIdCount++
	l.addSubscriber(s)
	defer l.deleteSubscriber(s)

	c2, err := websocket.Accept(w, r, nil)
	if err != nil {
		return err
	}
	mu.Lock()
	if closed {
		mu.Unlock()
		return net.ErrClosed
	}
	c = c2
	s.c = c
	mu.Unlock()
	defer c.CloseNow()

	c.Write(context.Background(), websocket.MessageText, messaging.CreateTextMessage("Welcome to lobby "+l.id).Parse())

	//ctx := c.CloseRead(context.Background()) // This closes the connection after one read (???!!!)
	ctx := context.Background()
	log.Printf("!! Client connected to lobby %s !!", l.id)

	// Send message to everyone that someone joined
	l.publishExcept(messaging.CreateTextMessage("JOINED "+player.clientId).Parse(), player.clientId)
	l.sendLobbyState()

	go func() {
		for {
			_, m, err := c.Read(ctx)

			log.Printf("READING: %s %v", m, err != nil)

			if err != nil {
				s.readErrCh <- err
				continue // return maybe??
			}

			msg := messaging.ToMessage(m)

			switch msg.Type {
			case messaging.MessageCommand:
				s.readCmdCh <- int(msg.Cmd)
				continue
			case messaging.MessageText:
				s.readMsgCh <- []byte(msg.Content)
				continue
			case messaging.MessageCorrupted:
				log.Printf("Ignoring received corrupted message from %s: %s", player.clientId, msg.Content)
				continue
			}
		}
	}()

	for {
		select {
		case msg := <-s.msgs:
			err := writeTimeout(ctx, time.Second*5, c, msg)
			if err != nil {
				log.Println("Client disconnected from websocket")
				l.exitLobby(player.clientId)
				return err
			}

		case cmd := <-s.readCmdCh:
			switch cmd {
			case messaging.CommandGameState:
				log.Printf("GAME STATE REQUEST")
				// Get player names
				var playerIds []string
				for _, s := range l.players {
					playerIds = append(playerIds, s.clientId)
				}
				gameDetails := l.game.GetGameDetails(playerIds)
				fmt.Printf("Sending detail string: %s", gameDetails)
				/*gameStateMsg := messaging.Message{
					Type:    messaging.MessageCommand,
					Cmd:     messaging.CommandGameState,
					Content: gameDetails,
				}*/
				c.Write(ctx, websocket.MessageText, messaging.CreateCommandMessage(messaging.CommandGameState, gameDetails).Parse())
			case messaging.CommandLobbyExit:
				log.Printf("EXIT LOBBY")
				// Get player names
				l.exitLobby(player.clientId)
			case messaging.CommandLobbyReady:
				log.Printf("READY")
				ok := l.ready(player.clientId)
				c.Write(ctx, websocket.MessageText, messaging.CreateTextMessage(fmt.Sprintf("%t", ok)).Parse())
				fmt.Println("VSE OK!")
			case messaging.CommandLobbyUnready:
				log.Printf("UNREADY")
				ok := l.unready(player.clientId)
				c.Write(ctx, websocket.MessageText, messaging.CreateTextMessage(fmt.Sprintf("%t", ok)).Parse())
			case 123:
				// Ping operation, do nothing for now... maybo do "Pong" in the future
				log.Printf("Ping received")
				c.Write(ctx, websocket.MessageText, messaging.CreateTextMessage("Pong").Parse()) // TODO: CMD???
			default:
				log.Printf("UNKNOWN CMD: %d", cmd)
			}

		case err := <-s.readErrCh:
			log.Printf("Error parsing client message: %v", err)
			err = writeTimeout(ctx, time.Second*5, c, []byte("IS_ALIVE"))
			if err != nil {
				// TODO: THis triggers after game ends and lobby gets disbanded... HOW TO FIX??
				log.Println("Client disconnected from websocket")
				l.exitLobby(player.clientId)
				return err
			}
		case <-ctx.Done():
			log.Println("Client disconnected from websocket")
			l.exitLobby(player.clientId)
			return ctx.Err()
		}
	}
}

// publish publishes the msg to all subscribers.
// It never blocks and so messages to slow subscribers
// are dropped.
func (l *Lobby) publish(msg []byte) {
	l.subscribersMu.Lock()
	defer l.subscribersMu.Unlock()

	//cs.publishLimiter.Wait(context.Background())

	for i, _ := range l.subscribers {
		s := l.subscribers[i]
		select {
		case s.msgs <- msg:
		default:
			go s.closeSlow()
		}
	}
}

func (l *Lobby) publishExcept(msg []byte, clientId string) {
	l.subscribersMu.Lock()
	defer l.subscribersMu.Unlock()

	//cs.publishLimiter.Wait(context.Background())

	for i, _ := range l.subscribers {
		s := l.subscribers[i]
		if s.player.clientId == clientId {
			continue
		}
		select {
		case s.msgs <- msg:
		default:
			go s.closeSlow()
		}
	}
}

func (l *Lobby) publishToClient(msg []byte, clientId int) {
	l.subscribersMu.Lock()
	defer l.subscribersMu.Unlock()

	//log.Printf("Trying to send message to client", clientId, "with subscriber cound: ", len(cs.subscribers))
	//cs.publishLimiter.Wait(context.Background())

	for i, _ := range l.subscribers {
		s := l.subscribers[i]
		fmt.Println("sub: ", s)
		if s.id == clientId {
			log.Printf("Sending to client...")
			select {
			case s.msgs <- msg:
			default:
				go s.closeSlow()
			}
			break
		}
	}
}

// addSubscriber registers a subscriber.
func (l *Lobby) addSubscriber(s *subscriber) {
	l.subscribersMu.Lock()
	l.subscribers = append(l.subscribers, s)
	l.subscribersMu.Unlock()
}

// deleteSubscriber deletes the given subscriber.
func (l *Lobby) deleteSubscriber(s *subscriber) {
	l.subscribersMu.Lock()

	log.Printf("DELETING SUBSCRIBER :(")

	for i, _ := range l.subscribers {
		if l.subscribers[i] == s {
			l.subscribers = append(l.subscribers[:i], l.subscribers[i+1:]...)
			break
		}
	}
	l.subscribersMu.Unlock()
}

func (l *Lobby) checkStartGame() {

	if len(l.players) < l.maxPlayers {
		return
	}

	for _, p := range l.players {
		if !p.ready {
			return
		}
	}

	log.Printf("Game is starting in lobby: %s", l.id)

	l.publish(messaging.CreateCommandMessage(messaging.CommandLobbyGameStarting, "").Parse())

	time.Sleep(5 * time.Second)
	log.Printf("Game starting NOW!")
	// Start game!!
	go l.startGame()
}

func (l *Lobby) startGame() {
	l.game = game.NewGame()

	var wg sync.WaitGroup

	getPlayerInput := func(player int) {
		ctx := context.Background()

		for {
			msg := <-l.subscribers[player].readMsgCh

			choice, err := strconv.Atoi(string(msg))
			if err != nil {
				log.Printf("Error converting choice to int... %v", err)
				l.subscribers[player].c.Write(ctx, websocket.MessageText, messaging.CreateTextMessage("Invalid choice type").Parse())
				continue
			}

			if choice < 0 || choice > 3 {
				log.Printf("Invalid choice: %d", choice)
				l.subscribers[player].c.Write(ctx, websocket.MessageText, messaging.CreateTextMessage("Invalid choice").Parse())
				continue
			}
			ok, _ := l.game.MakeChoice(player, intToPlayerChoice(choice))
			if !ok {
				log.Println("Something went wrong, choice not ok!")
				l.subscribers[player].c.Write(ctx, websocket.MessageText, messaging.CreateTextMessage("Game could not accept choice").Parse())
				continue
			}
			log.Println("Player made a choice! Sending OK response")
			l.subscribers[player].c.Write(ctx, websocket.MessageText, messaging.CreateTextMessage("OK").Parse())
			break
		}

		wg.Done()
	}

	for !l.game.IsFinished() {
		// Wait for input from both players
		wg.Add(2)

		l.publish(messaging.CreateTextMessage("0").Parse())

		go getPlayerInput(0)
		go getPlayerInput(1)

		wg.Wait()

		for !l.game.IsRoundFinished() {
			// Wait until round finished...

		}

		winner := l.game.CompleteRound()
		l.publish(messaging.CreateTextMessage("Winner: " + strconv.Itoa(winner)).Parse())

		log.Println("ROUND COMPLETED!")

		// TODO: new input signel etc...
	}

	log.Println("GAME FINISHED!!!!")
	winner := l.game.GetWinner()
	l.publish(messaging.CreateTextMessage("Player " + strconv.Itoa(winner) + " WON THE GAME!").Parse())

	l.publish(messaging.CreateTextMessage("1").Parse())

	time.Sleep(5 * time.Second)

	l.server.disbandLobby(l)

}
