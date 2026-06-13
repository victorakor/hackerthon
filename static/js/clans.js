// ── Clan Tab Entry Point ──────────────────────────────────────────────────────

function initClanTab() {
  renderClanTab();
}

function renderClanTab() {
  var container = document.getElementById('clan-view');
  if (!container) return;

  // Check if user is in a clan first
  apiFetch('/api/clans/mine')
    .then(function(r) {
      if (!r.ok) { renderClanBrowser(); return null; }
      return r.json();
    })
    .then(function(myClan) {
      if (!myClan || !myClan.id) {
        renderClanBrowser();
        return;
      }
      renderMyClanView(myClan);
    })
    .catch(function() {
      renderClanBrowser();
  });
}

// ── Clan Browser (user not in a clan) ────────────────────────────────────────

function renderClanBrowser() {
  var container = document.getElementById('clan-view');
  container.innerHTML = `
    <div class="clan-browser">
      <div class="clan-browser-header">
        <div>
          <h2 class="clan-browser-title">⚔️ Clan Battle</h2>
          <p class="clan-browser-sub">Join a clan, compete in raids, climb the clan leaderboard.</p>
        </div>
        <button class="btn-clan-create" onclick="showCreateClanModal()">+ Create Clan</button>
      </div>
      <div id="clan-list-area">
        <div class="clan-loading">Loading clans…</div>
      </div>
    </div>
    ${createClanModalHTML()}
  `;
  loadClanList();
}

function loadClanList() {
  apiFetch('/api/clans')
    .then(function(r) { return r.json(); })
    .then(function(clans) {
      var area = document.getElementById('clan-list-area');
      if (!area) return;
      if (!clans || clans.length === 0) {
        area.innerHTML = '<div class="clan-empty">No clans yet. Be the first to create one!</div>';
        return;
      }
      area.innerHTML = clans.map(function(c) {
        return `
          <div class="clan-card" onclick="showClanDetail(${c.id})">
            <div class="clan-card-left">
              <span class="clan-tag">[${escHtml(c.tag)}]</span>
              <div class="clan-card-info">
                <span class="clan-name">${escHtml(c.name)}</span>
                <span class="clan-desc">${escHtml(c.description || 'No description')}</span>
              </div>
            </div>
            <div class="clan-card-right">
              <span class="clan-rating">⭐ ${c.rating}</span>
              <span class="clan-members">${c.member_count}/50 members</span>
              <button class="btn-clan-join" onclick="event.stopPropagation(); joinClan(${c.id})">Join</button>
            </div>
          </div>
        `;
      }).join('');
    })
    .catch(function() {
      var area = document.getElementById('clan-list-area');
      if (area) area.innerHTML = '<div class="clan-empty">Failed to load clans.</div>';
    });
}

// ── Clan Detail Modal (from browser) ─────────────────────────────────────────

function showClanDetail(clanID) {
  apiFetch('/api/clans/' + clanID)
    .then(function(r) { return r.json(); })
    .then(function(clan) {
      var modal = document.getElementById('clan-detail-modal');
      if (!modal) {
        var el = document.createElement('div');
        el.id = 'clan-detail-modal';
        el.className = 'clan-modal-overlay';
        document.body.appendChild(el);
        modal = el;
      }
      modal.innerHTML = `
        <div class="clan-modal">
          <div class="clan-modal-header">
            <div>
              <span class="clan-tag">[${escHtml(clan.tag)}]</span>
              <span class="clan-modal-name">${escHtml(clan.name)}</span>
            </div>
            <button class="clan-modal-close" onclick="closeClanDetailModal()">✕</button>
          </div>
          <div class="clan-modal-meta">
            <span>⭐ ${clan.rating} rating</span>
            <span>👥 ${clan.member_count}/50 members</span>
          </div>
          <p class="clan-modal-desc">${escHtml(clan.description || 'No description')}</p>
          <div class="clan-member-list">
            <h4>Members</h4>
            ${clan.members.map(function(m) {
              return `<div class="clan-member-row">
                <span class="clan-role-badge ${m.role}">${roleBadge(m.role)}</span>
                <span>${escHtml(m.user_name)}</span>
              </div>`;
            }).join('')}
          </div>
          <div class="clan-modal-actions">
            ${clan.my_role
              ? `<span class="clan-already-member">You are a member (${clan.my_role})</span>`
              : `<button class="btn-clan-join" onclick="joinClan(${clan.id}); closeClanDetailModal()">Join Clan</button>`
            }
          </div>
        </div>
      `;
      modal.style.display = 'flex';
    });
}

function closeClanDetailModal() {
  var modal = document.getElementById('clan-detail-modal');
  if (modal) modal.style.display = 'none';
}

// ── My Clan View (user is in a clan) ─────────────────────────────────────────

function renderMyClanView(clan) {
  var container = document.getElementById('clan-view');
  container.innerHTML = `
    <div class="my-clan-view">
      <div class="my-clan-header">
        <div class="my-clan-title-row">
          <span class="clan-tag">[${escHtml(clan.tag)}]</span>
          <h2 class="my-clan-name">${escHtml(clan.name)}</h2>
          <span class="clan-role-badge ${clan.my_role}">${roleBadge(clan.my_role)}</span>
        </div>
        <div class="my-clan-meta">
          <span>⭐ ${clan.rating} rating</span>
          <span>👥 ${clan.member_count}/50 members</span>
        </div>
      </div>

      <div class="my-clan-tabs">
        <button class="clan-inner-tab active" onclick="switchClanInnerTab('members', this)">👥 Members</button>
        <button class="clan-inner-tab" onclick="switchClanInnerTab('chat', this)">💬 Chat</button>
        <button class="clan-inner-tab" onclick="switchClanInnerTab('raids', this)">⚔️ Raids</button>
      </div>

      <div id="clan-inner-members" class="clan-inner-panel">
        ${renderMembersPanel(clan)}
      </div>
      <div id="clan-inner-chat" class="clan-inner-panel" style="display:none">
        <div id="chat-mount"></div>
      </div>
      <div id="clan-inner-raids" class="clan-inner-panel" style="display:none">
        <div id="raids-mount"></div>
      </div>
    </div>
  `;

  // Store clan context for other modules
  window._myClan = clan;
}

function renderMembersPanel(clan) {
  var canLeave = true;
  var rows = clan.members.map(function(m) {
    return `
      <div class="clan-member-row">
        <span class="clan-role-badge ${m.role}">${roleBadge(m.role)}</span>
        <span class="member-name">${escHtml(m.user_name)}</span>
        <span class="member-joined">Joined ${formatDate(m.joined_at)}</span>
      </div>
    `;
  }).join('');

  return `
    <div class="members-panel">
      <div class="members-list">${rows}</div>
      <div class="members-actions">
        <button class="btn-clan-leave" onclick="confirmLeaveClan(${clan.id})">Leave Clan</button>
      </div>
    </div>
  `;
}

function switchClanInnerTab(name, btn) {
  // Hide all panels
  ['members','chat','raids'].forEach(function(t) {
    var el = document.getElementById('clan-inner-' + t);
    if (el) el.style.display = 'none';
  });
  // Deactivate all tab buttons
  document.querySelectorAll('.clan-inner-tab').forEach(function(b) {
    b.classList.remove('active');
  });
  // Show selected
  var panel = document.getElementById('clan-inner-' + name);
  if (panel) panel.style.display = '';
  btn.classList.add('active');

  // Lazy-init sub-modules
  if (name === 'chat' && window._myClan) {
    initClanChat(window._myClan.id);
  }
  if (name === 'raids' && window._myClan) {
    initRaidsPanel(window._myClan.id, window._myClan.my_role);
  }
}

// ── Create Clan Modal ─────────────────────────────────────────────────────────

function createClanModalHTML() {
  return `
    <div id="create-clan-modal" class="clan-modal-overlay" style="display:none">
      <div class="clan-modal">
        <div class="clan-modal-header">
          <h3>Create a Clan</h3>
          <button class="clan-modal-close" onclick="closeCreateClanModal()">✕</button>
        </div>
        <div class="clan-form">
          <label>Clan Name
            <input id="clan-name-input" type="text" placeholder="e.g. Code Warriors" maxlength="40">
          </label>
          <label>Tag (2–5 chars)
            <input id="clan-tag-input" type="text" placeholder="e.g. CW" maxlength="5">
          </label>
          <label>Description
            <textarea id="clan-desc-input" placeholder="What is your clan about?" maxlength="200" rows="3"></textarea>
          </label>
          <div id="clan-create-error" class="clan-form-error" style="display:none"></div>
          <button class="btn-clan-create" onclick="submitCreateClan()">Create Clan</button>
        </div>
      </div>
    </div>
  `;
}

function showCreateClanModal() {
  document.getElementById('create-clan-modal').style.display = 'flex';
}

function closeCreateClanModal() {
  document.getElementById('create-clan-modal').style.display = 'none';
  document.getElementById('clan-create-error').style.display = 'none';
}

function submitCreateClan() {
  var name = document.getElementById('clan-name-input').value.trim();
  var tag  = document.getElementById('clan-tag-input').value.trim();
  var desc = document.getElementById('clan-desc-input').value.trim();
  var errEl = document.getElementById('clan-create-error');

  if (!name || !tag) {
    errEl.textContent = 'Name and tag are required.';
    errEl.style.display = '';
    return;
  }
  if (tag.length < 2 || tag.length > 5) {
    errEl.textContent = 'Tag must be 2–5 characters.';
    errEl.style.display = '';
    return;
  }

  apiFetch('/api/clans', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name: name, tag: tag, description: desc })
  })
    .then(function(r) {
      if (!r.ok) return r.text().then(function(t) { throw new Error(t); });
      return r.json();
    })
    .then(function() {
      closeCreateClanModal();
      renderClanTab();
    })
    .catch(function(e) {
      errEl.textContent = e.message || 'Failed to create clan.';
      errEl.style.display = '';
    });
}

// ── Join / Leave ──────────────────────────────────────────────────────────────

function joinClan(clanID) {
  apiFetch('/api/clans/' + clanID + '/join', { method: 'POST' })
    .then(function(r) {
      if (!r.ok) return r.text().then(function(t) { throw new Error(t); });
      return r.json();
    })
    .then(function() {
      renderClanTab();
    })
    .catch(function(e) {
      alert(e.message || 'Failed to join clan.');
    });
}

function confirmLeaveClan(clanID) {
  if (!confirm('Are you sure you want to leave this clan?')) return;
  apiFetch('/api/clans/' + clanID + '/leave', { method: 'DELETE' })
    .then(function(r) {
      if (!r.ok) return r.text().then(function(t) { throw new Error(t); });
      return r.json();
    })
    .then(function() {
      window._myClan = null;
      renderClanTab();
    })
    .catch(function(e) {
      alert(e.message || 'Failed to leave clan.');
    });
}

// ── Utilities ─────────────────────────────────────────────────────────────────

function roleBadge(role) {
  if (role === 'clanhead') return '👑 Clanhead';
  if (role === 'general')  return '⚔️ General';
  return '🧑 Member';
}

function escHtml(str) {
  if (!str) return '';
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

function formatDate(iso) {
  if (!iso) return '';
  var d = new Date(iso);
  return d.toLocaleDateString();
}

async function initClanBadge() {
  try {
    // Fetch user's clan first (window._myClan not set yet at login time)
    var mineRes = await apiFetch('/api/clans/mine');
    if (!mineRes.ok) return;
    var myClan = await mineRes.json();
    if (!myClan || !myClan.id) return;

    var res = await apiFetch('/api/raids');
    if (!res.ok) return;
    var raids = await res.json();
    if (!raids || !raids.length) return;

    var hasActive = raids.some(function(r) {
      return r.status === 'active' &&
        r.clans.some(function(c) { return c.clan_id === myClan.id; });
    });
    if (hasActive) {
      var badge = document.getElementById('clan-badge');
      if (badge) badge.style.display = '';
    }
  } catch(e) {}
}