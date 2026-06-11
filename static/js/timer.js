function startTimer() {
  if (timerRunning) return;
  timerRunning = true;
  document.getElementById('timer-toggle-btn').textContent = '\u25AE\u25AE Pause';
  timerInterval = setInterval(function() {
    if (timerSeconds > 0) {
      timerSeconds--;
      updateTimerDisplay();
    } else {
      clearInterval(timerInterval);
      timerRunning = false;
    }
  }, 1000);
  startQuestionTick();
}

function stopTimer() {
  clearInterval(timerInterval);
  timerRunning = false;
  document.getElementById('timer-toggle-btn').textContent = '\u25BA Resume';
  stopQuestionTick();
}

function toggleTimer() {
  if (timerRunning) { stopTimer(); } else { startTimer(); }
}

function resetTimer() {
  stopTimer();
  timerSeconds = 5 * 60 * 60;
  updateTimerDisplay();
  reassignQuestionTimers();
}

function stopAllTimers() {
  clearInterval(timerInterval);
  stopQuestionTick();
  timerRunning = false;
}

function updateTimerDisplay() {
  var h = Math.floor(timerSeconds / 3600);
  var m = Math.floor((timerSeconds % 3600) / 60);
  var s = timerSeconds % 60;
  var display = document.getElementById('timer-display');
  display.textContent =
    String(h).padStart(2, '0') + ':' +
    String(m).padStart(2, '0') + ':' +
    String(s).padStart(2, '0');
  display.className = 'timer-display' +
    (timerSeconds < 600 ? ' critical' : timerSeconds < 1800 ? ' warning' : '');
}