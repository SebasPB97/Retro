'use strict';

// ─── State ───────────────────────────────────────────────────────────────────
const state = {
  session: null,
  currentUser: null,
  supabase: null,
  channel: null,
  focused: null,         // focused card object
  timerInterval: null,
};

// ─── Helpers ─────────────────────────────────────────────────────────────────
function escHtml(str) {
  if (!str) return '';
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

function getSessionID() {
  const match = location.pathname.match(/\/retro\/([^/?#]+)/);
  return match ? match[1] : null;
}

function getUserID() {
  let uid = sessionStorage.getItem('retro_user_id');
  if (!uid) {
    uid = crypto.randomUUID();
    sessionStorage.setItem('retro_user_id', uid);
  }
  return uid;
}

function getUsername() {
  let name = sessionStorage.getItem('retro_username');
  if (!name) {
    name = prompt('¿Cuál es tu nombre?', '');
    if (!name) name = 'Anon-' + Math.floor(Math.random() * 1000);
    sessionStorage.setItem('retro_username', name);
  }
  return name;
}

function isHost() {
  return state.session && state.currentUser &&
    state.session.host_id === state.currentUser.id;
}

function findCard(cardID) {
  if (!state.session) return null;
  return state.session.cards.find(c => c.id === cardID) || null;
}

// ─── API Helper ───────────────────────────────────────────────────────────────
async function api(endpoint, method = 'POST', body = null) {
  const opts = { method, headers: { 'Content-Type': 'application/json' } };
  if (body) opts.body = JSON.stringify(body);
  const res = await fetch(`/api/${endpoint}`, opts);
  if (!res.ok) {
    const txt = await res.text();
    throw new Error(txt);
  }
  return res.json().catch(() => null);
}

// ─── Connection Banner ────────────────────────────────────────────────────────
function showConnectionBanner(msg) {
  const el = document.getElementById('connection-status');
  document.getElementById('connection-msg').textContent = msg;
  el.style.display = 'flex';
}

function hideConnectionBanner() {
  document.getElementById('connection-status').style.display = 'none';
}

// ─── Init ─────────────────────────────────────────────────────────────────────
async function init() {
  const sessionID = getSessionID();
  if (!sessionID) { window.location.href = '/'; return; }

  const userID = getUserID();
  const username = getUsername();
  state.currentUser = { id: userID, username };

  setupEventListeners();
  showConnectionBanner('Conectando...');

  // Load Supabase config from backend
  let config;
  try {
    config = await fetch('/api/config').then(r => r.json());
  } catch (e) {
    showConnectionBanner('Error cargando configuración. Reintentando...');
    setTimeout(init, 3000);
    return;
  }

  state.supabase = supabase.createClient(config.supabase_url, config.supabase_anon_key);

  // Load initial session state
  try {
    const sess = await fetch(`/api/session?id=${sessionID}`).then(r => {
      if (!r.ok) throw new Error('Session not found');
      return r.json();
    });
    state.session = sess;
    if (!state.session.cards) state.session.cards = [];
    if (!state.session.columns) state.session.columns = [];
    if (!state.session.users) state.session.users = [];
    renderFullBoard();
    hideConnectionBanner();
  } catch (e) {
    showConnectionBanner('Sesión no encontrada. Redirigiendo...');
    setTimeout(() => window.location.href = '/', 2000);
    return;
  }

  // Subscribe to Supabase Realtime
  subscribeRealtime(sessionID, userID, username);
}

// ─── Realtime ─────────────────────────────────────────────────────────────────
function subscribeRealtime(sessionID, userID, username) {
  const channel = state.supabase.channel(`retro:${sessionID}`, {
    config: { presence: { key: userID } }
  });

  // Presence
  channel
    .on('presence', { event: 'sync' }, () => {
      const presenceState = channel.presenceState();
      const users = Object.values(presenceState).flat().map(p => ({
        id: p.user_id,
        username: p.username,
        is_host: p.is_host,
      }));
      if (state.session) {
        state.session.users = users;
        renderUsers();
      }
    })
    .on('presence', { event: 'join' }, () => {})
    .on('presence', { event: 'leave' }, () => {});

  // Sessions table (phase, focused_card, timer)
  channel.on('postgres_changes', {
    event: 'UPDATE', schema: 'public', table: 'sessions', filter: `id=eq.${sessionID}`
  }, handleSessionChange);

  // Columns
  channel.on('postgres_changes', {
    event: 'INSERT', schema: 'public', table: 'columns', filter: `session_id=eq.${sessionID}`
  }, handleColumnInsert);
  channel.on('postgres_changes', {
    event: 'DELETE', schema: 'public', table: 'columns', filter: `session_id=eq.${sessionID}`
  }, handleColumnDelete);

  // Cards
  channel.on('postgres_changes', {
    event: 'INSERT', schema: 'public', table: 'cards', filter: `session_id=eq.${sessionID}`
  }, handleCardInsert);
  channel.on('postgres_changes', {
    event: 'UPDATE', schema: 'public', table: 'cards', filter: `session_id=eq.${sessionID}`
  }, handleCardUpdate);
  channel.on('postgres_changes', {
    event: 'DELETE', schema: 'public', table: 'cards', filter: `session_id=eq.${sessionID}`
  }, handleCardDelete);

  // Votes — re-render entire column so cards re-sort by vote count
  channel.on('postgres_changes', {
    event: 'INSERT', schema: 'public', table: 'card_votes', filter: `session_id=eq.${sessionID}`
  }, ({ new: row }) => {
    const card = findCard(row.card_id);
    if (card && !card.votes.includes(row.user_id)) {
      card.votes.push(row.user_id);
      renderColumn(card.column_id);
      if (state.focused && state.focused.id === row.card_id) renderFocusVote();
    }
  });
  channel.on('postgres_changes', {
    event: 'DELETE', schema: 'public', table: 'card_votes', filter: `session_id=eq.${sessionID}`
  }, ({ old: row }) => {
    const card = findCard(row.card_id);
    if (card) {
      card.votes = card.votes.filter(v => v !== row.user_id);
      renderColumn(card.column_id);
      if (state.focused && state.focused.id === row.card_id) renderFocusVote();
    }
  });

  // Comments
  channel.on('postgres_changes', {
    event: 'INSERT', schema: 'public', table: 'comments', filter: `session_id=eq.${sessionID}`
  }, ({ new: row }) => {
    const card = findCard(row.card_id);
    if (card) {
      if (!card.comments) card.comments = [];
      // Avoid duplicates
      if (!card.comments.find(c => c.id === row.id)) {
        card.comments.push({ id: row.id, text: row.text, user_id: row.user_id, username: row.username, created_at: row.created_at });
        renderCardElement(row.card_id);
        if (state.focused && state.focused.id === row.card_id) {
          state.focused.comments = card.comments;
          renderComments();
        }
      }
    }
  });

  // Reactions
  channel.on('postgres_changes', {
    event: 'INSERT', schema: 'public', table: 'reactions', filter: `session_id=eq.${sessionID}`
  }, ({ new: row }) => {
    const card = findCard(row.card_id);
    if (card) {
      if (!card.reactions) card.reactions = [];
      if (!card.reactions.find(r => r.id === row.id)) {
        card.reactions.push({ id: row.id, emoji: row.emoji, user_id: row.user_id, username: row.username });
        renderCardElement(row.card_id);
        if (state.focused && state.focused.id === row.card_id) {
          state.focused.reactions = card.reactions;
          renderReactions();
        }
      }
    }
  });
  channel.on('postgres_changes', {
    event: 'DELETE', schema: 'public', table: 'reactions', filter: `session_id=eq.${sessionID}`
  }, ({ old: row }) => {
    const card = findCard(row.card_id);
    if (card) {
      card.reactions = (card.reactions || []).filter(rx => rx.id !== row.id);
      renderCardElement(row.card_id);
      if (state.focused && state.focused.id === row.card_id) {
        state.focused.reactions = card.reactions;
        renderReactions();
      }
    }
  });

  // Action items
  channel.on('postgres_changes', {
    event: 'INSERT', schema: 'public', table: 'action_items', filter: `session_id=eq.${sessionID}`
  }, ({ new: row }) => {
    const card = findCard(row.card_id);
    if (card) {
      if (!card.actions) card.actions = [];
      if (!card.actions.find(a => a.id === row.id)) {
        card.actions.push({ id: row.id, text: row.text, assignee: row.assignee, due_date: row.due_date, done: row.done, created_at: row.created_at });
        renderCardElement(row.card_id);
        if (state.focused && state.focused.id === row.card_id) {
          state.focused.actions = card.actions;
          renderActions();
        }
        renderActionsPanel();
      }
    }
  });
  channel.on('postgres_changes', {
    event: 'UPDATE', schema: 'public', table: 'action_items', filter: `session_id=eq.${sessionID}`
  }, ({ new: row }) => {
    const card = findCard(row.card_id);
    if (card) {
      const idx = (card.actions || []).findIndex(a => a.id === row.id);
      if (idx !== -1) card.actions[idx] = { ...card.actions[idx], done: row.done, text: row.text, assignee: row.assignee, due_date: row.due_date };
      renderCardElement(row.card_id);
      if (state.focused && state.focused.id === row.card_id) {
        state.focused.actions = card.actions;
        renderActions();
      }
      renderActionsPanel();
    }
  });
  channel.on('postgres_changes', {
    event: 'DELETE', schema: 'public', table: 'action_items', filter: `session_id=eq.${sessionID}`
  }, ({ old: row }) => {
    const card = findCard(row.card_id);
    if (card) {
      card.actions = (card.actions || []).filter(a => a.id !== row.id);
      renderCardElement(row.card_id);
      if (state.focused && state.focused.id === row.card_id) {
        state.focused.actions = card.actions;
        renderActions();
      }
      renderActionsPanel();
    }
  });

  channel.subscribe(async (status) => {
    if (status === 'SUBSCRIBED') {
      await channel.track({
        user_id: userID,
        username: username,
        is_host: state.session && state.session.host_id === userID,
      });
    }
  });

  state.channel = channel;
}

// ─── Realtime Handlers ────────────────────────────────────────────────────────
function handleSessionChange({ new: row }) {
  if (!state.session) return;
  const oldPhase = state.session.phase;
  const oldFocused = state.session.focused_card_id;
  const wasTimerRunning = state.session.timer && state.session.timer.running;

  state.session.phase = row.phase;
  state.session.focused_card_id = row.focused_card_id || '';

  if (row.timer_running && !wasTimerRunning) {
    state.session.timer = { duration: row.timer_duration, started_at: row.timer_started_at, running: true };
    startTimerDisplay(state.session.timer);
  } else if (!row.timer_running && wasTimerRunning) {
    if (state.session.timer) state.session.timer.running = false;
    stopTimerDisplay();
  }

  if (row.phase !== oldPhase) {
    renderPhase();
    renderAllColumns();
  }

  const newFocused = row.focused_card_id || '';
  const prevFocused = oldFocused || '';
  if (newFocused !== prevFocused) {
    if (newFocused) {
      const fc = findCard(newFocused);
      if (fc) openFocusOverlay(fc);
    } else {
      closeFocusOverlay();
    }
  }
}

function handleColumnInsert({ new: row }) {
  if (!state.session) return;
  if (!state.session.columns.find(c => c.id === row.id)) {
    state.session.columns.push({ id: row.id, name: row.name, color: row.color, order: row.order });
  }
  renderAllColumns();
}

function handleColumnDelete({ old: row }) {
  if (!state.session) return;
  state.session.columns = state.session.columns.filter(c => c.id !== row.id);
  state.session.cards = state.session.cards.filter(c => c.column_id !== row.id);
  renderAllColumns();
}

function handleCardInsert({ new: row }) {
  if (!state.session) return;
  if (state.session.cards.find(c => c.id === row.id)) return; // already exists
  state.session.cards.push({
    id: row.id,
    column_id: row.column_id,
    text: row.text,
    author: row.author,
    author_id: row.author_id,
    group_id: row.group_id || '',
    votes: [],
    comments: [],
    reactions: [],
    actions: [],
    created_at: row.created_at,
  });
  renderColumn(row.column_id);
}

function handleCardUpdate({ new: row }) {
  if (!state.session) return;
  const card = findCard(row.id);
  if (!card) return;
  const oldColID = card.column_id;
  card.text = row.text;
  card.column_id = row.column_id;
  card.group_id = row.group_id || '';
  if (oldColID !== row.column_id) {
    renderColumn(oldColID);
    renderColumn(row.column_id);
  } else {
    renderCardElement(row.id);
  }
  if (state.focused && state.focused.id === row.id) {
    state.focused.text = row.text;
    const textEl = document.getElementById('focus-card-text');
    if (textEl) textEl.textContent = row.text;
  }
}

function handleCardDelete({ old: row }) {
  if (!state.session) return;
  const card = findCard(row.id);
  const colID = card ? card.column_id : null;
  state.session.cards = state.session.cards.filter(c => c.id !== row.id);
  if (colID) renderColumn(colID);
  if (state.focused && state.focused.id === row.id) closeFocusOverlay();
}

// ─── Render ──────────────────────────────────────────────────────────────────
const PHASE_LABELS = { adding: 'Añadiendo', voting: 'Votando', discussing: 'Discutiendo', done: 'Terminada' };
const PHASE_COLORS = { adding: '#4caf50', voting: '#2196f3', discussing: '#ff9800', done: '#757575' };

function renderFullBoard() {
  if (!state.session) return;
  document.title = `${state.session.name} — RetroBoard`;
  document.getElementById('session-title').textContent = state.session.name;
  renderPhase();
  renderUsers();
  renderHostControls();
  renderAllColumns();
  renderActionsPanel();
  // If there is a focused card, reopen overlay
  if (state.session.focused_card_id) {
    const fc = findCard(state.session.focused_card_id);
    if (fc) openFocusOverlay(fc);
  }
  // Timer
  if (state.session.timer && state.session.timer.running) {
    startTimerDisplay(state.session.timer);
  }
}

function renderActionsPanel() {
  if (!state.session) return;
  const body = document.getElementById('actions-side-body');
  const countEl = document.getElementById('actions-side-count');
  const topbarCount = document.getElementById('topbar-action-count');
  if (!body) return;

  const allActions = [];
  (state.session.cards || []).forEach(card => {
    (card.actions || []).forEach(action => {
      allActions.push({ ...action, cardText: card.text, cardId: card.id });
    });
  });

  const pendingCount = allActions.filter(a => !a.done).length;
  if (countEl) {
    countEl.textContent = pendingCount;
    countEl.style.display = pendingCount > 0 ? 'inline' : 'none';
  }
  if (topbarCount) {
    topbarCount.textContent = pendingCount > 0 ? pendingCount : '';
  }

  if (allActions.length === 0) {
    body.innerHTML = '<p class="actions-side-empty">Sin accionables aún.<br>Abre una tarjeta para agregar.</p>';
    return;
  }

  allActions.sort((a, b) => (a.done === b.done ? 0 : a.done ? 1 : -1));

  body.innerHTML = allActions.map(a => `
    <div class="actions-side-item ${a.done ? 'done' : ''}" data-action-id="${escHtml(a.id)}" data-card-id="${escHtml(a.cardId)}">
      <div class="actions-side-card-ref" data-card-id="${escHtml(a.cardId)}">📌 ${escHtml(a.cardText)}</div>
      <div class="action-view" style="display:flex">
        <div class="action-details">
          <span class="action-text">${escHtml(a.text)}</span>
          ${a.assignee ? `<span class="action-assignee">→ ${escHtml(a.assignee)}</span>` : ''}
          ${a.due_date ? `<span class="action-due">📅 ${escHtml(a.due_date)}</span>` : ''}
        </div>
        <div class="action-side-btns">
          <button class="btn-icon action-edit-btn" title="Editar">✏️</button>
          <button class="btn-icon action-delete-btn" title="Eliminar">🗑</button>
        </div>
      </div>
      <div class="action-edit-form" style="display:none">
        <input type="text" class="action-edit-text" value="${escHtml(a.text)}" maxlength="300" />
        <input type="text" class="action-edit-assignee" placeholder="Responsable (opcional)" value="${escHtml(a.assignee || '')}" maxlength="80" />
        <input type="date" class="action-edit-due" value="${escHtml(a.due_date || '')}" />
        <div class="action-edit-btns">
          <button class="btn-primary action-save-btn" style="font-size:0.78rem;padding:5px 10px">Guardar</button>
          <button class="btn-ghost action-cancel-edit-btn" style="font-size:0.78rem;padding:5px 10px">Cancelar</button>
        </div>
      </div>
    </div>
  `).join('');

  body.querySelectorAll('.actions-side-item').forEach(item => {
    const actionId = item.dataset.actionId;
    const cardId = item.dataset.cardId;

    // Click on card ref → open focus overlay
    item.querySelector('.actions-side-card-ref').addEventListener('click', () => {
      const card = findCard(cardId);
      if (!card) return;
      if (isHost()) {
        focusCard(card.id).catch(err => console.error('focus error', err));
      } else {
        openFocusOverlay(card);
      }
    });

    // Edit button → show inline form
    item.querySelector('.action-edit-btn').addEventListener('click', () => {
      item.querySelector('.action-view').style.display = 'none';
      item.querySelector('.action-edit-form').style.display = 'flex';
      item.querySelector('.action-edit-text').focus();
    });

    // Cancel edit
    item.querySelector('.action-cancel-edit-btn').addEventListener('click', () => {
      item.querySelector('.action-view').style.display = 'flex';
      item.querySelector('.action-edit-form').style.display = 'none';
    });

    // Save edit
    const saveEdit = () => {
      const text = item.querySelector('.action-edit-text').value.trim();
      if (!text) return;
      const assignee = item.querySelector('.action-edit-assignee').value.trim();
      const dueDate = item.querySelector('.action-edit-due').value;
      editActionItem(actionId, text, assignee, dueDate)
        .catch(err => alert('Error al editar: ' + err.message));
      item.querySelector('.action-view').style.display = 'flex';
      item.querySelector('.action-edit-form').style.display = 'none';
    };
    item.querySelector('.action-save-btn').addEventListener('click', saveEdit);
    item.querySelector('.action-edit-text').addEventListener('keydown', e => {
      if (e.key === 'Enter') saveEdit();
      if (e.key === 'Escape') item.querySelector('.action-cancel-edit-btn').click();
    });

    // Delete button
    item.querySelector('.action-delete-btn').addEventListener('click', () => {
      if (confirm('¿Eliminar esta acción?')) {
        deleteActionItem(actionId)
          .catch(err => alert('Error al eliminar: ' + err.message));
      }
    });
  });
}

function renderPhase() {
  const badge = document.getElementById('phase-badge');
  const phase = state.session.phase;
  badge.textContent = PHASE_LABELS[phase] || phase;
  badge.style.background = PHASE_COLORS[phase] || '#555';

  // Update host phase buttons
  document.querySelectorAll('.phase-btn').forEach(btn => {
    btn.classList.toggle('active', btn.dataset.phase === phase);
  });
}

function renderUsers() {
  const users = state.session.users || [];
  document.getElementById('user-count').textContent = users.length;
  const pill = document.getElementById('users-pill');
  pill.title = users.map(u => u.username + (u.is_host ? ' (host)' : '')).join('\n');
}

function renderHostControls() {
  const ctrl = document.getElementById('host-controls');
  const addColZone = document.getElementById('add-column-zone');
  if (isHost()) {
    ctrl.style.display = 'flex';
    addColZone.style.display = 'flex';
  } else {
    ctrl.style.display = 'none';
    addColZone.style.display = 'none';
  }
}

function renderAllColumns() {
  if (!state.session) return;
  const wrapper = document.getElementById('columns-wrapper');
  const sorted = [...state.session.columns].sort((a, b) => a.order - b.order);
  // Remove columns that no longer exist
  const existingIDs = new Set(sorted.map(c => c.id));
  wrapper.querySelectorAll('.column').forEach(el => {
    if (!existingIDs.has(el.dataset.colId)) el.remove();
  });
  // Add or update columns
  sorted.forEach(col => {
    let colEl = wrapper.querySelector(`.column[data-col-id="${col.id}"]`);
    if (!colEl) {
      colEl = createColumnElement(col);
      wrapper.appendChild(colEl);
    }
    renderColumnCards(colEl, col);
  });
}

function renderColumn(colID) {
  if (!state.session) return;
  const col = state.session.columns.find(c => c.id === colID);
  const wrapper = document.getElementById('columns-wrapper');
  if (!col) {
    const el = wrapper.querySelector(`.column[data-col-id="${colID}"]`);
    if (el) el.remove();
    return;
  }
  let colEl = wrapper.querySelector(`.column[data-col-id="${col.id}"]`);
  if (!colEl) {
    colEl = createColumnElement(col);
    wrapper.appendChild(colEl);
  }
  renderColumnCards(colEl, col);
}

function createColumnElement(col) {
  const el = document.createElement('div');
  el.className = 'column';
  el.dataset.colId = col.id;
  el.innerHTML = `
    <div class="column-header" style="border-left-color: ${escHtml(col.color)}">
      <span class="column-title">${escHtml(col.name)}</span>
      ${isHost() ? `<button class="btn-icon delete-col-btn" data-col-id="${escHtml(col.id)}" title="Eliminar columna">🗑</button>` : ''}
    </div>
    <div class="add-card-form" style="display:${state.session.phase === 'adding' ? 'block' : 'none'}">
      <textarea class="add-card-textarea" placeholder="Escribe una tarjeta..." rows="3" maxlength="500"></textarea>
      <button class="btn-primary add-card-btn">Añadir tarjeta</button>
    </div>
    <div class="cards-list" data-col-id="${escHtml(col.id)}"></div>
  `;

  // Delete column button
  const delBtn = el.querySelector('.delete-col-btn');
  if (delBtn) {
    delBtn.addEventListener('click', () => {
      if (confirm('¿Eliminar esta columna y todas sus tarjetas?')) {
        deleteColumn(col.id).catch(err => alert('Error: ' + err.message));
      }
    });
  }

  // Add card
  const addBtn = el.querySelector('.add-card-btn');
  const textarea = el.querySelector('.add-card-textarea');
  addBtn.addEventListener('click', () => {
    const text = textarea.value.trim();
    if (!text) return;
    addCard(col.id, text).catch(err => alert('Error al añadir tarjeta: ' + err.message));
    textarea.value = '';
  });
  textarea.addEventListener('keydown', e => {
    if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
      addBtn.click();
    }
  });

  return el;
}

function renderColumnCards(colEl, col) {
  // Show/hide add form based on phase
  const addForm = colEl.querySelector('.add-card-form');
  if (addForm) addForm.style.display = state.session.phase === 'adding' ? 'block' : 'none';

  const list = colEl.querySelector('.cards-list');
  const cards = state.session.cards
    .filter(c => c.column_id === col.id)
    .sort((a, b) => (b.votes ? b.votes.length : 0) - (a.votes ? a.votes.length : 0));

  // Group cards by group_id
  const groupMap = {};
  const ungrouped = [];
  cards.forEach(card => {
    if (card.group_id) {
      if (!groupMap[card.group_id]) groupMap[card.group_id] = [];
      groupMap[card.group_id].push(card);
    } else {
      ungrouped.push(card);
    }
  });

  list.innerHTML = '';

  // Render groups
  Object.values(groupMap).forEach(groupCards => {
    const groupEl = document.createElement('div');
    groupEl.className = 'card-group';
    groupCards.forEach(card => {
      groupEl.appendChild(buildCardElement(card));
    });
    list.appendChild(groupEl);
  });

  // Render ungrouped
  ungrouped.forEach(card => {
    list.appendChild(buildCardElement(card));
  });
}

function buildCardElement(card) {
  const userID = state.currentUser ? state.currentUser.id : '';
  const hasVoted = card.votes && card.votes.includes(userID);
  const voteCount = card.votes ? card.votes.length : 0;
  const commentCount = card.comments ? card.comments.length : 0;
  const actionCount = card.actions ? card.actions.length : 0;
  const reactionSummary = buildReactionSummary(card.reactions || []);
  const canDelete = (card.author_id === userID || isHost());
  const canEdit = (card.author_id === userID || isHost());

  const el = document.createElement('div');
  el.className = 'card' + (hasVoted ? ' voted' : '');
  el.dataset.cardId = card.id;

  el.innerHTML = `
    <div class="card-text">${escHtml(card.text)}</div>
    <div class="card-meta">
      <span class="card-author">✍ ${escHtml(card.author)}</span>
      <div class="card-actions-row">
        ${state.session.phase === 'voting' || state.session.phase === 'discussing' ? `
        <button class="vote-btn ${hasVoted ? 'voted' : ''}" data-card-id="${card.id}" title="${hasVoted ? 'Quitar voto' : 'Votar'}">
          👍 ${voteCount}
        </button>` : `<span class="vote-count">👍 ${voteCount}</span>`}
        ${commentCount > 0 ? `<span class="meta-badge">💬 ${commentCount}</span>` : ''}
        ${actionCount > 0 ? `<span class="meta-badge">✅ ${actionCount}</span>` : ''}
        ${canEdit ? `<button class="btn-icon edit-card-btn" data-card-id="${card.id}" title="Editar">✏️</button>` : ''}
        ${canDelete ? `<button class="btn-icon delete-card-btn" data-card-id="${card.id}" title="Eliminar">🗑</button>` : ''}
        ${isHost() ? `<button class="btn-icon focus-card-btn" data-card-id="${card.id}" title="Enfocar">🔍</button>` : ''}
      </div>
    </div>
    ${reactionSummary ? `<div class="card-reactions">${reactionSummary}</div>` : ''}
  `;

  // Vote
  const voteBtn = el.querySelector('.vote-btn');
  if (voteBtn) {
    voteBtn.addEventListener('click', e => {
      e.stopPropagation();
      voteCard(card.id).catch(err => console.error('vote error', err));
    });
  }

  // Focus (host only)
  const focusBtn = el.querySelector('.focus-card-btn');
  if (focusBtn) {
    focusBtn.addEventListener('click', e => {
      e.stopPropagation();
      focusCard(card.id).catch(err => console.error('focus error', err));
    });
  }

  // Delete
  const delBtn = el.querySelector('.delete-card-btn');
  if (delBtn) {
    delBtn.addEventListener('click', e => {
      e.stopPropagation();
      if (confirm('¿Eliminar esta tarjeta?')) {
        deleteCard(card.id).catch(err => alert('Error: ' + err.message));
      }
    });
  }

  // Edit
  const editBtn = el.querySelector('.edit-card-btn');
  if (editBtn) {
    editBtn.addEventListener('click', e => {
      e.stopPropagation();
      const newText = prompt('Editar tarjeta:', card.text);
      if (newText && newText.trim() && newText.trim() !== card.text) {
        updateCard(card.id, newText.trim()).catch(err => alert('Error: ' + err.message));
      }
    });
  }

  // Click card → open focus overlay
  el.addEventListener('click', () => {
    if (isHost()) {
      focusCard(card.id).catch(err => console.error('focus error', err));
    } else {
      // Non-host users open a local view
      openFocusOverlay(card);
    }
  });

  return el;
}

function renderCardElement(cardID) {
  const card = findCard(cardID);
  if (!card) return;
  const oldEl = document.querySelector(`.card[data-card-id="${cardID}"]`);
  if (!oldEl) return;
  const newEl = buildCardElement(card);
  oldEl.parentNode.replaceChild(newEl, oldEl);
}

function buildReactionSummary(reactions) {
  if (!reactions || reactions.length === 0) return '';
  const counts = {};
  reactions.forEach(r => {
    counts[r.emoji] = (counts[r.emoji] || 0) + 1;
  });
  return Object.entries(counts)
    .map(([emoji, count]) => `<span class="reaction-pill">${emoji} ${count}</span>`)
    .join('');
}

// ─── Focus Overlay ────────────────────────────────────────────────────────────
function openFocusOverlay(card) {
  state.focused = card;
  const overlay = document.getElementById('focus-overlay');

  document.getElementById('focus-card-text').textContent = card.text;
  document.getElementById('focus-author').textContent = `✍ ${card.author}`;

  renderFocusVote();
  renderReactions();
  renderComments();
  renderActions();

  overlay.style.display = 'flex';
  requestAnimationFrame(() => overlay.classList.add('visible'));
}

function closeFocusOverlay() {
  const overlay = document.getElementById('focus-overlay');
  overlay.classList.remove('visible');
  setTimeout(() => {
    overlay.style.display = 'none';
    state.focused = null;
  }, 300);
}

function renderFocusVote() {
  const card = state.focused;
  if (!card) return;
  const userID = state.currentUser ? state.currentUser.id : '';
  const hasVoted = card.votes && card.votes.includes(userID);
  const btn = document.getElementById('focus-vote-btn');
  btn.textContent = hasVoted ? `👍 ${card.votes.length} (votado)` : `👍 ${card.votes ? card.votes.length : 0} Votar`;
  btn.className = 'vote-btn' + (hasVoted ? ' voted' : '');
}

function renderReactions() {
  const card = state.focused;
  if (!card) return;
  const userID = state.currentUser ? state.currentUser.id : '';
  const display = document.getElementById('reactions-display');
  const reactions = card.reactions || [];

  const counts = {};
  const userReacted = {};
  reactions.forEach(r => {
    counts[r.emoji] = (counts[r.emoji] || 0) + 1;
    if (r.user_id === userID) userReacted[r.emoji] = true;
  });

  if (Object.keys(counts).length === 0) {
    display.innerHTML = '<span class="no-reactions">Sin reacciones aún</span>';
    return;
  }

  display.innerHTML = Object.entries(counts)
    .map(([emoji, count]) => `
      <span class="reaction-pill large ${userReacted[emoji] ? 'mine' : ''}" data-emoji="${escHtml(emoji)}" title="${userReacted[emoji] ? 'Quitar reacción' : 'Reaccionar'}">
        ${emoji} <strong>${count}</strong>
      </span>
    `).join('');

  display.querySelectorAll('.reaction-pill').forEach(pill => {
    pill.addEventListener('click', () => {
      if (state.focused) {
        addReaction(state.focused.id, pill.dataset.emoji).catch(err => console.error('reaction error', err));
      }
    });
  });
}

function renderComments() {
  const card = state.focused;
  if (!card) return;
  const list = document.getElementById('comments-list');
  const comments = card.comments || [];
  if (comments.length === 0) {
    list.innerHTML = '<p class="empty-text">Sin comentarios aún.</p>';
    return;
  }
  list.innerHTML = comments.map(c => `
    <div class="comment-item">
      <div class="comment-header">
        <strong>${escHtml(c.username)}</strong>
        <span class="comment-time">${formatTime(c.created_at)}</span>
      </div>
      <div class="comment-text">${escHtml(c.text)}</div>
    </div>
  `).join('');
  list.scrollTop = list.scrollHeight;
}

function renderActions() {
  const card = state.focused;
  if (!card) return;
  const list = document.getElementById('actions-list');
  const actions = card.actions || [];
  if (actions.length === 0) {
    list.innerHTML = '<p class="empty-text">Sin acciones aún.</p>';
    return;
  }
  list.innerHTML = actions.map(a => `
    <div class="action-item ${a.done ? 'done' : ''}">
      <input type="checkbox" class="action-checkbox" data-card-id="${escHtml(card.id)}" data-action-id="${escHtml(a.id)}" data-done="${a.done}" ${a.done ? 'checked' : ''} />
      <div class="action-details">
        <span class="action-text">${escHtml(a.text)}</span>
        ${a.assignee ? `<span class="action-assignee">→ ${escHtml(a.assignee)}</span>` : ''}
        ${a.due_date ? `<span class="action-due">📅 ${escHtml(a.due_date)}</span>` : ''}
      </div>
    </div>
  `).join('');

  list.querySelectorAll('.action-checkbox').forEach(cb => {
    cb.addEventListener('change', () => {
      const done = cb.dataset.done === 'true';
      toggleActionDone(cb.dataset.cardId, cb.dataset.actionId, done)
        .catch(err => console.error('toggle action error', err));
    });
  });
}

function formatTime(ts) {
  if (!ts) return '';
  const d = new Date(ts);
  if (isNaN(d)) return '';
  return d.toLocaleTimeString('es', { hour: '2-digit', minute: '2-digit' });
}

// ─── Timer ────────────────────────────────────────────────────────────────────
function startTimerDisplay(timer) {
  stopTimerDisplay();
  const display = document.getElementById('timer-display');
  const countdown = document.getElementById('timer-countdown');
  display.style.display = 'flex';

  const startedAt = new Date(timer.started_at).getTime();
  const duration = timer.duration * 1000;

  function tick() {
    const now = Date.now();
    const elapsed = now - startedAt;
    const remaining = Math.max(0, duration - elapsed);
    const secs = Math.floor(remaining / 1000);
    const m = Math.floor(secs / 60).toString().padStart(2, '0');
    const s = (secs % 60).toString().padStart(2, '0');
    countdown.textContent = `${m}:${s}`;
    if (remaining <= 10000) countdown.classList.add('urgent');
    else countdown.classList.remove('urgent');
    if (remaining <= 0) {
      countdown.textContent = '⏰ Tiempo!';
      stopTimerDisplay();
    }
  }
  tick();
  state.timerInterval = setInterval(tick, 500);

  // Show stop button for host
  if (isHost()) {
    document.getElementById('stop-timer-btn').style.display = 'inline-flex';
    document.getElementById('start-timer-btn').style.display = 'none';
  }
}

function stopTimerDisplay() {
  if (state.timerInterval) {
    clearInterval(state.timerInterval);
    state.timerInterval = null;
  }
  document.getElementById('timer-display').style.display = 'none';
  if (isHost()) {
    document.getElementById('stop-timer-btn').style.display = 'none';
    document.getElementById('start-timer-btn').style.display = 'inline-flex';
  }
}

// ─── Action Senders (replacing sendWS) ───────────────────────────────────────
function addCard(columnId, text) {
  return api('cards', 'POST', {
    session_id: state.session.id,
    column_id: columnId,
    text,
    author: state.currentUser.username,
    author_id: state.currentUser.id,
  });
}

function voteCard(cardId) {
  return api('votes', 'POST', {
    card_id: cardId,
    session_id: state.session.id,
    user_id: state.currentUser.id,
  });
}

function focusCard(cardId) {
  return api('sessions', 'PUT', { id: state.session.id, focused_card_id: cardId });
}

function closeFocusRemote() {
  return api('sessions', 'PUT', { id: state.session.id, focused_card_id: null });
}

function addComment(cardId, text) {
  return api('comments', 'POST', {
    card_id: cardId,
    session_id: state.session.id,
    text,
    user_id: state.currentUser.id,
    username: state.currentUser.username,
  });
}

function addReaction(cardId, emoji) {
  return api('reactions', 'POST', {
    card_id: cardId,
    session_id: state.session.id,
    emoji,
    user_id: state.currentUser.id,
    username: state.currentUser.username,
  });
}

function addAction(cardId, text, assignee, dueDate) {
  return api('actions', 'POST', {
    card_id: cardId,
    session_id: state.session.id,
    text,
    assignee: assignee || '',
    due_date: dueDate || '',
  });
}

function toggleActionDone(cardId, actionId, currentDone) {
  return api('actions', 'PUT', { id: actionId, done: !currentDone });
}

function editActionItem(actionId, text, assignee, dueDate) {
  return api('actions', 'PUT', { id: actionId, text, assignee: assignee || '', due_date: dueDate || '' });
}

function deleteActionItem(actionId) {
  return api(`actions?id=${actionId}&sessionId=${state.session.id}`, 'DELETE');
}

function deleteSessionAction() {
  return api(`sessions?id=${state.session.id}`, 'DELETE');
}

function moveCard(cardId, columnId) {
  return api('cards', 'PUT', { id: cardId, column_id: columnId });
}

function groupCards(cardIds, groupId) {
  const gid = groupId || crypto.randomUUID();
  return Promise.all(cardIds.map(id => api('cards', 'PUT', { id, group_id: gid })));
}

function ungroupCard(cardId) {
  return api('cards', 'PUT', { id: cardId, group_id: null });
}

function deleteCard(cardId) {
  return api(`cards?id=${cardId}&userId=${state.currentUser.id}&sessionId=${state.session.id}`, 'DELETE');
}

function updateCard(cardId, text) {
  return api('cards', 'PUT', { id: cardId, text });
}

function changePhase(phase) {
  return api('sessions', 'PUT', { id: state.session.id, phase });
}

function startTimer(duration) {
  return api('timer', 'PUT', { session_id: state.session.id, action: 'start', duration });
}

function stopTimer() {
  return api('timer', 'PUT', { session_id: state.session.id, action: 'stop' });
}

function addColumn(name, color) {
  return api('columns', 'POST', { session_id: state.session.id, name, color });
}

function deleteColumn(columnId) {
  return api(`columns?id=${columnId}&sessionId=${state.session.id}`, 'DELETE');
}

// ─── Event Listeners ──────────────────────────────────────────────────────────
function setupEventListeners() {
  // Copy link
  document.getElementById('copy-link-btn').addEventListener('click', () => {
    navigator.clipboard.writeText(location.href).then(() => {
      const btn = document.getElementById('copy-link-btn');
      btn.textContent = '✅ Copiado!';
      setTimeout(() => { btn.textContent = '🔗 Copiar link'; }, 2000);
    });
  });

  // Export dropdown
  const exportBtn = document.getElementById('export-btn');
  const exportMenu = document.getElementById('export-menu');
  exportBtn.addEventListener('click', e => {
    e.stopPropagation();
    exportMenu.classList.toggle('open');
  });
  document.addEventListener('click', () => exportMenu.classList.remove('open'));

  document.getElementById('export-json').addEventListener('click', e => {
    e.preventDefault();
    if (!state.session) return;
    window.open(`/api/export?sessionId=${state.session.id}&format=json`, '_blank');
    exportMenu.classList.remove('open');
  });
  document.getElementById('export-md').addEventListener('click', e => {
    e.preventDefault();
    if (!state.session) return;
    window.open(`/api/export?sessionId=${state.session.id}&format=markdown`, '_blank');
    exportMenu.classList.remove('open');
  });

  // Host: phase buttons
  document.querySelectorAll('.phase-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      changePhase(btn.dataset.phase).catch(err => alert('Error: ' + err.message));
    });
  });

  // Host: timer
  document.getElementById('start-timer-btn').addEventListener('click', () => {
    const dur = parseInt(document.getElementById('timer-duration').value, 10);
    startTimer(dur).catch(err => alert('Error: ' + err.message));
  });
  document.getElementById('stop-timer-btn').addEventListener('click', () => {
    stopTimer().catch(err => alert('Error: ' + err.message));
  });

  // Host: add column
  document.getElementById('show-add-col-btn').addEventListener('click', () => {
    document.getElementById('add-column-form').style.display = 'flex';
    document.getElementById('show-add-col-btn').style.display = 'none';
    document.getElementById('new-col-name').focus();
  });
  document.getElementById('cancel-add-col').addEventListener('click', () => {
    document.getElementById('add-column-form').style.display = 'none';
    document.getElementById('show-add-col-btn').style.display = 'inline-flex';
  });
  document.getElementById('confirm-add-col').addEventListener('click', () => {
    const name = document.getElementById('new-col-name').value.trim();
    const color = document.getElementById('new-col-color').value;
    if (!name) return;
    addColumn(name, color).catch(err => alert('Error: ' + err.message));
    document.getElementById('new-col-name').value = '';
    document.getElementById('add-column-form').style.display = 'none';
    document.getElementById('show-add-col-btn').style.display = 'inline-flex';
  });

  // Focus overlay: close button
  document.getElementById('focus-close-btn').addEventListener('click', () => {
    if (isHost()) {
      closeFocusRemote().catch(err => console.error('close focus error', err));
    } else {
      closeFocusOverlay();
    }
  });

  // Focus overlay: vote
  document.getElementById('focus-vote-btn').addEventListener('click', () => {
    if (state.focused) {
      voteCard(state.focused.id).catch(err => console.error('vote error', err));
    }
  });

  // Focus overlay: emoji picker
  document.querySelectorAll('.emoji-option').forEach(opt => {
    opt.addEventListener('click', () => {
      if (state.focused) {
        addReaction(state.focused.id, opt.dataset.emoji).catch(err => console.error('reaction error', err));
      }
    });
  });

  // Focus overlay: add comment
  document.getElementById('submit-comment-btn').addEventListener('click', submitComment);
  document.getElementById('comment-input').addEventListener('keydown', e => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      submitComment();
    }
  });

  // Focus overlay: add action
  document.getElementById('submit-action-btn').addEventListener('click', submitAction);

  // Close overlay on background click
  document.getElementById('focus-overlay').addEventListener('click', e => {
    if (e.target === document.getElementById('focus-overlay')) {
      if (isHost()) {
        closeFocusRemote().catch(err => console.error('close focus error', err));
      } else {
        closeFocusOverlay();
      }
    }
  });

  // ESC key closes overlay
  document.addEventListener('keydown', e => {
    if (e.key === 'Escape' && state.focused) {
      if (isHost()) {
        closeFocusRemote().catch(err => console.error('close focus error', err));
      } else {
        closeFocusOverlay();
      }
    }
  });

  // Actions side panel toggle (button inside panel)
  document.getElementById('actions-side-toggle').addEventListener('click', () => {
    toggleActionsPanel();
  });

  // Topbar panel toggle button
  document.getElementById('toggle-panel-btn').addEventListener('click', () => {
    toggleActionsPanel();
  });

  // Delete session (host only)
  document.getElementById('delete-session-btn').addEventListener('click', () => {
    if (!state.session) return;
    if (!confirm(`¿Eliminar la sesión "${state.session.name}" y todos sus datos? Esta acción no se puede deshacer.`)) return;
    deleteSessionAction()
      .then(() => { window.location.href = '/'; })
      .catch(err => alert('Error al eliminar: ' + err.message));
  });
}

function toggleActionsPanel() {
  const panel = document.getElementById('actions-side-panel');
  const toggleBtn = document.getElementById('actions-side-toggle');
  panel.classList.toggle('collapsed');
  const isCollapsed = panel.classList.contains('collapsed');
  if (toggleBtn) toggleBtn.textContent = isCollapsed ? '‹' : '›';
}

function submitComment() {
  if (!state.focused) return;
  const input = document.getElementById('comment-input');
  const text = input.value.trim();
  if (!text) return;
  addComment(state.focused.id, text).catch(err => alert('Error al comentar: ' + err.message));
  input.value = '';
}

function submitAction() {
  if (!state.focused) return;
  const text = document.getElementById('action-text-input').value.trim();
  const assignee = document.getElementById('action-assignee-input').value.trim();
  const dueDate = document.getElementById('action-due-input').value;
  if (!text) return;
  addAction(state.focused.id, text, assignee, dueDate)
    .then(() => renderActionsPanel())
    .catch(err => alert('Error al agregar acción: ' + err.message));
  document.getElementById('action-text-input').value = '';
  document.getElementById('action-assignee-input').value = '';
  document.getElementById('action-due-input').value = '';
}

// ─── Bootstrap ────────────────────────────────────────────────────────────────
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', init);
} else {
  init();
}
