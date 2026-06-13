// ── Raids Panel ──────────────────────────────────────────────────────────────

var _raidsState = {
  clanID: null,
  myRole: null,
  pollHandle: null
};

function initRaidsPanel(clanID, myRole) {
  _raidsState.clanID = clanID;
  _raidsState.myRole = myRole;
  stopRaidsPoll();

  var mount = document.getElementById('raids-mount');
  if (!mount) return;

  var canCreate = (myRole === 'clanhead' || myRole === 'general');

  mount.innerHTML = `
    <div class="raids-panel">
      <div class="raids-header">
        <h3>⚔️ Raids</h3>
        ${canCreate ? '<button class="btn-clan-create" onclick="showCreateRaidModal()">+ Set Raid</button>' : ''}
      </div>
      <div id="raids-list-area">
        <div class="clan-loading">Loading raids…</div>
      </div>
    </div>
    ${canCreate ? createRaidModalHTML() : ''}
  `;

  loadRaidsList();
  _raidsState.pollHandle = setInterval(loadRaidsList, 10000);
}

function stopRaidsPoll() {
  if (_raidsState.pollHandle) {
    clearInterval(_raidsState.pollHandle);
    _raidsState.pollHandle = null;
  }
}

function loadRaidsList() {
  apiFetch('/api/raids')
    .then(function(r) { return r.json(); })
    .then(function(raids) {
      var area = document.getElementById('raids-list-area');
      if (!area) return;
      if (!raids || raids.length === 0) {
        area.innerHTML = '<div class="clan-empty">No raids scheduled. Clanheads/Generals can set one!</div>';
        return;
      }
      area.innerHTML = raids.map(renderRaidCard).join('');
      raids.forEach(function(raid) {
        if (raid.status === 'upcoming') startRaidCountdown(raid.id, raid.scheduled_at);
        if (raid.status === 'active' && raid.ends_at) startRaidCountdown(raid.id, raid.ends_at, true);
      });
    })
    .catch(function() {
      var area = document.getElementById('raids-list-area');
      if (area) area.innerHTML = '<div class="clan-empty">Failed to load raids.</div>';
    });
}

function renderRaidCard(raid) {
  var statusBadge = {
    upcoming: '<span class="raid-status raid-upcoming">⏳ Upcoming</span>',
    active: '<span class="raid-status raid-active">🔥 Active</span>',
    completed: '<span class="raid-status raid-completed">✅ Completed</span>'
  }[raid.status] || '';

  var myClanID = window._myClan ? window._myClan.id : null;
  var inRaid = raid.clans.some(function(c) { return c.clan_id === myClanID; });

  var clansHtml = raid.clans.map(function(c) {
    var isMine = c.clan_id === myClanID ? ' raid-clan-mine' : '';
    var rankHtml = raid.status !== 'upcoming' ? `<span class="raid-clan-rank">#${c.rank}</span>` : '';
    return `
      <div class="raid-clan-row${isMine}">
        ${rankHtml}
        <span class="raid-clan-name">${escHtml(c.clan_name)}</span>
        <span class="raid-clan-score">${raid.status !== 'upcoming' ? c.score + ' pts' : ''}</span>
      </div>
    `;
  }).join('');

  var actionHtml = '';
  if (raid.status === 'active' && inRaid) {
    actionHtml = `<button class="btn-clan-join" onclick="enterRaidArena(${raid.id})">Enter Arena</button>`;
  } else if (raid.status === 'upcoming') {
    actionHtml = `<div class="raid-countdown" id="raid-countdown-${raid.id}">Starts in —</div>`;
  } else if (raid.status === 'active') {
    actionHtml = `<div class="raid-countdown" id="raid-countdown-${raid.id}">Ends in —</div>`;
  }

  return `
    <div class="raid-card">
      <div class="raid-card-header">
        <span class="raid-initiator">${escHtml(raid.initiating_clan_name)}</span> set a raid
        ${statusBadge}
      </div>
      <div class="raid-card-meta">
        <span>📅 ${formatDateTime(raid.scheduled_at)}</span>
        ${raid.question_count ? `<span>📝 ${raid.question_count} questions</span>` : ''}
        ${raid.duration_minutes ? `<span>⏱ ${raid.duration_minutes} min</span>` : ''}
      </div>
      <div class="raid-clans-list">${clansHtml}</div>
      <div class="raid-card-actions">${actionHtml}</div>
    </div>
  `;
}

function startRaidCountdown(raidID, targetISO, isEndCountdown) {
  var el = document.getElementById('raid-countdown-' + raidID);
  if (!el) return;
  var target = new Date(targetISO).getTime();

  function tick() {
    el = document.getElementById('raid-countdown-' + raidID);
    if (!el) return;
    var diff = target - Date.now();
    if (diff <= 0) {
      el.textContent = isEndCountdown ? 'Ending…' : 'Starting…';
      loadRaidsList();
      return;
    }
    var mins = Math.floor(diff / 60000);
    var secs = Math.floor((diff % 60000) / 1000);
    el.textContent = (isEndCountdown ? 'Ends in ' : 'Starts in ') + mins + 'm ' + secs + 's';
    setTimeout(tick, 1000);
  }
  tick();
}

function formatDateTime(iso) {
  if (!iso) return '';
  var d = new Date(iso);
  return d.toLocaleString();
}

// ── Create Raid Modal ─────────────────────────────────────────────────────────

function createRaidModalHTML() {
  return `
    <div id="create-raid-modal" class="clan-modal-overlay" style="display:none">
      <div class="clan-modal">
        <div class="clan-modal-header">
          <h3>Set a Raid</h3>
          <button class="clan-modal-close" onclick="closeCreateRaidModal()">✕</button>
        </div>
        <div class="clan-form">
          <label>Scheduled Time
            <input id="raid-time-input" type="datetime-local">
          </label>
          <label>Target Clans (select one or more)
            <div id="raid-target-clans" class="raid-target-clans">
              <div class="clan-loading">Loading clans…</div>
            </div>
          </label>
          <div id="raid-create-error" class="clan-form-error" style="display:none"></div>
          <p class="raid-form-note">⚠️ Raids cannot be cancelled or rejected once set. Question count and duration are calculated automatically based on total participants.</p>
          <button class="btn-clan-create" onclick="submitCreateRaid()">Set Raid</button>
        </div>
      </div>
    </div>
  `;
}

function showCreateRaidModal() {
  document.getElementById('create-raid-modal').style.display = 'flex';
  loadRaidTargetClans();
}

function closeCreateRaidModal() {
  document.getElementById('create-raid-modal').style.display = 'none';
  document.getElementById('raid-create-error').style.display = 'none';
}

function loadRaidTargetClans() {
  apiFetch('/api/clans')
    .then(function(r) { return r.json(); })
    .then(function(clans) {
      var area = document.getElementById('raid-target-clans');
      if (!area) return;
      var myClanID = window._myClan ? window._myClan.id : null;
      var others = clans.filter(function(c) { return c.id !== myClanID; });
      if (others.length === 0) {
        area.innerHTML = '<div class="clan-empty">No other clans to raid yet.</div>';
        return;
      }
      area.innerHTML = others.map(function(c) {
        return `
          <label class="raid-target-clan-option">
            <input type="checkbox" value="${c.id}">
            <span class="clan-tag">[${escHtml(c.tag)}]</span> ${escHtml(c.name)}
            <span class="raid-target-rating">⭐ ${c.rating}</span>
          </label>
        `;
      }).join('');
    })
    .catch(function() {
      var area = document.getElementById('raid-target-clans');
      if (area) area.innerHTML = '<div class="clan-empty">Failed to load clans.</div>';
    });
}

function submitCreateRaid() {
  var timeInput = document.getElementById('raid-time-input').value;
  var errEl = document.getElementById('raid-create-error');

  if (!timeInput) {
    errEl.textContent = 'Please select a scheduled time.';
    errEl.style.display = '';
    return;
  }

  var scheduledAt = new Date(timeInput);
  if (isNaN(scheduledAt.getTime())) {
    errEl.textContent = 'Invalid date/time.';
    errEl.style.display = '';
    return;
  }
  if (scheduledAt.getTime() < Date.now() + 5 * 60000) {
    errEl.textContent = 'Scheduled time must be at least 5 minutes from now.';
    errEl.style.display = '';
    return;
  }

  var checked = document.querySelectorAll('#raid-target-clans input[type="checkbox"]:checked');
  var targetClanIDs = Array.prototype.map.call(checked, function(cb) { return parseInt(cb.value, 10); });

  if (targetClanIDs.length === 0) {
    errEl.textContent = 'Select at least one clan to raid.';
    errEl.style.display = '';
    return;
  }

  apiFetch('/api/raids', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      scheduled_at: scheduledAt.toISOString(),
      target_clan_ids: targetClanIDs
    })
  })
    .then(function(r) {
      if (!r.ok) return r.text().then(function(t) { throw new Error(t); });
      return r.json();
    })
    .then(function() {
      closeCreateRaidModal();
      showToast('Raid scheduled!', 'success');
      loadRaidsList();
    })
    .catch(function(e) {
      errEl.textContent = e.message || 'Failed to create raid.';
      errEl.style.display = '';
    });
}

// ── Enter Arena ───────────────────────────────────────────────────────────────

function enterRaidArena(raidID) {
  if (typeof initRaidArena === 'function') {
    initRaidArena(raidID);
  } else {
    showToast('Raid arena not available', 'error');
  }
}