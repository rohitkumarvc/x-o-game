from fastapi import FastAPI, WebSocket, Request, WebSocketDisconnect
from fastapi.responses import HTMLResponse
from fastapi.staticfiles import StaticFiles
from fastapi.templating import Jinja2Templates
import asyncio

app = FastAPI()

app.mount("/static", StaticFiles(directory="static"), name="static")
templates = Jinja2Templates(directory="templates")

games = {}

def reset_game_board(game_id, starter="X"):
    """Resets the board for a new round, setting the specified starter."""
    if game_id in games:
        games[game_id]["board"] = [["" for _ in range(3)] for _ in range(3)]
        games[game_id]["current_player"] = starter  # Set the new starting player
        games[game_id]["rematch_requests"] = set()


@app.get("/", response_class=HTMLResponse)
async def read_root(request: Request):
    return templates.TemplateResponse("index.html", {"request": request})

async def broadcast(game_id: str, message: dict):
    if game_id in games:
        for player in games[game_id]["players"]:
            await player["ws"].send_json(message)

@app.websocket("/ws/{game_id}")
async def websocket_endpoint(websocket: WebSocket, game_id: str):
    await websocket.accept()
    
    if game_id not in games:
        games[game_id] = {
            "board": [["" for _ in range(3)] for _ in range(3)],
            "players": [],
            "current_player": "X",
            "score": {"X": 0, "O": 0},
            "rematch_requests": set(),
            "starting_player_for_round": "X"  # Track who starts the round
        }
    
    game = games[game_id]
    
    if len(game["players"]) >= 2:
        await websocket.send_json({"error": "Game is full"})
        await websocket.close()
        return

    player_symbol = "X" if len(game["players"]) == 0 else "O"
    game["players"].append({"symbol": player_symbol, "ws": websocket})
    
    await websocket.send_json({"event": "player_assignment", "player": player_symbol})

    if len(game["players"]) == 2:
        await broadcast(game_id, {"event": "start_game", "current_player": game["current_player"], "score": game["score"]})

    try:
        while True:
            data = await websocket.receive_json()
            event = data.get("event")

            if event == "make_move":
                if game["current_player"] == player_symbol and len(game["players"]) == 2:
                    row, col = data["row"], data["col"]
                    if game["board"][row][col] == "":
                        game["board"][row][col] = player_symbol
                        
                        if check_win(game["board"], player_symbol):
                            game["score"][player_symbol] += 1
                            await broadcast(game_id, {"event": "win", "player": player_symbol, "board": game["board"], "score": game["score"]})
                        elif check_draw(game["board"]):
                            await broadcast(game_id, {"event": "draw", "board": game["board"]})
                        else:
                            game["current_player"] = "O" if player_symbol == "X" else "X"
                            await broadcast(game_id, {"event": "move", "board": game["board"], "current_player": game["current_player"]})
            
            elif event == "rematch_request":
                game["rematch_requests"].add(player_symbol)
                if len(game["rematch_requests"]) == 2:
                    # --- The Implemented Alternating Logic ---
                    current_starter = game["starting_player_for_round"]
                    next_starter = "O" if current_starter == "X" else "X"
                    
                    # Update who will start the *next* round
                    game["starting_player_for_round"] = next_starter
                    
                    # Reset the board, passing in the new starter
                    reset_game_board(game_id, starter=next_starter)
                    
                    await broadcast(game_id, {"event": "new_game", "board": game["board"], "current_player": game["current_player"], "score": game["score"]})

    except WebSocketDisconnect:
        player_to_remove = next((p for p in game["players"] if p["ws"] == websocket), None)
        if player_to_remove:
            game["players"].remove(player_to_remove)
            if game["players"]:
                await broadcast(game_id, {"event": "opponent_left"})

        if not game["players"] and game_id in games:
            del games[game_id]

def check_win(board, player):
    for i in range(3):
        if all(board[i][j] == player for j in range(3)) or all(board[j][i] == player for j in range(3)):
            return True
    if all(board[i][i] == player for i in range(3)) or all(board[i][2 - i] == player for i in range(3)):
        return True
    return False

def check_draw(board):
    return all(cell != "" for row in board for cell in row)
