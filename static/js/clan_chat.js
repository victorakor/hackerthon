// ── Clan Chat Panel ────────────────────────────────────────────────────────────

var _clanChatState = {
  clanID: null,
  lastMsgID: 0,
  pollHandle: null,
  reactionEmojis: ['👍', '🔥', '💀', '✅', '🤝', '⚔️']
};

function initClanChat(clanID) {
  // If switching clans or re-entering, reset state
  if (_clanChatState.clanID !== clanID) {
    _clanChatState.clanID = clanID;
    _clanChatState.lastMsgID = 0;
  }
  stopClanChatPoll();

  var mount = document.getElementById('chat-mount');
  if (!mount) return;
  mount.innerHTML = `
    <div class="clan-chat">
      <div id="clan-chat-messages" class="clan-chat-messages">
        <div class="clan-loading">Loading chat…</div>
      </div>
      <div class="clan-chat-input-row">
        <textarea id="clan-chat-input" class="clan-chat-input" placeholder="Share code, plan a raid… (use \`\`\` for code blocks)" rows="2" maxlength="2000"></textarea>
        <button class="btn-clan-send" onclick="sendClanChatMessage()">Send</button>
      </div>
    </div>
  `;

  document.getElementById('clan-chat-input').addEventListener('keydown', function(e) {
    if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
      e.preventDefault();
      sendClanChatMessage();
    }
  });

  loadClanChat(true);
  _clanChatState.pollHandle = setInterval(function() {
    loadClanChat(false);
  }, 3000);
}

function stopClanChatPoll() {
  if (_clanChatState.pollHandle) {
    clearInterval(_clanChatState.pollHandle);
    _clanChatState.pollHandle = null;
  }
}

function loadClanChat(initial) {
  var clanID = _clanChatState.clanID;
  if (!clanID) return;

  apiFetch('/api/clans/' + clanID + '/chat?since=' + _clanChatState.lastMsgID)
    .then(function(r) { return r.json(); })
    .then(function(msgs) {
      var container = document.getElementById('clan-chat-messages');
      if (!container) return;

      if (initial && (!msgs || msgs.length === 0)) {
        container.innerHTML = '<div class="clan-empty">No messages yet. Say hello to your clan!</div>';
        return;
      }
      if (initial) {
        container.innerHTML = '';
      }
      if (!msgs || msgs.length === 0) return;

      var wasAtBottom = isScrolledToBottom(container);
      if (initial) container.innerHTML = '';

      msgs.forEach(function(m) {
        container.insertAdjacentHTML('beforeend', renderClanMessage(m));
        if (m.id > _clanChatState.lastMsgID) _clanChatState.lastMsgID = m.id;
      });

      if (initial || wasAtBottom) {
        container.scrollTop = container.scrollHeight;
      }
    })
    .catch(function() {
      var container = document.getElementById('clan-chat-messages');
      if (container && initial) {
        container.innerHTML = '<div class="clan-empty">Failed to load chat.</div>';
      }
    });
}

function isScrolledToBottom(el) {
  return el.scrollHeight - el.scrollTop - el.clientHeight < 60;
}

function renderClanMessage(m) {
  var isMe = currentUser && m.user_id === currentUser.id;
  var bubbleClass = isMe ? 'clan-msg clan-msg-me' : 'clan-msg';
  var reactionsHtml = renderReactions(m.id, m.reactions || []);
  return `
    <div class="${bubbleClass}" data-msg-id="${m.id}">
      <div class="clan-msg-meta">
        <span class="clan-role-badge ${m.role}">${roleBadge(m.role)}</span>
        <span class="clan-msg-author">${escHtml(m.user_name)}</span>
        <span class="clan-msg-time">${timeAgo(m.created_at)}</span>
      </div>
      <div class="clan-msg-content">${renderMessageContent(m.content)}</div>
      <div class="clan-msg-actions">
        ${reactionsHtml}
        <div class="clan-reaction-picker">
          <button class="clan-reaction-add" onclick="toggleReactionPicker(${m.id})">+ 😊</button>
          <div class="clan-reaction-options" id="reaction-options-${m.id}" style="display:none">
            ${_clanChatState.reactionEmojis.map(function(e) {
              return `<span onclick="reactToMessage(${m.id}, '${e}')">${e}</span>`;
            }).join('')}
          </div>
        </div>
      </div>
    </div>
  `;
}

// Render message content — supports ```code blocks``` and plain text
function renderMessageContent(content) {
  var escaped = escHtml(content);
  // Replace ```...``` blocks (after escaping, backticks survive escHtml)
  var withCode = escaped.replace(/```([\s\S]*?)```/g, function(_, code) {
    return '<pre class="clan-code-block"><code>' + code.trim() + '</code></pre>';
  });
  // Inline `code`
  withCode = withCode.replace(/`([^`]+)`/g, '<code class="clan-inline-code">$1</code>');
  // Preserve newlines outside code blocks
  return withCode.replace(/\n/g, '<br>');
}

function renderReactions(msgID, reactions) {
  if (!reactions || reactions.length === 0) return '<div class="clan-reactions"></div>';
  return '<div class="clan-reactions">' + reactions.map(function(rx) {
    var activeClass = rx.reacted ? 'reacted' : '';
    return `<span class="clan-reaction-pill ${activeClass}" onclick="reactToMessage(${msgID}, '${rx.emoji}')">${rx.emoji} ${rx.count}</span>`;
  }).join('') + '</div>';
}

function toggleReactionPicker(msgID) {
  var el = document.getElementById('reaction-options-' + msgID);
  if (!el) return;
  var isOpen = el.style.display !== 'none';
  // Close all other open pickers
  document.querySelectorAll('.clan-reaction-options').forEach(function(o) {
    o.style.display = 'none';
  });
  el.style.display = isOpen ? 'none' : 'inline-flex';
}

function reactToMessage(msgID, emoji) {
  var clanID = _clanChatState.clanID;
  if (!clanID) return;

  // Determine if already reacted (toggle behavior)
  var msgEl = document.querySelector('[data-msg-id="' + msgID + '"]');
  var alreadyReacted = false;
  if (msgEl) {
    var pill = msgEl.querySelector('.clan-reaction-pill.reacted');
    if (pill && pill.textContent.indexOf(emoji) !== -1) alreadyReacted = true;
  }

  var method = alreadyReacted ? 'DELETE' : 'POST';

  apiFetch('/api/clans/' + clanID + '/chat/' + msgID + '/react', {
    method: method,
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ emoji: emoji })
  })
    .then(function(r) { return r.json(); })
    .then(function(reactions) {
      if (!msgEl) return;
      var actionsEl = msgEl.querySelector('.clan-msg-actions');
      var pickerHtml = `
        <div class="clan-reaction-picker">
          <button class="clan-reaction-add" onclick="toggleReactionPicker(${msgID})">+ 😊</button>
          <div class="clan-reaction-options" id="reaction-options-${msgID}" style="display:none">
            ${_clanChatState.reactionEmojis.map(function(e) {
              return `<span onclick="reactToMessage(${msgID}, '${e}')">${e}</span>`;
            }).join('')}
          </div>
        </div>
      `;
      actionsEl.innerHTML = renderReactions(msgID, reactions) + pickerHtml;
    })
    .catch(function() {
      showToast('Failed to react', 'error');
    });
}

function sendClanChatMessage() {
  var clanID = _clanChatState.clanID;
  if (!clanID) return;
  var input = document.getElementById('clan-chat-input');
  var content = input.value.trim();
  if (!content) return;

  input.disabled = true;
  apiFetch('/api/clans/' + clanID + '/chat', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ content: content })
  })
    .then(function(r) {
      if (!r.ok) return r.text().then(function(t) { throw new Error(t); });
      return r.json();
    })
    .then(function() {
      input.value = '';
      input.disabled = false;
      loadClanChat(false);
    })
    .catch(function(e) {
      input.disabled = false;
      showToast(e.message || 'Failed to send message', 'error');
    });
}

// Close reaction pickers when clicking outside
document.addEventListener('click', function(e) {
  if (!e.target.closest('.clan-reaction-picker')) {
    document.querySelectorAll('.clan-reaction-options').forEach(function(o) {
      o.style.display = 'none';
    });
  }
});