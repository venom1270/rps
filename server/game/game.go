package game

import (
	"log"
	"math/rand"
	"strconv"
)

type PlayerChoice int

const (
	ROCK     PlayerChoice = iota // 0
	PAPER                        // 1
	SCISSORS                     // 2
	JOKER                        // 3
)

type GameState int

const (
	WAITING GameState = iota
	ROUND_FINISHED
	GAME_FINISHED
)

type Game struct {
	players      []([]PlayerChoice)
	scores       []int
	toWin        int
	numChoices   int
	currentRound int
	state        GameState
}

func NewGame() *Game {
	return &Game{
		toWin:        3,
		state:        WAITING,
		scores:       []int{0, 0},
		currentRound: 0,
		numChoices:   0,
		players:      [][]PlayerChoice{[]PlayerChoice{}, []PlayerChoice{}},
	}
}

func (g *Game) MakeChoice(player int, choice PlayerChoice) (bool, bool) {
	if player < 0 || player > 1 {
		// Invalid player
		return false, false
	}

	if len(g.players[player]) > g.currentRound {
		// Already played this round
		return false, false
	}

	g.players[player] = append(g.players[player], choice)
	g.numChoices++

	roundFinished := false

	if g.numChoices == len(g.players) {
		// All players made their choices - calculate results and start new round if needed
		//g.completeRound()
		g.state = ROUND_FINISHED
		roundFinished = true
	}

	return true, roundFinished
}

func (g *Game) CompleteRound() int {
	p1 := g.players[0][g.currentRound]
	p2 := g.players[1][g.currentRound]

	if p1 == p2 && p1 != JOKER {
		// Stalemate
		g.currentRound++
		return -1
	}

	winner := -1

	switch p1 {
	case ROCK:
		if p2 == SCISSORS {
			winner = 0
		} else if p2 == PAPER {
			winner = 1
		} else if p2 == JOKER {
			winner = 1
		}
	case PAPER:
		if p2 == SCISSORS {
			winner = 1
		} else if p2 == ROCK {
			winner = 0
		} else if p2 == JOKER {
			winner = 1
		}
	case SCISSORS:
		if p2 == ROCK {
			winner = 1
		} else if p2 == PAPER {
			winner = 0
		} else if p2 == JOKER {
			winner = 0
			log.Print("Player 1 loses 1 point for losing with the JOKER!")
			g.addScore(1, -1)
		}
	case JOKER:
		if p2 == ROCK {
			winner = 0
		} else if p2 == PAPER {
			winner = 0
		} else if p2 == SCISSORS {
			winner = 1
			log.Print("Player 0 loses 1 point for losing with the JOKER!")
			g.addScore(0, -1)
		} else {
			// 50-50 chance for each to win
			randomFloat := rand.Float64()
			if randomFloat >= 0.5 {
				winner = 1
				log.Print("BOTH PLAYERS USED JOKER! Player 0 loses 1 point for losing with the JOKER!")
				g.addScore(0, -1)
			} else {
				winner = 0
				log.Print("BOTH PLAYERS USED JOKER! Player 1 loses 1 point for losing with the JOKER!")
				g.addScore(1, -1)
			}
		}
	}

	if winner != -1 {
		g.scores[winner]++
		if g.scores[winner] >= g.toWin {
			g.state = GAME_FINISHED
			return winner
		}
	}

	g.currentRound++
	g.state = WAITING
	g.numChoices = 0
	return winner
}

func (g *Game) IsFinished() bool {
	return g.state == GAME_FINISHED
}

func (g *Game) IsRoundFinished() bool {
	return g.state == ROUND_FINISHED
}

func (g *Game) GetWinner() int {
	maxI := 0
	maxScore := 0
	for i, s := range g.scores {
		if s > maxScore {
			maxI = i
			maxScore = s
		}
	}
	return maxI
}

func (g *Game) GetScores() []int {
	return g.scores
}

func (g *Game) addScore(player, points int) {
	if player < len(g.players) {
		g.scores[player] += points
		if g.scores[player] < 0 {
			g.scores[player] = 0
		}
	}
}

func (g *Game) GetGameDetails(clientIds []string) string {
	str := ""
	for i, v := range g.players {
		str += clientIds[i] + ":[" + strconv.Itoa(g.scores[i]) + ","
		for _, vv := range v {
			str += strconv.Itoa(int(vv)) + ","
		}
		if str[len(str)-1] == ',' {
			str = str[:len(str)-1]
		}
		str += "];"
	}
	if len(str) > 0 {
		str = str[:len(str)-1]
	}
	return str
}
