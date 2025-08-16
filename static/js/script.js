// --- Main View Elements ---
const gameSetup = document.getElementById("game-setup");
const waitingRoom = document.getElementById("waiting-room");
const gameContainer = document.getElementById("game-container");

// --- Buttons and Inputs ---
const gameIdInput = document.getElementById("game-id-input");
const joinGameBtn = document.getElementById("join-game-btn");
const createGameBtn = document.getElementById("create-game-btn");
const copyGameIdBtn = document.getElementById("copy-game-id-btn");

// --- Display Elements ---
const statusDiv = document.getElementById("status");
const cells = document.querySelectorAll(".cell");
const displayGameId = document.getElementById("display-game-id");
const displayGameIdWaiting = document.getElementById("display-game-id-waiting");
const displayPlayerSymbol = document.getElementById("display-player-symbol");
const scoreXDiv = document.getElementById("score-x");
const scoreODiv = document.getElementById("score-o");

// --- Modal Elements ---
const endGameModal = document.getElementById("end-game-modal");
const modalTitle = document.getElementById("modal-title");
const rematchBtn = document.getElementById("rematch-btn");
const newGameBtn = document.getElementById("new-game-btn");

let websocket;
let gameId;
let player;

// --- View Management ---
function showView(viewName) {
    gameSetup.classList.add("hidden");
    waitingRoom.classList.add("hidden");
    gameContainer.classList.add("hidden");
    const viewToShow = document.getElementById(viewName);
    if (viewToShow) {
        viewToShow.classList.remove("hidden");
    }
}

// --- Modal Management ---
function showEndGameModal(title) {
    modalTitle.textContent = title;
    endGameModal.classList.remove("hidden");
    rematchBtn.disabled = false;
    rematchBtn.textContent = "Request Rematch";
}

function hideEndGameModal() {
    endGameModal.classList.add("hidden");
}

// --- Event Listeners ---
createGameBtn.addEventListener("click", () => {
    gameId = Math.random().toString(36).substring(2, 8);
    displayGameIdWaiting.textContent = gameId;
    showView('waiting-room');
    connectWebSocket();
});

joinGameBtn.addEventListener("click", () => {
    gameId = gameIdInput.value.trim();
    if (gameId) {
        showView('waiting-room');
        connectWebSocket();
    } else {
        alert("Please enter a valid Game ID.");
    }
});

copyGameIdBtn.addEventListener("click", () => {
    navigator.clipboard.writeText(gameId).then(() => {
        copyGameIdBtn.textContent = 'Copied!';
        setTimeout(() => { copyGameIdBtn.textContent = 'Copy ID'; }, 2000);
    });
});

rematchBtn.addEventListener("click", () => {
    websocket.send(JSON.stringify({ event: "rematch_request" }));
    rematchBtn.textContent = "Waiting for Opponent...";
    rematchBtn.disabled = true;
});

newGameBtn.addEventListener("click", () => {
    location.reload();
});


// --- WebSocket Logic ---
function connectWebSocket() {
    websocket = new WebSocket(`ws://${window.location.host}/ws/${gameId}`);

    websocket.onopen = () => console.log("WebSocket connection established");

    websocket.onmessage = (event) => {
        const data = JSON.parse(event.data);

        if (data.error) {
            alert(data.error);
            showView('game-setup');
            return;
        }

        switch (data.event) {
            case "player_assignment":
                player = data.player;
                displayPlayerSymbol.textContent = player;
                break;
            case "start_game":
                updateScore(data.score);
                displayGameId.textContent = gameId;
                showView('game-container');
                updateTurnIndicator(data.current_player);
                statusDiv.textContent = `Game started! It's Player ${data.current_player}'s turn.`;
                break;
            case "move":
                updateBoard(data.board);
                updateTurnIndicator(data.current_player);
                statusDiv.textContent = (data.current_player === player) ? "It's your turn." : `It's Player ${data.current_player}'s turn.`;
                break;
            case "win":
                updateBoard(data.board);
                updateScore(data.score);
                disableBoard();
                showEndGameModal((data.player === player) ? "You Win!" : `Player ${data.player} Wins!`);
                break;
            case "draw":
                updateBoard(data.board);
                disableBoard();
                showEndGameModal("It's a Draw!");
                break;
            case "new_game":
                hideEndGameModal();
                resetBoard();
                updateBoard(data.board);
                updateScore(data.score);
                updateTurnIndicator(data.current_player);
                statusDiv.textContent = `Rematch! It's Player ${data.current_player}'s turn.`;
                break;
            case "opponent_left":
                statusDiv.textContent = "Your opponent has left the game.";
                disableBoard();
                hideEndGameModal();
                break;
        }
    };

    websocket.onclose = () => {
        console.log("WebSocket connection closed");
        if (!statusDiv.textContent.includes("left")) {
            statusDiv.textContent = "Connection lost. Please refresh.";
        }
        disableBoard();
        hideEndGameModal();
    };

    websocket.onerror = (error) => {
        console.error("WebSocket error:", error);
        statusDiv.textContent = "An error occurred. Please refresh the page.";
    };
}

// --- Game Board & UI Logic ---
cells.forEach(cell => {
    cell.addEventListener("click", () => {
        // Check if the cell is empty by seeing if it has a child span
        if (!cell.querySelector('span') && player && !cell.style.cursor.includes('not-allowed')) {
            const row = cell.dataset.row;
            const col = cell.dataset.col;
            websocket.send(JSON.stringify({ event: "make_move", row: parseInt(row), col: parseInt(col) }));
        }
    });
});

function updateBoard(board) {
    board.forEach((row, i) => {
        row.forEach((value, j) => {
            const cell = document.querySelector(`.cell[data-row='${i}'][data-col='${j}']`);
            
            // If there's a value but the cell is empty, create the span
            if (value && !cell.querySelector('span')) {
                const span = document.createElement('span');
                span.textContent = value;
                cell.appendChild(span);
            } 
            // If there's no value but the cell has a span, remove it
            else if (!value && cell.querySelector('span')) {
                cell.innerHTML = '';
            }

            // Update styling class
            cell.classList.remove('X', 'O');
            if (value) {
                cell.classList.add(value);
            }
        });
    });
}

function resetBoard() {
    cells.forEach(cell => {
        cell.innerHTML = ""; // This removes the inner span
        cell.style.cursor = 'pointer';
        cell.classList.remove('X', 'O');
    });
}

function disableBoard() {
    cells.forEach(cell => {
        cell.style.cursor = 'not-allowed';
    });
}

function updateScore(score) {
    scoreXDiv.textContent = `Player X: ${score.X}`;
    scoreODiv.textContent = `Player O: ${score.O}`;
}

function updateTurnIndicator(currentPlayer) {
    scoreXDiv.classList.remove('current-player');
    scoreODiv.classList.remove('current-player');
    if (currentPlayer === 'X') {
        scoreXDiv.classList.add('current-player');
    } else {
        scoreODiv.classList.add('current-player');
    }
}

// --- Initial State ---
document.addEventListener('DOMContentLoaded', () => {
    showView('game-setup');
});
