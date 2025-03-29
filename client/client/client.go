package client

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/venom1270/RPS/messaging"
)

type ClientState int

const (
	CONNECTED      ClientState = iota // 0
	IN_LOBBY                          // 1
	IN_LOBBY_READY                    // 2
	IN_GAME                           // 3
)

type Client struct {
	url string
	c   *websocket.Conn
	id  string

	State ClientState
	Lobby string
	ctx   context.Context
}

func NewClient(url string, clientId string) *Client {

	cl := &Client{
		id:    clientId,
		State: CONNECTED,
		url:   url,
	}

	return cl
}

func (cl *Client) Connect(ctx context.Context, url string, method, lobby string) error {

	log.Printf("Trying to connect client '%d' to lobby '%s'", cl.id, lobby)

	finalUrl := url + "/" + method + "/" + lobby + "/" + cl.id

	log.Printf("Final URL: %s", finalUrl)

	c, _, err := websocket.Dial(ctx, finalUrl, nil)
	if err != nil {
		return err
	}

	/*c, _, err := websocket.Dial(ctx, url+"/subscribe/"+lobby, nil)
	if err != nil {
		return err
	}*/

	cl.c = c
	cl.ctx = ctx

	log.Printf("Client  with id '%s' connected to lobby '%s'", cl.id, cl.Lobby)

	go func() {

		fmt.Println("Initilizing ping every 10 seconds...")
		for {

			select {
			case <-cl.ctx.Done():
				fmt.Println("CTX closed, stopping pings...")
				return
			default:
				//pingMsg := messaging.Message{messaging.MessageCommand, messaging.Command(123), "PING"}
				//err := c.Write(ctx, websocket.MessageText, pingMsg.Parse())
				//log.Println("Ping sent....")
				if err != nil {
					log.Println("Ping error or server disconnected:", err)
					log.Println("SERVER DISCONNECTED! To continue you may have to input something")
					log.Printf("%v", cl.Close())
					return
				}
			}
			time.Sleep(10 * time.Second)
		}
	}()

	return nil
}

func (cl *Client) SendMessage(msg string) error {
	log.Printf("SENDING MESSAGE: %s", msg)
	return cl.c.Write(cl.ctx, websocket.MessageText, []byte(msg))
}

func (cl *Client) SendMessage2(msg messaging.Message) error {
	log.Printf("SENDING MESSAGE: %v", msg)
	return cl.c.Write(cl.ctx, websocket.MessageText, []byte(msg.Parse()))
}

func (cl *Client) CallMethod(ctx context.Context, msg string, method string) (body string, err error) {

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, cl.url+"/"+method, strings.NewReader(cl.id+" "+msg))
	resp, err := http.DefaultClient.Do(req)

	/*defer func(resp *http.Response) {
		if err != nil && resp.StatusCode != http.StatusBadRequest {
			cl.c.Close(websocket.StatusInternalError, "publish failed")
		}
	}(resp)*/

	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("publish request failed: %v", resp.StatusCode)
	}
	if resp.StatusCode == http.StatusBadRequest {
		// TODO: send soemthing to client that says invalid request
		return "", fmt.Errorf("Bad request")
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error reading response body....")
		return "", nil
	}

	return string(bodyBytes), nil
}

func (cl *Client) NextMessage() (string, error) {
	typ, b, err := cl.c.Read(context.Background())
	if err != nil {
		return "", err
	}

	if typ != websocket.MessageText {
		cl.c.Close(websocket.StatusUnsupportedData, "expected text message")
		return "", fmt.Errorf("expected text message but got %v", typ)
	}
	return string(b), nil
}

func (cl *Client) Close() error {
	cl.State = CONNECTED
	cl.Lobby = ""
	return cl.c.Close(websocket.StatusNormalClosure, "")
}
