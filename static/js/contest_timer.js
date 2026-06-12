// ── Contest Timer ─────────────────────────────────────────────────────────────
// Shared by both challenges and tournaments.
// Manages the countdown, auto-triggers contest mode when time arrives,
// and fires callbacks on start/end.

var _contestTimer = null;

var ContestTimer = {
  // activeContest holds the current running contest state
  // { type: 'challenge'|'tournament', id, endsAt (Date), questionIDs, onStart, onEnd }
  active: null,

  // Start polling for a pending contest that hasn't begun yet.
  // scheduledAt: ISO string, questionIDs: array, callbacks: { onStart, onEnd }
  schedule: function(type, id, scheduledAt, questionIDs, callbacks) {
    ContestTimer.clear();
    var startsAt = new Date(scheduledAt);
    var endsAt   = new Date(startsAt.getTime() + 60 * 60 * 1000); // +1 hour

    ContestTimer.active = {
      type:        type,
      id:          id,
      startsAt:    startsAt,
      endsAt:      endsAt,
      questionIDs: questionIDs || [],
      onStart:     callbacks.onStart || function(){},
      onEnd:       callbacks.onEnd   || function(){}
    };

    ContestTimer._tick();
  },

  // Call this when the contest is already active (status === 'active')
  // and we just need the countdown + end detection.
  resume: function(type, id, scheduledAt, questionIDs, callbacks) {
    ContestTimer.clear();
    var startsAt = new Date(scheduledAt);
    var endsAt   = new Date(startsAt.getTime() + 60 * 60 * 1000);

    ContestTimer.active = {
      type:        type,
      id:          id,
      startsAt:    startsAt,
      endsAt:      endsAt,
      questionIDs: questionIDs || [],
      onStart:     callbacks.onStart || function(){},
      onEnd:       callbacks.onEnd   || function(){},
      started:     true   // already active, skip the start trigger
    };

    ContestTimer._tick();
  },

  clear: function() {
    if (_contestTimer) {
      clearInterval(_contestTimer);
      _contestTimer = null;
    }
    ContestTimer.active = null;
    ContestTimer._hideCountdown();
  },

  _tick: function() {
    // Run immediately then every second
    ContestTimer._evaluate();
    _contestTimer = setInterval(ContestTimer._evaluate, 1000);
  },

  _evaluate: function() {
    var ctx = ContestTimer.active;
    if (!ctx) return;

    var now = Date.now();

    // ── Before start ──────────────────────────────────────────────────────────
    if (!ctx.started) {
      var msToStart = ctx.startsAt.getTime() - now;
      if (msToStart > 0) {
        ContestTimer._renderCountdown('Starts in', msToStart, 'countdown-pre');
        return;
      }
      // Just crossed the start line
      ctx.started = true;
      ctx.onStart(ctx.questionIDs);
    }

    // ── During contest ────────────────────────────────────────────────────────
    var msLeft = ctx.endsAt.getTime() - now;
    if (msLeft > 0) {
      ContestTimer._renderCountdown('Time remaining', msLeft, 'countdown-active');
      return;
    }

    // ── Contest over ──────────────────────────────────────────────────────────
    ContestTimer._hideCountdown();
    clearInterval(_contestTimer);
    _contestTimer = null;
    var endCtx = ctx;
    ContestTimer.active = null;
    endCtx.onEnd();
  },

  _renderCountdown: function(label, ms, cssClass) {
    var el = document.getElementById('contest-countdown');
    if (!el) return;

    var totalSec = Math.floor(ms / 1000);
    var h = Math.floor(totalSec / 3600);
    var m = Math.floor((totalSec % 3600) / 60);
    var s = totalSec % 60;

    var hh = h > 0 ? h + ':' : '';
    var mm = (h > 0 ? String(m).padStart(2,'0') : m) + ':';
    var ss = String(s).padStart(2, '0');

    el.style.display = 'flex';
    el.className = 'contest-countdown ' + cssClass;
    el.innerHTML =
      '<span class="countdown-label">' + escHtml(label) + '</span>' +
      '<span class="countdown-time">' + hh + mm + ss + '</span>';

    // Turn red in last 5 minutes
    if (ms < 5 * 60 * 1000) {
      el.classList.add('countdown-danger');
    }
  },

  _hideCountdown: function() {
    var el = document.getElementById('contest-countdown');
    if (el) el.style.display = 'none';
  }
};


// ── Notification poller ───────────────────────────────────────────────────────
// Polls /api/notifications every 15 seconds and dispatches events.

var _notifPollTimer = null;
var _lastNotifID    = 0;

function startNotificationPolling() {
  if (_notifPollTimer) return;
  pollNotifications(); // immediate first check
  _notifPollTimer = setInterval(pollNotifications, 15000);
}

function stopNotificationPolling() {
  if (_notifPollTimer) {
    clearInterval(_notifPollTimer);
    _notifPollTimer = null;
  }
}

async function pollNotifications() {
  try {
    var res = await apiFetch('/api/contest-notifications?since=' + _lastNotifID);
    if (!res.ok) return;
    var notifs = await res.json();
    if (!notifs || notifs.length === 0) return;

    notifs.forEach(function(n) {
      if (n.id > _lastNotifID) _lastNotifID = n.id;
      dispatchNotification(n);
    });
  } catch(e) {}
}

function dispatchNotification(n) {
  var payload = {};
  try { payload = JSON.parse(n.payload); } catch(e) {}

  switch (n.kind) {
    case 'challenge_received':
      onChallengeReceived(payload);
      break;
    case 'challenge_accepted':
      onChallengeAccepted(payload);
      break;
    case 'challenge_rejected':
      onChallengeRejected(payload);
      break;
    case 'challenge_needs_questions':
    case 'challenge_pending_questions':
      onChallengeNeedsQuestions(payload);
      break;
    case 'contest_start':
      onContestStart(payload);
      break;
    case 'contest_end':
      onContestEnd(payload);
      break;
    case 'tournament_start':
      onTournamentStart(payload);
      break;
    case 'tournament_end':
      onTournamentEnd(payload);
      break;
  }
}

// Stubs — overridden by challenges.js / tournaments.js
function onChallengeReceived(p)      { showNotifBadge(); }
function onChallengeAccepted(p)      { showNotifBadge(); }
function onChallengeRejected(p)      { showNotifBadge(); }
function onChallengeNeedsQuestions(p){ showNotifBadge(); }
function onContestStart(p)           {}
function onContestEnd(p)             {}
function onTournamentStart(p)        {}
function onTournamentEnd(p)          {}

function showNotifBadge() {
  var b = document.getElementById('notif-badge');
  if (b) { b.style.display = 'inline-block'; }
}
function hideNotifBadge() {
  var b = document.getElementById('notif-badge');
  if (b) { b.style.display = 'none'; }
}