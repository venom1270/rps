package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/venom1270/RPS/client"
	"github.com/venom1270/RPS/messaging"
)

var ctx context.Context
var cancel context.CancelFunc
var cl *client.Client

func main() {
	log.SetFlags(0)

	err := run()
	if err != nil {
		log.Fatal(err)
	}
}

// run initializes the chatServer and then
// starts a http.Server for the passed in address.
func run() error {
	if len(os.Args) < 2 {
		return errors.New("please provide an address to listen on as the first argument and this client's ID as the second")
	}

	url := os.Args[1]
	clientId := os.Args[2]
	return startClient(url, clientId)
}

func startClient(url string, clientId string) error {

	//ctx, cancel := context.WithTimeout(context.Background(), time.Minute*10)
	//defer cancel()
	ctx, cancel = context.WithCancel(context.Background())

	cl = client.NewClient(url, clientId)

	response := ""

	for {

		switch cl.State {
		case client.CONNECTED:
			fmt.Println("STATE: Connected to server")
		case client.IN_LOBBY:
			fmt.Println("STATE: In lobby:", cl.Lobby)
		case client.IN_LOBBY_READY:
			fmt.Println("STATE: READY, in lobby:", cl.Lobby)
		case client.IN_GAME:
			fmt.Println("STATE: IN GAME, lobby:", cl.Lobby)
		}

		fmt.Printf("\n *** OPTIONS ***\n1: getLobbyList\n2: createLobby [name]\n3: joinLobby [name]\n4: exitLobby\n5: SET READY (final, if in lobby)\n*******\n")

		var err error
		method := ""
		fmt.Scan(&method)

		msg := ""
		fmt.Scan(&msg)
		switch method {
		case "1":
			response, err = cl.CallMethod(ctx, msg, "getLobbyList")
			if err != nil {
				fmt.Println(err)
			} else {
				fmt.Println(response)
			}
		case "2":
			err = cl.Connect(ctx, url, "createLobby", msg)
			if err != nil {
				log.Printf("ERROR CREATING AND JOINING TO LOBBY!!! %v", err)
				break
			}
			cl.State = client.IN_LOBBY
			cl.Lobby = msg

			websocketHandling()

		case "3":
			err = cl.Connect(ctx, url, "joinLobby", msg)
			if err != nil {
				log.Printf("ERROR JOINING TO LOBBY!!! %v", err)
				break
			}

			cl.State = client.IN_LOBBY
			cl.Lobby = msg

			websocketHandling()

		case "4":
			response, err = cl.CallMethod(ctx, msg, "exitLobby")
			if err != nil {
				fmt.Println(err)
			}
			cl.State = client.CONNECTED
		case "5":
			response, err = cl.CallMethod(ctx, msg, "ready")
			if err != nil {
				fmt.Println(err)
			}
			cl.State = client.IN_LOBBY_READY

			err = cl.Connect(ctx, url, "TODO_DELETE THIS", cl.Lobby)
			if err != nil {
				log.Printf("ERROR CONNECTING TO LOBBY!!! %v", err)
				break
			}

			// Start game - look at websocket messages
			websocketHandling()

		default:
			fmt.Println("INVALID METHOD!")
		}

	}

	return nil
}

func websocketHandling() {

	var err error

	serverOk := true
	gameEnd := false

	for serverOk {

		go func() {
			for {
				msg, err := cl.NextMessage()
				log.Printf("GOT MESSAGE: %s", msg)
				if err != nil {
					continue
				}
				if msg == "0" {
					fmt.Println("Input signal recived. Please input your choice (0-3)\n0 - ROCK\n1 - PAPER\n2 - SCISSORS\n3 - JOKER (dangerous card, defeated by SCISSORS and sometimes JOKER)\n")
					break
				} else if msg == "1" {
					fmt.Println("Game ended. Disconnecting...")
					gameEnd = true
					break
				} else {
					fmt.Println(msg)
				}
			}

			if gameEnd {
				//break
			}
		}()

		for {
			var choice string
			_, err = fmt.Scan(&choice)
			if err != nil {
				fmt.Printf("Invalid input! %v\n", err)
				continue
			}

			// To simulate ready
			if len(choice) > 1 && choice[0:2] == "0:" {
				// If it's a CMD message, server won't respond so we have to continue.
				cl.SendMessage(choice)
				// Get scores (test)
				//if choice == "CMD:555" || choice == "CMD:556" || choice == "CMD:0" {
				//r, _ := cl.NextMessage()
				//fmt.Println(r)
				//}

				// TODO: this is ugly :(
				continue
			}

			cl.SendMessage2(messaging.Message{
				Type:    messaging.MessageText,
				Cmd:     messaging.CommandChoice,
				Content: choice,
			})

			/*response, err := cl.NextMessage()
			if err != nil {
				fmt.Printf("Error receiving response from server, lobby probably disbanded due to timeout of one of the clients. %v", err)
				serverOk = false
				break
			}

			if response == "OK" {
				fmt.Println("Choice accepted, waiting for other player(s)...")
				break
			} else {
				fmt.Printf("Choice was not accepted! %s\n", response)
				continue
			}*/

		}
	}

	// GAME END
	cancel()

	// Create new context
	ctx, cancel = context.WithCancel(context.Background())
}
