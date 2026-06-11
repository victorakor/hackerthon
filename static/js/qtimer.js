function startQuestionTick() {
  if (questionTickInterval) return;
  questionTickInterval = setInterval(function() {
    Object.keys(questionTimers).forEach(function(qidStr) {
      var qid = parseInt(qidStr);
      if (questionTimers[qid] > 0) {
        questionTimers[qid]--;
        updateQTimerDisplay(qid);
        if (questionTimers[qid] === 0) expireQuestion(qid);
      }
    });
  }, 1000);
}

function stopQuestionTick() {
  clearInterval(questionTickInterval);
  questionTickInterval = null;
}

function expireQuestion(qid) {
  var el = document.querySelector('.q-item[data-id="' + qid + '"]');
  if (el) {
    el.style.transition = 'opacity 0.8s';
    el.style.opacity = '0';
    setTimeout(function() { el.remove(); updateProgress(); }, 800);
  }
  if (currentQuestion && currentQuestion.id === qid) {
    document.getElementById('detail-pane').innerHTML =
      '<div class="empty-state"><div class="empty-state-icon">&#x23F1;</div>' +
      '<div class="empty-state-text">Time\'s up for this question!</div></div>';
    currentQuestion = null;
  }
}

function updateQTimerDisplay(qid) {
  var el = document.querySelector('.q-timer[data-id="' + qid + '"]');
  if (!el) return;
  var s = questionTimers[qid] || 0;
  var m = Math.floor(s / 60);
  var sec = s % 60;
  el.textContent = String(m).padStart(2, '0') + ':' + String(sec).padStart(2, '0') + ' left';
  el.className = 'q-timer' + (s < 120 ? ' expiring' : '');
}

function qTimerStr(qid) {
  var s = questionTimers[qid] !== undefined ? questionTimers[qid] : 0;
  var m = Math.floor(s / 60);
  var sec = s % 60;
  return String(m).padStart(2, '0') + ':' + String(sec).padStart(2, '0') + ' left';
}

function assignQuestionTimers() {
  var visible = questions.filter(function(q) { return q.visible; });
  var easyTime = 12 * 60, medTime = 18 * 60, hardTime = 25 * 60;
  var total = 0;
  visible.forEach(function(q) {
    total += q.difficulty === 'easy' ? easyTime : q.difficulty === 'medium' ? medTime : hardTime;
  });
  var scale = total > 0 ? (5 * 3600) / total : 1;
  visible.forEach(function(q) {
    if (questionTimers[q.id] === undefined) {
      var base = q.difficulty === 'easy' ? easyTime : q.difficulty === 'medium' ? medTime : hardTime;
      questionTimers[q.id] = Math.round(base * scale);
    }
  });
  if (timerRunning) startQuestionTick();
}

function reassignQuestionTimers() {
  stopQuestionTick();
  questionTimers = {};
  var visible = questions.filter(function(q) { return q.visible; });
  var easyTime = 12 * 60, medTime = 18 * 60, hardTime = 25 * 60;
  var total = 0;
  visible.forEach(function(q) {
    total += q.difficulty === 'easy' ? easyTime : q.difficulty === 'medium' ? medTime : hardTime;
  });
  var scale = total > 0 ? (5 * 3600) / total : 1;
  visible.forEach(function(q) {
    var base = q.difficulty === 'easy' ? easyTime : q.difficulty === 'medium' ? medTime : hardTime;
    questionTimers[q.id] = Math.round(base * scale);
  });
  renderQuestionList();
  if (timerRunning) startQuestionTick();
}