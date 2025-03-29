# RPS - Multiplayer Rock-Paper-Scissors game in GO

Proof-of-concept GO server for hosting a simple Rock-Paper-Scissors (RPS) server.

## Features

- Game server (GO)
  - Hosting lobbies (max 2 players)
  - Game logic
- Game client (GO)
  - Make and join lobbies (REST)
  - Lobby state management (websockets)
  - Play the game (websockets)
- ~~Soon™~~ Unity game client (it's done!)

Common scripts for running a server, clients, and building Docker images are provided.

## Hosting

The game is currently hosted on [Render](https://render.com/).

URL: `https://rps-e20j.onrender.com`

They offer a great free plan without the need to input a credit card!

## Changelog

- Some server changes have been made to better accomodate Unity client
  - clientId changed from int to string
  - added command `556` (game state)
  - other minor changes regarding server responses

- There was a peculiar problem where comparison of two identical strings, e.g. `qwe` and `qwe` returned false. The issues was with wrong handling of input on the Unity client - the client sent string `qwe`, but with an added Unicode character at the end: `\u200b`, making the "real" string `qwe\u200b`. That is called *unicode zero-width space (ZWSP)*. I was sending the raw input from Unity InputField, instead of the "correct" one, which resulted in the lobby id string "mismatch". Visually the strings are the same, but one has length 3, and the other length 6 (with the added ZWSP)!

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

All methods require body string in the form of `<clientId> <rest of the message>`, for example `1 myLobby`. 

A websocket connection is established upon joining a lobby (either via `joinLobby` or `createLobby`).

### Websockets

Communication is based on a special packet consisting of three parts:

`TYPE` - `COMMAND` - `CONTENT`

- `TYPE` defines the type: `text` or `command` type packet. In special cases when the packet is unable to be parsed, it can be set to `corrupted`.
  - `text` packets are just text that the client can display, human readable
  - `command` packets control the flow and state of the game or connection
- `COMMAND` specifies the command in case of packet type `command`. It's a number, defined in an enum. Might change to string to ensure easier compatibility between different clients
- `CONTENT` is just string content. Based on the command, the content bears different levels of importantance. Some commands have empty (`""`) content, while others have encoded game states etc.

By using websockets with above messaging protocol, we control the whole flow of the game. The flow looks roughly like this **[THIS MAY BE OUTDATED]**:

- Wait for game start/input signal from server (`0`)
- Read player/client input (0-3) and send it to server
    - Player input can also be a *command*: `CMD:X`, where `X` is a number. Currently only command `555` is supported - it retrieves the current score
- If input was confirmed, server responds with `OK`, otherwise repeat
- Wait for input signal (first bullet point), but if the signal is `1`, end the game

Simulataneously, a goroutine is sending Ping signals every few seconds to check for potential disconnects. If a timeout is detected, connection gets closed. Pinging is not well supported in the `coder/websocket` library, so I used a custom command: `CMD:123`.

On server side, when one client disconnects, the lobby gets disbanded (all open connections to the lobby get closed).

In short: there are two kinds of messages: *normal* and *command* messages. *Command* messages are in the form of `CMD:X`, while all other messages are considered *normal*.

There may still be some situations where unexpected network interruptions break the clients (or server), but the most common cases should be handled by the current implementation.

#### Commands

Commands are defined in an *enum* (GO does not have native enums, os it's a close approximation). Look in the `messaging` module for a list of available commands.

## Future

✅The idea is to have a standalone custom game server that I can build any kind of client I want to. The next step (besides polishing the server) would be to make a client with some nice UI/graphics, such as Unity, which is already in development.

Next steps would be to implement a more complex game - which was *the real* goal from the start.