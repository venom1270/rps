package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/venom1270/RPS/game"
)

type gameServer struct {
	// serveMux routes the various endpoints to the appropriate handler.
	serveMux http.ServeMux
	// LOBBIES
	lobbies []*Lobby
}

func newGameServer() *gameServer {
	cs := &gameServer{}
	cs.serveMux.Handle("/", http.FileServer(http.Dir(".")))

	// Lobby functions
	cs.serveMux.HandleFunc("/getLobbyList", cs.getLobbyList)
	cs.serveMux.HandleFunc("/createLobby/", cs.createLobbyHandler)
	cs.serveMux.HandleFunc("/joinLobby/", cs.joinLobbyHandler)

	return cs
}

func (cs *gameServer) getMsg(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	if r.Method != "POST" {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return []byte{}, errors.New("wrong method")
	}
	body := http.MaxBytesReader(w, r.Body, 8192)
	msg, err := io.ReadAll(body)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusRequestEntityTooLarge), http.StatusRequestEntityTooLarge)
		return []byte{}, errors.New("error reading meassage")
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

func (cs *gameServer) createLobbyHandler(w http.ResponseWriter, r *http.Request) {

	params := strings.Split(strings.TrimPrefix(r.URL.Path, "/createLobby/"), "/")

	lobbyId := params[0]
	clientId := params[1]

	log.Println("Mesage accepted with lobby name:", lobbyId)

	lobby := cs.createLobby(lobbyId)
	if lobby == nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	cs.joinLobby(w, r, lobby, clientId)
	//ok := cs.joinLobby(w, r, lobby, clientId)
	/*if !ok {
		w.WriteHeader(http.StatusBadRequest)
	}*/
}

func (cs *gameServer) createLobby(lobbyName string) *Lobby {
	exists := cs.getLobbyByName(lobbyName)
	if exists != nil {
		log.Printf("Lobby creation failed - '%s' already exists", lobbyName)
		return nil
	}

	newLobby := &Lobby{
		id:         lobbyName,
		maxPlayers: 2,
		state:      "CREATED",

		subscriberMessageBuffer: 16,
		subscriberIdCount:       0,
		logf:                    log.Printf,
		subscribers:             []*subscriber{},
		server:                  cs,
	}

	cs.lobbies = append(cs.lobbies, newLobby)

	return newLobby
}

func (cs *gameServer) joinLobbyHandler(w http.ResponseWriter, r *http.Request) {
	params := strings.Split(strings.TrimPrefix(r.URL.Path, "/joinLobby/"), "/")

	fmt.Println(r.URL.Path)
	fmt.Println(params)
	fmt.Println(params[0])

	lobbyId := params[0]
	clientId := params[1]

	log.Println("Mesage accepted with lobby name:", lobbyId)

	var lobby *Lobby
	for i, l := range cs.lobbies {
		if l.id == lobbyId {
			lobby = cs.lobbies[i]
			break
		}
	}

	if lobby == nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Printf("Error joining to lobby %s - it does not exist!", lobbyId)
		return
	}

	cs.joinLobby(w, r, lobby, clientId)
	/*ok := cs.joinLobby(w, r, lobby, clientId)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
	}*/
}

func (cs *gameServer) joinLobby(w http.ResponseWriter, r *http.Request, lobby *Lobby, clientId string) bool {

	player := Player{clientId: clientId, ready: false}
	if len(lobby.players) < lobby.maxPlayers {
		lobby.players = append(lobby.players, player)
		log.Printf("Joining player %s to lobby %s successful! %d/%d", clientId, lobby.id, len(lobby.players), lobby.maxPlayers)
	} else {
		log.Printf("Joining player %s to lobby %s fail! Not enough space!", clientId, lobby.id)
		return false
	}

	err := lobby.subscribe(w, r, &player)

	if errors.Is(err, context.Canceled) {
		return false
	}
	if websocket.CloseStatus(err) == websocket.StatusNormalClosure ||
		websocket.CloseStatus(err) == websocket.StatusGoingAway {
		return false
	}

	if websocket.CloseStatus(err) == -1 {
		// TODO: or if "host"?
		if len(lobby.players) == 0 {
			cs.disbandLobby(lobby)
		}
	}
	if err != nil {
		//cs.logf("%v", err)
		log.Printf("%v", err)
		return false
	}

	return true
}

func subscribeDelayCheck(w http.ResponseWriter, r *http.Request, lobby *Lobby, player *Player) error {
	errChan := make(chan error, 1)

	// Start the goroutine
	go func() {
		errChan <- lobby.subscribe(w, r, player)
	}()

	select {
	case err := <-errChan:
		if err != nil {
			fmt.Println("Error occurred:", err)
			return err
		} else {
			fmt.Println("No error!")
		}
	case <-time.After(2 * time.Second): // Timeout after 3 seconds
		fmt.Println("Timed out, moving on...")
	}

	return nil
}

func (cs *gameServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cs.serveMux.ServeHTTP(w, r)
}

func (cs *gameServer) getLobbyByName(name string) *Lobby {
	for i, _ := range cs.lobbies {
		if cs.lobbies[i].id == name {
			return cs.lobbies[i]
		}
	}
	return nil
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

	r := c.Write(ctx, websocket.MessageText, msg)
	return r
}
