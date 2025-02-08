package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/venom1270/RPS/game"
)

type Player struct {
	clientId string
	ready    bool
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

type gameServer struct {
	// serveMux routes the various endpoints to the appropriate handler.
	serveMux http.ServeMux
	// LOBBIES
	lobbies []*Lobby
}

func newGameServer() *gameServer {
	cs := &gameServer{}
	cs.serveMux.Handle("/", http.FileServer(http.Dir(".")))
	cs.serveMux.HandleFunc("/subscribe/", cs.subscribeHandler) // lobbyId PATH parameter required
	//cs.serveMux.HandleFunc("/publish", cs.publishHandler) We don't need publish righjt not???

	// Lobby functions
	cs.serveMux.HandleFunc("/getLobbyList", cs.getLobbyList)
	cs.serveMux.HandleFunc("/createLobby", cs.createLobby)
	cs.serveMux.HandleFunc("/joinLobby", cs.joinLobby)
	cs.serveMux.HandleFunc("/exitLobby", cs.exitLobby)
	cs.serveMux.HandleFunc("/ready", cs.ready)

	return cs
}

func (cs *gameServer) getMsg(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	if r.Method != "POST" {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return []byte{}, errors.New("Wrong method")
	}
	body := http.MaxBytesReader(w, r.Body, 8192)
	msg, err := io.ReadAll(body)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusRequestEntityTooLarge), http.StatusRequestEntityTooLarge)
		return []byte{}, errors.New("Error reading meassage")
	}
	return msg, nil
}

// Splits client string. Returns clientId, rest of msg, error
func (cs *gameServer) splitClientMsg(msg []byte) (string, string, error) {
	msgStr := string(msg)
	if len(msg) < 1 {
		return "", "", errors.New("msg len is 0")
	}
	split := strings.Split(msgStr, " ")
	clientId := split[0]
	/*if err != nil {
		return "", "", errors.New("error converting clientId to int")
	}*/

	body := ""
	if len(split) > 1 {
		body = strings.Join(split[1:], " ")
	}

	return clientId, body, nil

}

func (cs *gameServer) getLobbyList(w http.ResponseWriter, r *http.Request) {
	msg, err := cs.getMsg(w, r)
	if err != nil {
		// Error occured, ignore
		return
	}

	_, _, err = cs.splitClientMsg(msg)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Println("error splitting client msg")
		return
	}

	w.WriteHeader(http.StatusAccepted)

	// Format lobby string
	responseStr := ""
	for _, l := range cs.lobbies {
		responseStr += l.String() + ";"
	}
	if len(responseStr) > 0 {
		responseStr = responseStr[:len(responseStr)-1]
	} else {
		responseStr = "No lobbies!"
	}

	log.Println(responseStr)

	w.Write([]byte(responseStr))
}

func (cs *gameServer) createLobby(w http.ResponseWriter, r *http.Request) {
	msg, err := cs.getMsg(w, r)
	if err != nil {
		// Error occured, ignore
		return
	}

	_, msgStr, err := cs.splitClientMsg(msg)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Println("error splitting client msg", err)
		return
	}

	log.Println("Mesage accepted:", msg)

	exists := cs.getLobbyByName(msgStr)
	if exists != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Printf("Lobby creation failed - '%s' already exists", msgStr)
		return
	}

	newLobby := &Lobby{
		id:         msgStr,
		maxPlayers: 2,
		state:      "CREATED",

		subscriberMessageBuffer: 16,
		subscriberIdCount:       0,
		logf:                    log.Printf,
		subscribers:             []*subscriber{},
		server:                  cs,
	}

	cs.lobbies = append(cs.lobbies, newLobby)

	w.WriteHeader(http.StatusAccepted)
	//cs.publishToClient([]byte(responseStr), clientId)
}

func (cs *gameServer) joinLobby(w http.ResponseWriter, r *http.Request) {
	msg, err := cs.getMsg(w, r)
	if err != nil {
		// Error occured, ignore
		return
	}

	clientId, msgStr, err := cs.splitClientMsg(msg)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Println("error splitting client msg", err)
		return
	}

	log.Println("Mesage accepted:", msg)

	for i, l := range cs.lobbies {
		if l.id == msgStr {
			if len(l.players) < l.maxPlayers {
				cs.lobbies[i].players = append(cs.lobbies[i].players, Player{clientId: clientId, ready: false})
				log.Printf("Joining player %s to lobby %s successful! %d/%d", clientId, l.id, len(l.players), l.maxPlayers)
				w.WriteHeader(http.StatusAccepted)
				w.Write([]byte(l.id))
				return
			} else {
				log.Printf("Joining player %s to lobby %s fail! Not enough space!", clientId, l.id)
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			break
		}
	}

	w.WriteHeader(http.StatusBadRequest)
	//cs.publishToClient([]byte(responseStr), clientId)
}

func (cs *gameServer) exitLobby(w http.ResponseWriter, r *http.Request) {
	msg, err := cs.getMsg(w, r)
	if err != nil {
		// Error occured, ignore
		return
	}

	clientId, _, err := cs.splitClientMsg(msg)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Println("error splitting client msg", err)
		return
	}

	log.Println("Exit lobby accepted:", clientId)

	lobby := cs.getClientLobby(clientId)
	if cs.removeClientFromLobby(clientId, lobby) {
		w.WriteHeader(http.StatusAccepted)
		log.Println("Exit lobby success!")
	} else {
		w.WriteHeader(http.StatusBadRequest)
		log.Println("Exit lobby fail!")
	}

	//cs.publishToClient([]byte(responseStr), clientId)
}

func (cs *gameServer) getClientLobby(clientId string) string {
	for _, l := range cs.lobbies {
		for _, p := range l.players {
			if p.clientId == clientId {
				return l.id
			}
		}
	}
	return ""
}

func (cs *gameServer) removeClientFromLobby(clientId string, lobby string) bool {
	for li, l := range cs.lobbies {
		if l.id == lobby {
			for i, v := range cs.lobbies[li].players {
				if v.clientId == clientId {
					cs.lobbies[li].players = append(cs.lobbies[li].players[:i], cs.lobbies[li].players[i+1:]...)
					return true
				}
			}
			break
		}
	}
	return false
}

func (cs *gameServer) ready(w http.ResponseWriter, r *http.Request) {
	msg, err := cs.getMsg(w, r)
	if err != nil {
		// Error occured, ignore
		return
	}

	clientId, _, err := cs.splitClientMsg(msg)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Println("error splitting client msg", err)
		return
	}

	lobby := cs.getClientLobby(clientId)

	for i, l := range cs.lobbies {
		if l.id == lobby {
			for ip, p := range cs.lobbies[i].players {
				if p.clientId == clientId {
					cs.lobbies[i].players[ip].ready = true
					log.Println("Ready for clientId", clientId, "success!")
					w.WriteHeader(http.StatusAccepted)
					go l.checkStartGame()
					return
				}
			}
			break
		}
	}

	w.WriteHeader(http.StatusBadRequest)

}

func (cs *gameServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cs.serveMux.ServeHTTP(w, r)
}

// subscriber represents a subscriber.
// Messages are sent on the msgs channel and if the client
// cannot keep up with the messages, closeSlow is called.
type subscriber struct {
	id   int
	msgs chan []byte

	readCmdCh chan int
	readMsgCh chan []byte
	readErrCh chan error

	closeSlow func()
	c         *websocket.Conn
}

func (cs *gameServer) getLobbyByName(name string) *Lobby {
	for i, _ := range cs.lobbies {
		if cs.lobbies[i].id == name {
			return cs.lobbies[i]
		}
	}
	return nil
}

// subscribeHandler accepts the WebSocket connection and then subscribes
// it to all future messages.
func (cs *gameServer) subscribeHandler(w http.ResponseWriter, r *http.Request) {

	lobbyId := strings.TrimPrefix(r.URL.Path, "/subscribe/")
	lobby := cs.getLobbyByName(lobbyId)

	if lobbyId == "" || lobby == nil {
		log.Printf("Lobby with id '%s' does not exist!", lobbyId)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err := lobby.subscribe(w, r)

	if errors.Is(err, context.Canceled) {
		return
	}
	if websocket.CloseStatus(err) == websocket.StatusNormalClosure ||
		websocket.CloseStatus(err) == websocket.StatusGoingAway {
		return
	}

	if websocket.CloseStatus(err) == -1 {
		cs.disbandLobby(lobby)
	}
	if err != nil {
		//cs.logf("%v", err)
		log.Printf("%v", err)
		return
	}
}

func (cs *gameServer) disbandLobby(l *Lobby) {
	log.Printf("Disconnecting subscribers from lobby %s", l.id)
	for i, _ := range l.subscribers {
		if len(l.subscribers) > i && l.subscribers[i] != nil { // TODO: len check is a workaround for deleting subscibers...
			l.subscribers[i].c.Close(websocket.StatusAbnormalClosure, "TIMOUT")
		}
	}

	log.Printf("Removing lobby %s", l.id)
	for i, _ := range cs.lobbies {
		if cs.lobbies[i].id == l.id {
			cs.lobbies = append(cs.lobbies[:i], cs.lobbies[i+1:]...)
			break
		}
	}

	log.Printf("Done disbanding lobby %s", l.id)

}

// publishHandler reads the request body with a limit of 8192 bytes and then publishes
// the received message.
func (l *Lobby) publishHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	body := http.MaxBytesReader(w, r.Body, 8192)
	msg, err := io.ReadAll(body)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusRequestEntityTooLarge), http.StatusRequestEntityTooLarge)
		return
	}

	l.publish(msg)

	w.WriteHeader(http.StatusAccepted)
}

// subscribe subscribes the given WebSocket to all broadcast messages.
// It creates a subscriber with a buffered msgs chan to give some room to slower
// connections and then registers the subscriber. It then listens for all messages
// and writes them to the WebSocket. If the context is cancelled or
// an error occurs, it returns and deletes the subscription.
//
// It uses CloseRead to keep reading from the connection to process control
// messages and cancel the context if the connection drops.
func (l *Lobby) subscribe(w http.ResponseWriter, r *http.Request) error {
	var mu sync.Mutex
	var c *websocket.Conn
	var closed bool
	s := &subscriber{
		id:        l.subscriberIdCount,
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

	c.Write(context.Background(), websocket.MessageText, []byte("Welcome to lobby "+l.id))

	//ctx := c.CloseRead(context.Background()) // This closes the connection after one read (???!!!)
	ctx := context.Background()
	log.Printf("!! Client connected to lobby %s !!", l.id)

	go func() {
		for {
			_, message, err := c.Read(ctx)

			if err != nil {
				s.readErrCh <- err
				continue // return maybe??
			}

			if len(message) > 4 && string(message[0:4]) == "CMD:" {
				command, err := strconv.Atoi(string(message)[4:])
				if err != nil {
					s.readErrCh <- err
					continue
				}
				s.readCmdCh <- command
				continue
			}

			// Normal message
			s.readMsgCh <- message
		}
	}()

	for {
		select {
		case msg := <-s.msgs:
			//c.Write(ctx, websocket.MessageBinary, []byte("QWEQWEQWE"))
			err := writeTimeout(ctx, time.Second*5, c, msg)
			if err != nil {
				log.Println("Client disconnected from websocket")
				return err
			}

		case cmd := <-s.readCmdCh:
			switch cmd {
			case 555:
				log.Printf("GOR 555 CMD!!")
				msg := "** SCORES **\n"
				for i, s := range l.game.GetScores() {
					msg += fmt.Sprintf("Player %d: %d\n", i, s)
				}
				c.Write(ctx, websocket.MessageText, []byte(msg))
				// TODO: other commands....
			case 556:
				log.Printf("GAME STATE REQUEST")
				// Get player names
				var playerIds []string
				for _, s := range l.players {
					playerIds = append(playerIds, s.clientId)
				}
				gameDetails := "CMD:556 " + l.game.GetGameDetails(playerIds)
				fmt.Printf("Sending detail string: %s", gameDetails)
				c.Write(ctx, websocket.MessageText, []byte(gameDetails))
				// TODO: other commands....
			case 123:
				// Ping operation, do nothing for now... maybo do "Pong" in the future
			default:
				log.Printf("CMD: %d", cmd)
			}

		case err := <-s.readErrCh:
			log.Printf("Error parsing client message: %v", err)
			err = writeTimeout(ctx, time.Second*5, c, []byte("IS_ALIVE"))
			if err != nil {
				// TODO: THis triggers after game ends and lobby gets disbanded... HOW TO FIX??
				log.Println("Client disconnected from websocket")
				return err
			}
		case <-ctx.Done():
			log.Println("Client disconnected from websocket")
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
				log.Printf("Error converting msg to choice (int)... %v", err)
				l.subscribers[player].c.Write(ctx, websocket.MessageText, []byte("Invalid choice type"))
				continue
			}
			if choice < 0 || choice > 3 {
				log.Printf("Invalid choice: %d", choice)
				l.subscribers[player].c.Write(ctx, websocket.MessageText, []byte("Invalid choice"))
				continue
			}
			ok, _ := l.game.MakeChoice(player, intToPlayerChoice(choice))
			if !ok {
				log.Println("Something went wrong, choice not ok!")
				l.subscribers[player].c.Write(ctx, websocket.MessageText, []byte("Game could not accept choice"))
				continue
			}
			log.Println("Player made a choice! Sending OK response")
			l.subscribers[player].c.Write(ctx, websocket.MessageText, []byte("OK"))
			break
		}

		wg.Done()
	}

	for !l.game.IsFinished() {
		// Wait for input from both players
		wg.Add(2)

		l.publish([]byte("0"))

		go getPlayerInput(0)
		go getPlayerInput(1)

		wg.Wait()

		for !l.game.IsRoundFinished() {
			// Wait until round finished...

		}

		winner := l.game.CompleteRound()
		//l.publish([]byte("Player " + strconv.Itoa(winner) + " won the round!"))
		l.publish([]byte("Winner: " + strconv.Itoa(winner)))

		log.Println("ROUND COMPLETED!")

		// TODO: new input signel etc...
	}

	log.Println("GAME FINISHED!!!!")
	winner := l.game.GetWinner()
	l.publish([]byte("Player " + strconv.Itoa(winner) + " WON THE GAME!"))

	l.publish([]byte("1"))

	time.Sleep(5 * time.Second)

	l.server.disbandLobby(l)

}

func intToPlayerChoice(choice int) game.PlayerChoice {
	switch choice {
	case int(game.ROCK):
		return game.ROCK
	case int(game.SCISSORS):
		return game.SCISSORS
	case int(game.PAPER):
		return game.PAPER
	case int(game.JOKER):
		return game.JOKER
	}
	return game.ROCK // TODO!!!
}

func writeTimeout(ctx context.Context, timeout time.Duration, c *websocket.Conn, msg []byte) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return c.Write(ctx, websocket.MessageText, msg)
}
