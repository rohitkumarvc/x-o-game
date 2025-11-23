package main

import (
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

// --- Structs & Types ---

type Score struct {
	X int `json:"X"`
	O int `json:"O"`
}

type Player struct {
	Symbol string          `json:"symbol"`
	Conn   *websocket.Conn `json:"-"` // Ignore in JSON
}

type Game struct {
	ID                     string
	Board                  [3][3]string
	Players                []*Player
	CurrentPlayer          string
	Score                  Score
	RematchRequests        map[string]bool // Using map as set
	StartingPlayerForRound string
	Mutex                  sync.Mutex // To make the game thread-safe
}

type InboundMessage struct {
	Event string `json:"event"`
	Row   int    `json:"row"`
	Col   int    `json:"col"`
}

type OutboundMessage struct {
	Event         string       `json:"event"`
	Player        string       `json:"player,omitempty"`
	Board         [3][3]string `json:"board,omitempty"`
	CurrentPlayer string       `json:"current_player,omitempty"`
	Score         *Score       `json:"score,omitempty"`
	Error         string       `json:"error,omitempty"`
}

// --- Global State ---

var (
	games      = make(map[string]*Game)
	gamesMutex sync.RWMutex // Lock for the games map
	upgrader   = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins (like FastAPI default)
		},
	}
	templates = template.Must(template.ParseGlob("templates/*.html"))
)

// --- Game Logic Helpers ---

func resetGameBoard(game *Game, starter string) {
	game.Board = [3][3]string{
		{"", "", ""},
		{"", "", ""},
		{"", "", ""},
	}
	game.CurrentPlayer = starter
	game.RematchRequests = make(map[string]bool)
}

func checkWin(board [3][3]string, player string) bool {
	// Check rows and cols
	for i := 0; i < 3; i++ {
		if (board[i][0] == player && board[i][1] == player && board[i][2] == player) ||
			(board[0][i] == player && board[1][i] == player && board[2][i] == player) {
			return true
		}
	}
	// Check diagonals
	if (board[0][0] == player && board[1][1] == player && board[2][2] == player) ||
		(board[0][2] == player && board[1][1] == player && board[2][0] == player) {
		return true
	}
	return false
}

func checkDraw(board [3][3]string) bool {
	for _, row := range board {
		for _, cell := range row {
			if cell == "" {
				return false
			}
		}
	}
	return true
}

func broadcast(game *Game, msg OutboundMessage) {
	for _, p := range game.Players {
		// In production, you might want a write lock on the connection
		// or use a channel to prevent concurrent writes to the same socket.
		err := p.Conn.WriteJSON(msg)
		if err != nil {
			log.Printf("Error broadcasting to player %s: %v", p.Symbol, err)
		}
	}
}

// --- HTTP Handlers ---

func readRoot(w http.ResponseWriter, r *http.Request) {
	templates.ExecuteTemplate(w, "index.html", nil)
}

func keepJobAlive(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "Job is alive"})
}

// --- WebSocket Handler ---

func websocketHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	gameID := vars["game_id"]

	// Upgrade HTTP to WebSocket
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Upgrade error:", err)
		return
	}

	// Lock Global Map to find or create game
	gamesMutex.Lock()
	game, exists := games[gameID]
	if !exists {
		game = &Game{
			ID:                     gameID,
			Board:                  [3][3]string{{"", "", ""}, {"", "", ""}, {"", "", ""}},
			Players:                make([]*Player, 0),
			CurrentPlayer:          "X",
			Score:                  Score{X: 0, O: 0},
			RematchRequests:        make(map[string]bool),
			StartingPlayerForRound: "X",
		}
		games[gameID] = game
	}
	gamesMutex.Unlock()

	// Lock Game specific logic
	game.Mutex.Lock()

	if len(game.Players) >= 2 {
		ws.WriteJSON(OutboundMessage{Error: "Game is full"})
		ws.Close()
		game.Mutex.Unlock()
		return
	}

	playerSymbol := "X"
	if len(game.Players) > 0 {
		playerSymbol = "O"
	}

	newPlayer := &Player{Symbol: playerSymbol, Conn: ws}
	game.Players = append(game.Players, newPlayer)

	// Send assignment
	ws.WriteJSON(OutboundMessage{Event: "player_assignment", Player: playerSymbol})

	// Start game if full
	if len(game.Players) == 2 {
		broadcast(game, OutboundMessage{
			Event:         "start_game",
			CurrentPlayer: game.CurrentPlayer,
			Score:         &game.Score,
		})
	}
	game.Mutex.Unlock()

	// Cleanup function for when socket closes
	defer func() {
		game.Mutex.Lock()
		// Find and remove player
		for i, p := range game.Players {
			if p.Conn == ws {
				game.Players = append(game.Players[:i], game.Players[i+1:]...)
				break
			}
		}
		
		if len(game.Players) > 0 {
			broadcast(game, OutboundMessage{Event: "opponent_left"})
		} else {
			// Remove game from global map if empty
			gamesMutex.Lock()
			delete(games, gameID)
			gamesMutex.Unlock()
		}
		game.Mutex.Unlock()
		ws.Close()
	}()

	// Read Loop
	for {
		var msg InboundMessage
		err := ws.ReadJSON(&msg)
		if err != nil {
			// WebSocketDisconnect equivalent
			break
		}

		game.Mutex.Lock() // Lock for state mutation

		if msg.Event == "make_move" {
			if game.CurrentPlayer == playerSymbol && len(game.Players) == 2 {
				row, col := msg.Row, msg.Col
				
				// Validate move
				if row >= 0 && row < 3 && col >= 0 && col < 3 && game.Board[row][col] == "" {
					game.Board[row][col] = playerSymbol

					if checkWin(game.Board, playerSymbol) {
						if playerSymbol == "X" {
							game.Score.X++
						} else {
							game.Score.O++
						}
						broadcast(game, OutboundMessage{
							Event:  "win",
							Player: playerSymbol,
							Board:  game.Board,
							Score:  &game.Score,
						})
					} else if checkDraw(game.Board) {
						broadcast(game, OutboundMessage{
							Event: "draw",
							Board: game.Board,
						})
					} else {
						// Switch Turn
						if playerSymbol == "X" {
							game.CurrentPlayer = "O"
						} else {
							game.CurrentPlayer = "X"
						}
						broadcast(game, OutboundMessage{
							Event:         "move",
							Board:         game.Board,
							CurrentPlayer: game.CurrentPlayer,
						})
					}
				}
			}
		} else if msg.Event == "rematch_request" {
			game.RematchRequests[playerSymbol] = true
			
			if len(game.RematchRequests) == 2 {
				// --- Alternating Logic ---
				currentStarter := game.StartingPlayerForRound
				nextStarter := "X"
				if currentStarter == "X" {
					nextStarter = "O"
				}

				game.StartingPlayerForRound = nextStarter
				resetGameBoard(game, nextStarter)

				broadcast(game, OutboundMessage{
					Event:         "new_game",
					Board:         game.Board,
					CurrentPlayer: game.CurrentPlayer,
					Score:         &game.Score,
				})
			}
		}

		game.Mutex.Unlock()
	}
}

func main() {
	r := mux.NewRouter()

	// Static Files
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// Routes
	r.HandleFunc("/", readRoot).Methods("GET")
	r.HandleFunc("/keep_job_alive", keepJobAlive).Methods("GET")
	r.HandleFunc("/ws/{game_id}", websocketHandler)

	log.Println("Server starting on :8000")
	log.Fatal(http.ListenAndServe(":8000", r))
}
