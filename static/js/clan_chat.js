// ── Clan Chat Panel ────────────────────────────────────────────────────────────

var _clanChatState = {
  clanID: null,
  lastMsgID: 0,
  pollHandle: null,
  visibilityHandle: null,
  replyToID: null,
  replyToUser: null,
  replyToContent: null,
  reactionEmojis: ['👍', '🔥', '💀', '✅', '🤝', '⚔️']
};

function initClanChat(clanID) {
  // If switching clans or re-entering, reset state
  if (_clanChatState.clanID !== clanID) {
    _clanChatState.clanID = clanID;
    _clanChatState.lastMsgID = 0;
  }
  _clanChatState.replyToID = null;
  _clanChatState.replyToUser = null;
  _clanChatState.replyToContent = null;

  stopClanChatPoll();

  var mount = document.getElementById('chat-mount');
  if (!mount) return;
  mount.innerHTML = `
    <div class="clan-chat">
      <div id="clan-chat-messages" class="clan-chat-messages">
        <div class="clan-loading">Loading chat…</div>
      </div>
      <div id="clan-chat-reply-bar" class="clan-chat-reply-bar" style="display:none"></div>
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
    // Clear reply on Escape
    if (e.key === 'Escape') {
      clearReply();
    }
  });

  // Re-load full chat when the user switches back to this tab — fixes the
  // "only shows on reload" bug caused by the poll being cleared on tab change
  // and the container never re-rendering on first click back.
  if (_clanChatState.visibilityHandle) {
    document.removeEventListener('visibilitychange', _clanChatState.visibilityHandle);
  }
  _clanChatState.visibilityHandle = function() {
    if (document.visibilityState === 'visible' && _clanChatState.clanID === clanID) {
      loadClanChat(false);
    }
  };
  document.addEventListener('visibilitychange', _clanChatState.visibilityHandle);

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
  if (_clanChatState.visibilityHandle) {
    document.removeEventListener('visibilitychange', _clanChatState.visibilityHandle);
    _clanChatState.visibilityHandle = null;
  }
}

function loadClanChat(initial) {
  var clanID = _clanChatState.clanID;
  if (!clanID) return;

  // initial load OR after a tab re-focus passes since=0 to get full history
  var since = initial ? 0 : _clanChatState.lastMsgID;

  apiFetch('/api/clans/' + clanID + '/chat?since=' + since)
    .then(function(r) { return r.json(); })
    .then(function(msgs) {
      var container = document.getElementById('clan-chat-messages');
      if (!container) return;

      if (initial && (!msgs || msgs.length === 0)) {
        container.innerHTML = '<div class="clan-empty">No messages yet. Say hello to your clan!</div>';
        return;
      }
      if (!msgs || msgs.length === 0) return;

      var wasAtBottom = isScrolledToBottom(container);
      if (initial) container.innerHTML = '';

      msgs.forEach(function(m) {
        // Avoid duplicating messages that arrived via poll between initial fetches
        if (document.querySelector('[data-msg-id="' + m.id + '"]')) {
          if (m.id > _clanChatState.lastMsgID) _clanChatState.lastMsgID = m.id;
          return;
        }
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

  // Reply-to quote banner
  var replyBannerHtml = '';
  if (m.reply_to && m.reply_to_user_name) {
    replyBannerHtml = `
      <div class="clan-msg-reply-quote" onclick="_scrollToMessage(${m.reply_to})">
        <span class="clan-msg-reply-author">↩ ${escHtml(m.reply_to_user_name)}</span>
        <span class="clan-msg-reply-preview">${escHtml(m.reply_to_content || '')}</span>
      </div>`;
  }

  return `
    <div class="${bubbleClass}" data-msg-id="${m.id}">
      <div class="clan-msg-meta">
        <span class="clan-role-badge ${m.role}">${roleBadge(m.role)}</span>
        <span class="clan-msg-author">${escHtml(m.user_name)}</span>
        <span class="clan-msg-time">${timeAgo(m.created_at)}</span>
        <button class="clan-msg-reply-btn" onclick="startReply(${m.id}, '${escHtml(m.user_name).replace(/'/g,"\\'")}', '${escHtml((m.content||'').substring(0,80)).replace(/'/g,"\\'")}')">↩ Reply</button>
      </div>
      ${replyBannerHtml}
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

// ── Reply threading ───────────────────────────────────────────────────────────

function startReply(msgID, authorName, contentPreview) {
  _clanChatState.replyToID = msgID;
  _clanChatState.replyToUser = authorName;
  _clanChatState.replyToContent = contentPreview;

  var bar = document.getElementById('clan-chat-reply-bar');
  if (bar) {
    bar.style.display = '';
    bar.innerHTML = `
      <div class="clan-reply-bar-inner">
        <span class="clan-reply-bar-label">Replying to <strong>${escHtml(authorName)}</strong></span>
        <span class="clan-reply-bar-preview">${escHtml(contentPreview)}</span>
        <button class="clan-reply-bar-cancel" onclick="clearReply()">✕</button>
      </div>`;
  }

  var input = document.getElementById('clan-chat-input');
  if (input) input.focus();
}

function clearReply() {
  _clanChatState.replyToID = null;
  _clanChatState.replyToUser = null;
  _clanChatState.replyToContent = null;
  var bar = document.getElementById('clan-chat-reply-bar');
  if (bar) { bar.style.display = 'none'; bar.innerHTML = ''; }
}

function _scrollToMessage(msgID) {
  var el = document.querySelector('[data-msg-id="' + msgID + '"]');
  if (el) {
    el.scrollIntoView({ behavior: 'smooth', block: 'center' });
    el.classList.add('clan-msg-highlight');
    setTimeout(function() { el.classList.remove('clan-msg-highlight'); }, 1500);
  }
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

  var payload = { content: content };
  if (_clanChatState.replyToID) {
    payload.reply_to = _clanChatState.replyToID;
  }

  input.disabled = true;
  apiFetch('/api/clans/' + clanID + '/chat', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload)
  })
    .then(function(r) {
      if (!r.ok) return r.text().then(function(t) { throw new Error(t); });
      return r.json();
    })
    .then(function() {
      input.value = '';
      input.disabled = false;
      clearReply();
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
