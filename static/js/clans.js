// ── Clan Tab Entry Point ──────────────────────────────────────────────────────

function initClanTab() {
  console.log('[CLAN] initClanTab called');
  renderClanTab();
}

function renderClanTab() {
  console.log('[CLAN] renderClanTab called');
  var container = document.getElementById('clan-view');
  if (!container) {
    console.error('[CLAN] ERROR: #clan-view element not found in DOM!');
    return;
  }
  console.log('[CLAN] #clan-view found, fetching /api/clans/mine ...');

  // Check if user is in a clan first
  apiFetch('/api/clans/mine')
    .then(function(r) {
      console.log('[CLAN] /api/clans/mine response — status:', r.status, 'ok:', r.ok);
      if (!r.ok) {
        console.warn('[CLAN] /api/clans/mine not ok (status ' + r.status + '), calling renderClanBrowser()');
        renderClanBrowser();
        return null;
      }
      // Clone response so we can read body twice — once as text for logging, once as JSON
      return r.clone().text().then(function(raw) {
        console.log('[CLAN] /api/clans/mine raw body:', JSON.stringify(raw));
        try {
          var parsed = JSON.parse(raw);
          console.log('[CLAN] /api/clans/mine parsed JSON:', JSON.stringify(parsed));
          return parsed;
        } catch(e) {
          console.error('[CLAN] /api/clans/mine JSON.parse FAILED:', e.message, '— raw was:', JSON.stringify(raw));
          return null;
        }
      });
    })
    .then(function(myClan) {
      if (myClan === undefined) return; // already handled above
      console.log('[CLAN] myClan value:', JSON.stringify(myClan), '| type:', typeof myClan);
      if (!myClan || !myClan.id) {
        console.log('[CLAN] No clan membership found (myClan is null/empty/no id), calling renderClanBrowser()');
        renderClanBrowser();
        return;
      }
      console.log('[CLAN] User is in clan id=' + myClan.id + ', calling renderMyClanView()');
      renderMyClanView(myClan);
    })
    .catch(function(err) {
      console.error('[CLAN] UNCAUGHT ERROR in renderClanTab fetch chain:', err && err.message, err);
      renderClanBrowser();
    });
}

// ── Clan Browser (user not in a clan) ────────────────────────────────────────

function renderClanBrowser() {
  console.log('[CLAN] renderClanBrowser called');
  var container = document.getElementById('clan-view');
  if (!container) {
    console.error('[CLAN] ERROR: #clan-view not found inside renderClanBrowser!');
    return;
  }
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
  console.log('[CLAN] renderClanBrowser: innerHTML set, checking for Create button and modal in DOM...');
  var btn = container.querySelector('.btn-clan-create');
  var modal = document.getElementById('create-clan-modal');
  console.log('[CLAN] .btn-clan-create found:', !!btn, '| #create-clan-modal found:', !!modal);
  loadClanList();
}

function loadClanList() {
  console.log('[CLAN] loadClanList called, fetching /api/clans ...');
  apiFetch('/api/clans')
    .then(function(r) {
      console.log('[CLAN] /api/clans response — status:', r.status, 'ok:', r.ok);
      if (!r.ok) {
        throw new Error('HTTP ' + r.status);
      }
      return r.json();
    })
    .then(function(clans) {
      console.log('[CLAN] /api/clans parsed JSON — type:', typeof clans, 'isArray:', Array.isArray(clans), 'length:', clans ? clans.length : 'N/A');
      var area = document.getElementById('clan-list-area');
      if (!area) {
        console.error('[CLAN] ERROR: #clan-list-area not found! Was renderClanBrowser called first?');
        return;
      }
      if (!clans || clans.length === 0) {
        console.log('[CLAN] No clans exist — rendering empty state with Create button');
        area.innerHTML = '<div class="clan-empty">No clans yet. Be the first to create one!<br><br><button class="btn-clan-create" onclick="showCreateClanModal()" style="margin-top:8px">+ Create Clan</button></div>';
        console.log('[CLAN] Empty state rendered. Create button in area:', !!area.querySelector('.btn-clan-create'));
        return;
      }
      console.log('[CLAN] Rendering ' + clans.length + ' clan(s)');
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
    .catch(function(err) {
      console.error('[CLAN] ERROR in loadClanList:', err);
      var area = document.getElementById('clan-list-area');
      if (area) {
        area.innerHTML = '<div class="clan-empty">Failed to load clans. <a href="#" onclick="loadClanList();return false">Retry</a><br><br><button class="btn-clan-create" onclick="showCreateClanModal()" style="margin-top:8px">+ Create Clan</button></div>';
      } else {
        console.error('[CLAN] ERROR: #clan-list-area also missing during error handling!');
      }
    });
}

// ── Clan Detail Modal (from browser) ─────────────────────────────────────────

function showClanDetail(clanID) {
  console.log('[CLAN] showClanDetail called for clanID:', clanID);
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
  console.log('[CLAN] renderMyClanView called for clan:', clan.name, 'role:', clan.my_role);
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
  console.log('[CLAN] showCreateClanModal called');
  var modal = document.getElementById('create-clan-modal');
  if (!modal) {
    console.error('[CLAN] ERROR: #create-clan-modal not found in DOM! Was renderClanBrowser() called?');
    return;
  }
  modal.style.display = 'flex';
  console.log('[CLAN] Modal display set to flex');
}

function closeCreateClanModal() {
  var modal = document.getElementById('create-clan-modal');
  if (modal) modal.style.display = 'none';
  var err = document.getElementById('clan-create-error');
  if (err) err.style.display = 'none';
}

function submitCreateClan() {
  var name = document.getElementById('clan-name-input').value.trim();
  var tag  = document.getElementById('clan-tag-input').value.trim();
  var desc = document.getElementById('clan-desc-input').value.trim();
  var errEl = document.getElementById('clan-create-error');

  console.log('[CLAN] submitCreateClan — name:', name, 'tag:', tag);

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

  console.log('[CLAN] Posting to /api/clans ...');
  apiFetch('/api/clans', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name: name, tag: tag, description: desc })
  })
    .then(function(r) {
      console.log('[CLAN] POST /api/clans response — status:', r.status, 'ok:', r.ok);
      if (!r.ok) return r.text().then(function(t) { throw new Error(t); });
      return r.json();
    })
    .then(function(data) {
      console.log('[CLAN] Clan created successfully:', JSON.stringify(data));
      closeCreateClanModal();
      renderClanTab();
    })
    .catch(function(e) {
      console.error('[CLAN] ERROR creating clan:', e);
      errEl.textContent = e.message || 'Failed to create clan.';
      errEl.style.display = '';
    });
}

// ── Join / Leave ──────────────────────────────────────────────────────────────

function joinClan(clanID) {
  console.log('[CLAN] joinClan called for clanID:', clanID);
  apiFetch('/api/clans/' + clanID + '/join', { method: 'POST' })
    .then(function(r) {
      console.log('[CLAN] JOIN response — status:', r.status, 'ok:', r.ok);
      if (!r.ok) return r.text().then(function(t) { throw new Error(t); });
      return r.json();
    })
    .then(function() {
      renderClanTab();
    })
    .catch(function(e) {
      console.error('[CLAN] ERROR joining clan:', e);
      alert(e.message || 'Failed to join clan.');
    });
}

function confirmLeaveClan(clanID) {
  if (!confirm('Are you sure you want to leave this clan?')) return;
  apiFetch('/api/clans/' + clanID + '/leave', { method: 'DELETE' })
    .then(function(r) {
      console.log('[CLAN] LEAVE response — status:', r.status, 'ok:', r.ok);
      if (!r.ok) return r.text().then(function(t) { throw new Error(t); });
      return r.json();
    })
    .then(function() {
      window._myClan = null;
      renderClanTab();
    })
    .catch(function(e) {
      console.error('[CLAN] ERROR leaving clan:', e);
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
