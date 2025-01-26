# RPS - Multiplayer Rock-Paper-Scissors game in GO

Proof-of-concept GO server for hosting a simple Rock-Paper-Scissors (RPS) server.

## Features

- Game server
  - Hosting lobbies (max 2 players)
  - Game logic
- Game client
  - Make and join lobbies (REST)
  - Play the game (websockets)

Common scripts for running a server, clients, and building Docker images are provided.

## Hosting

The game is currently hosted on [Render](https://render.com/).

URL: `https://rps-e20j.onrender.com`

They offer a great free plan without the need to input a credit card!

## The game

The game is a simple *RPS* game with an additional twist - the *JOKER*.

The *Joker* card always defeats *Rock* and *Paper*. But since it's a paper card, it struggles against *Scissors*. It might seem lika a good bet, but beware - losing with the *Joker* reduces your points by 1 (-1)! While you can't go below 0 points, you might think "oh I can just start the game by choosing the *Joker*" - but your opponent might think the same and counter you, so it's not that straightforward!

What if both players choose the *Joker*? Then a coin is flipped and a winner is determined randomly. The loser still gets a penalty of -1 points!

The winner is the player that reaches 3 points first!

## Implementation

The server and game flow are implemented in two steps: *lobby management* and the *game*.

### Lobby management

Lobby management is done using standard HTTP/REST requests. Those requests include:
- `/getLobbyList`: gets a list of current lobbies as a string in format: `<LOBBY_NAME>,<PLAYERS>,<MAX_PLAYERS>,<STATE>;...`
- `/createLobby`: create a new lobby (*requires body string*)
- `/joinLobby`: join specified lobby (*requires body string*)
- `/exitLobby`: exit current lobby
- `/ready`: set ready  and wait for server signal for game start - this will estabilsh a websocket connection from client to server, and block all input until server signal

All methods require body string in the form of `<clientId> <rest of the message>`, for example `1 myLobby`. 

When client is ready, a websocket connection is established by calling the `/subscribe/<lobby>` method.

### Websockets

Websocket controls the whole flow of the game. The flow looks roughly like this:

- Wait for game start/input signal from server (`0`)
- Read player/client input (0-3) and send it to server
    - Player input can also be a *command*: `CMD:X`, where `X` is a number. Currently only command `555` is supported - it retrieves the current score
- If input was confirmed, server responds with `OK`, otherwise repeat
- Wait for input signal (first bullet point), but if the signal is `1`, end the game

Simulataneously, a goroutine is sending Ping signals every few seconds to check for potential disconnects. If a timeout is detected, connection gets closed. Pinging is not well supported in the `coder/websocket` library, so I used a custom command: `CMD:123`.

On server side, when one client disconnects, the lobby gets disbanded (all open connections to the lobby get closed).

In short: there are two kinds of messages: *normal* and *command* messages. *Command* messages are in the form of `CMD:X`, while all other messages are considered *normal*.

There may still be some situations where unexpected network interruptions break the clients (or server), but the most common cases should be handled by the current implementation.

## Future

The idea is to have a standalone custom game server that I can build any kind of client I want to. The next step (besides polishing the server) would be to make a client with some nice UI/graphics, such as Unity.