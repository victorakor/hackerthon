// ── Contest Timer ─────────────────────────────────────────────────────────────

var _contestTimer = null;

var ContestTimer = {
  active: null,

  // For contests not yet started — pass scheduledAt ISO string and durationMinutes.
  schedule: function(type, id, scheduledAt, durationMinutes, callbacks) {
    ContestTimer.clear();
    var startsAt = new Date(scheduledAt);
    var endsAt   = new Date(startsAt.getTime() + durationMinutes * 60 * 1000);

    ContestTimer.active = {
      type:        type,
      id:          id,
      startsAt:    startsAt,
      endsAt:      endsAt,
      onStart:     callbacks.onStart || function(){},
      onEnd:       callbacks.onEnd   || function(){}
    };

    ContestTimer._tick();
  },

  // For already-active contests — pass endsAt ISO string directly from server.
  resume: function(type, id, endsAt, callbacks) {
    ContestTimer.clear();
    var endsAtDate = new Date(endsAt);

    ContestTimer.active = {
      type:    type,
      id:      id,
      endsAt:  endsAtDate,
      onStart: callbacks.onStart || function(){},
      onEnd:   callbacks.onEnd   || function(){},
      started: true
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
    ContestTimer._evaluate();
    _contestTimer = setInterval(ContestTimer._evaluate, 1000);
  },

  _evaluate: function() {
    var ctx = ContestTimer.active;
    if (!ctx) return;

    var now = Date.now();

    if (!ctx.started) {
      var msToStart = ctx.startsAt.getTime() - now;
      if (msToStart > 0) {
        ContestTimer._renderCountdown('Starts in', msToStart, 'countdown-pre');
        return;
      }
      ctx.started = true;
      ctx.onStart();
    }

    var msLeft = ctx.endsAt.getTime() - now;
    if (msLeft > 0) {
      ContestTimer._renderCountdown('Time remaining', msLeft, 'countdown-active');
      return;
    }

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

var _notifPollTimer = null;
var _lastNotifID    = 0;

function _notifIDKey() {
  return 'lastNotifID_' + (currentUser ? currentUser.id : 'anon');
}

function startNotificationPolling() {
  if (_notifPollTimer) return;
  var saved = parseInt(localStorage.getItem(_notifIDKey()) || '0', 10);
  if (saved > _lastNotifID) _lastNotifID = saved;
  pollNotifications();
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
      if (n.id > _lastNotifID) {
        _lastNotifID = n.id;
        localStorage.setItem(_notifIDKey(), _lastNotifID);
      }
      dispatchNotification(n);
    });
  } catch(e) {}
}

function dispatchNotification(n) {
  var payload = {};
  try { payload = JSON.parse(n.payload); } catch(e) {}

  switch (n.kind) {
    case 'challenge_received':        onChallengeReceived(payload);       break;
    case 'challenge_accepted':        onChallengeAccepted(payload);       break;
    case 'challenge_rejected':        onChallengeRejected(payload);       break;
    case 'challenge_needs_questions':
    case 'challenge_pending_questions': onChallengeNeedsQuestions(payload); break;
    case 'contest_start':             onContestStart(payload);            break;
    case 'contest_end':               onContestEnd(payload);              break;
    case 'tournament_start':          onTournamentStart(payload);         break;
    case 'tournament_end':            onTournamentEnd(payload);           break;
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