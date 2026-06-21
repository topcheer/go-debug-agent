package debugagent

import "fmt"

// renderChatPage renders the chat UI HTML with the given base path.
func renderChatPage(basePath string) string {
	html := chatHTMLTemplate
	return fmt.Sprintf(html, basePath)
}

// chatHTMLTemplate is the dark-themed chat UI.
// All literal % characters are escaped as %% because fmt.Sprintf is used.
// The single %s is replaced with the base path at runtime.
// Backtick characters are avoided inside the raw string literal; instead the JS
// uses String.fromCharCode(96) and new RegExp() to construct backtick-based patterns.
const chatHTMLTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Go Debug Agent</title>
<style>
  :root {
    --bg: #0d1117;
    --surface: #161b22;
    --surface-hover: #1c2128;
    --border: #30363d;
    --text: #e6edf3;
    --text-muted: #8b949e;
    --accent: #58a6ff;
    --accent-hover: #79c0ff;
    --green: #3fb950;
    --orange: #d29922;
    --red: #f85149;
    --purple: #bc8cff;
    --max-width: 880px;
    --code-bg: #010409;
  }
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body {
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Helvetica, Arial, sans-serif;
    background: var(--bg);
    color: var(--text);
    height: 100vh;
    display: flex;
    flex-direction: column;
  }
  .header {
    border-bottom: 1px solid var(--border);
    padding: 12px 24px;
    display: flex;
    align-items: center;
    justify-content: space-between;
    background: var(--surface);
    flex-shrink: 0;
  }
  .header-title {
    font-size: 15px; font-weight: 600;
    display: flex; align-items: center; gap: 8px;
  }
  .header-title .dot {
    width: 8px; height: 8px; border-radius: 50%%;
    background: var(--green); box-shadow: 0 0 6px var(--green);
  }
  .header-info { font-size: 12px; color: var(--text-muted); }
  .chat-container { flex: 1; overflow-y: auto; padding: 24px; }
  .chat-messages {
    max-width: var(--max-width); margin: 0 auto;
    display: flex; flex-direction: column; gap: 16px;
  }
  .message { display: flex; gap: 12px; animation: fadeIn 0.2s ease; }
  @keyframes fadeIn {
    from { opacity: 0; transform: translateY(8px); }
    to { opacity: 1; transform: translateY(0); }
  }
  .message-avatar {
    width: 32px; height: 32px; border-radius: 6px;
    display: flex; align-items: center; justify-content: center;
    font-size: 14px; flex-shrink: 0; font-weight: 600;
  }
  .message.user .message-avatar { background: var(--accent); color: #fff; }
  .message.agent .message-avatar { background: #6e40c9; color: #fff; }
  .message-content {
    flex: 1; line-height: 1.65; font-size: 14px;
    overflow-wrap: break-word; word-break: break-word;
  }
  .message.agent .message-content {
    background: var(--surface); border: 1px solid var(--border);
    border-radius: 8px; padding: 16px 20px;
  }

  .tool-badges {
    max-width: var(--max-width); margin: 0 auto;
    display: flex; flex-wrap: wrap; gap: 6px;
  }
  .tool-badge {
    display: inline-flex; align-items: center; gap: 5px;
    padding: 3px 10px; border-radius: 12px;
    font-size: 12px; font-family: 'SF Mono', Monaco, monospace;
    border: 1px solid var(--border); background: var(--surface);
    color: var(--text-muted); line-height: 1.4;
    animation: fadeIn 0.2s ease;
  }
  .tool-badge.pending { border-color: var(--orange); color: var(--orange); }
  .tool-badge.success { border-color: rgba(63,185,80,0.4); color: var(--green); }
  .tool-badge.error { border-color: rgba(248,81,73,0.4); color: var(--red); }
  .tool-badge .dot-icon {
    width: 7px; height: 7px; border-radius: 50%%; flex-shrink: 0;
  }
  .tool-badge.pending .dot-icon {
    background: var(--orange);
    animation: spinFade 0.8s linear infinite;
  }
  .tool-badge.success .dot-icon { background: var(--green); }
  .tool-badge.error .dot-icon { background: var(--red); }
  @keyframes spinFade {
    0%% { opacity: 1; } 50%% { opacity: 0.3; } 100%% { opacity: 1; }
  }

  .message-content > *:first-child { margin-top: 0; }
  .message-content > *:last-child { margin-bottom: 0; }
  .md p { margin: 0 0 10px; }
  .md h1, .md h2, .md h3, .md h4, .md h5, .md h6 {
    margin: 18px 0 8px; font-weight: 600; line-height: 1.3;
  }
  .md h1 { font-size: 1.4em; border-bottom: 1px solid var(--border); padding-bottom: 6px; }
  .md h2 { font-size: 1.25em; border-bottom: 1px solid var(--border); padding-bottom: 5px; }
  .md h3 { font-size: 1.1em; }
  .md h4 { font-size: 1em; color: var(--text-muted); }
  .md ul, .md ol { margin: 6px 0 10px; padding-left: 22px; }
  .md li { margin-bottom: 3px; }
  .md li > ul, .md li > ol { margin: 3px 0; }
  .md code {
    font-family: 'SF Mono', Monaco, 'Cascadia Code', monospace;
    font-size: 0.88em; background: rgba(110,118,129,0.25);
    padding: 2px 5px; border-radius: 4px;
  }
  .md pre {
    background: var(--code-bg); border: 1px solid var(--border);
    border-radius: 8px; overflow: hidden; margin: 10px 0; position: relative;
  }
  .md pre .code-header {
    display: flex; align-items: center; justify-content: space-between;
    padding: 5px 12px; background: rgba(255,255,255,0.04);
    border-bottom: 1px solid var(--border); font-size: 11px;
    color: var(--text-muted); font-family: 'SF Mono', Monaco, monospace;
  }
  .md pre .copy-btn {
    background: none; border: 1px solid var(--border); border-radius: 4px;
    color: var(--text-muted); padding: 1px 8px; font-size: 11px; cursor: pointer;
    transition: all 0.15s;
  }
  .md pre .copy-btn:hover { background: var(--surface-hover); color: var(--text); }
  .md pre code {
    display: block; background: none; padding: 10px 14px;
    font-size: 13px; line-height: 1.5; overflow-x: auto;
  }
  .md blockquote {
    margin: 8px 0; padding: 6px 14px; border-left: 3px solid var(--accent);
    background: rgba(88,166,255,0.06); color: var(--text-muted);
    border-radius: 0 6px 6px 0;
  }
  .md blockquote p { margin: 0; }
  .md table {
    border-collapse: collapse; width: 100%%; margin: 10px 0;
    font-size: 13px; display: block; overflow-x: auto;
  }
  .md thead { background: rgba(255,255,255,0.04); }
  .md th, .md td { border: 1px solid var(--border); padding: 5px 12px; text-align: left; }
  .md th { font-weight: 600; white-space: nowrap; }
  .md tr:nth-child(even) { background: rgba(255,255,255,0.02); }
  .md hr { border: none; border-top: 1px solid var(--border); margin: 14px 0; }
  .md a { color: var(--accent); text-decoration: none; }
  .md a:hover { text-decoration: underline; }
  .md strong { font-weight: 600; }
  .md em { font-style: italic; }

  .typing { display: inline-flex; gap: 4px; padding: 4px 0; }
  .typing span {
    width: 6px; height: 6px; background: var(--text-muted);
    border-radius: 50%%; animation: bounce 1.4s ease infinite;
  }
  .typing span:nth-child(2) { animation-delay: 0.2s; }
  .typing span:nth-child(3) { animation-delay: 0.4s; }
  @keyframes bounce {
    0%%, 60%%, 100%% { transform: translateY(0); opacity: 0.4; }
    30%% { transform: translateY(-6px); opacity: 1; }
  }

  .input-area {
    border-top: 1px solid var(--border); padding: 16px 24px;
    background: var(--surface); flex-shrink: 0;
  }
  .input-wrapper {
    max-width: var(--max-width); margin: 0 auto;
    display: flex; gap: 8px; align-items: flex-end;
  }
  .input-field {
    flex: 1; background: var(--bg); border: 1px solid var(--border);
    border-radius: 8px; padding: 10px 14px; color: var(--text);
    font-size: 14px; font-family: inherit; resize: none;
    max-height: 120px; min-height: 42px; line-height: 1.5;
    transition: border-color 0.15s;
  }
  .input-field:focus { outline: none; border-color: var(--accent); }
  .input-field::placeholder { color: var(--text-muted); }
  .send-btn {
    background: var(--accent); color: #fff; border: none; border-radius: 8px;
    padding: 10px 18px; font-size: 14px; font-weight: 500; cursor: pointer;
    transition: background 0.15s; white-space: nowrap;
  }
  .send-btn:hover:not(:disabled) { background: var(--accent-hover); }
  .send-btn:disabled { opacity: 0.5; cursor: not-allowed; }

  ::-webkit-scrollbar { width: 6px; height: 6px; }
  ::-webkit-scrollbar-track { background: var(--bg); }
  ::-webkit-scrollbar-thumb { background: var(--border); border-radius: 3px; }
  ::-webkit-scrollbar-thumb:hover { background: var(--text-muted); }

  .error-banner {
    max-width: var(--max-width); margin: 0 auto; padding: 8px 12px;
    background: rgba(248, 81, 73, 0.1); border: 1px solid var(--red);
    border-radius: 6px; font-size: 13px; color: var(--red);
  }
  .system-notice {
    max-width: var(--max-width); margin: 0 auto; padding: 8px 12px;
    background: rgba(210, 153, 34, 0.08); border: 1px solid var(--orange);
    border-radius: 6px; font-size: 12px; color: var(--orange); text-align: center;
  }
</style>
</head>
<body>
  <div class="header">
    <div class="header-title">
      <span class="dot"></span>
      Go Debug Agent
    </div>
    <div class="header-info">
      Connected to live runtime &middot; <a href="#" id="clear-link" style="color:var(--text-muted);text-decoration:none;">Clear</a>
    </div>
  </div>

  <div class="chat-container" id="chat-container">
    <div class="chat-messages" id="chat-messages">
      <div class="message agent">
        <div class="message-avatar">AI</div>
        <div class="message-content md"><p>Hi! I'm your <strong>Go Debug Agent</strong>.</p><p>I can inspect goroutines, memory, GC, HTTP requests, build info, database pools, network, and more. What's the issue you're debugging?</p></div>
      </div>
    </div>
  </div>

  <div class="input-area">
    <div class="input-wrapper">
      <textarea class="input-field" id="input" placeholder="Describe the issue... (Shift+Enter for newline)" rows="1"></textarea>
      <button class="send-btn" id="send">Send</button>
    </div>
  </div>

<script>
// BT = backtick character (avoided inside Go raw string literal)
var BT = String.fromCharCode(96);
var tripleBT = BT + BT + BT;

const sessionId = 'session-' + Math.random().toString(36).substring(2, 11);
const BASE_PATH = '%s';

const chatMessages = document.getElementById('chat-messages');
const chatContainer = document.getElementById('chat-container');
const input = document.getElementById('input');
const sendBtn = document.getElementById('send');
const clearLink = document.getElementById('clear-link');

let isStreaming = false;
let currentAgentContent = null;
let renderTimer = null;
let toolBadgesContainer = null;

input.addEventListener('input', () => {
  input.style.height = 'auto';
  input.style.height = Math.min(input.scrollHeight, 120) + 'px';
});
input.addEventListener('keydown', (e) => {
  if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); sendMessage(); }
});
sendBtn.addEventListener('click', sendMessage);
clearLink.addEventListener('click', (e) => {
  e.preventDefault();
  if (confirm('Clear conversation history?')) {
    fetch(BASE_PATH + '/api/clear', {
      method: 'POST', headers: {'Content-Type':'application/json'},
      body: JSON.stringify({sessionId: sessionId})
    }).then(() => {
      chatMessages.innerHTML = '';
      addAgentMessage('Conversation cleared. What can I help you debug?');
    });
  }
});

function autoScroll() {
  chatContainer.scrollTop = chatContainer.scrollHeight;
}

function escapeHtml(text) {
  const div = document.createElement('div');
  div.textContent = text;
  return div.innerHTML;
}

function renderMarkdown(text) {
  if (!text) return '';
  const codeBlocks = [];
  var reCodeBlock = new RegExp(tripleBT + '(\\w*)\\n([\\s\\S]*?)' + tripleBT, 'g');
  let processed = text.replace(reCodeBlock, (m, lang, code) => {
    const idx = codeBlocks.length;
    codeBlocks.push({ lang: lang || '', code: code.replace(/\n$/, '') });
    return '\x00CODEBLOCK' + idx + '\x00';
  });

  processed = escapeHtml(processed);

  const lines = processed.split('\n');
  const result = [];
  let inList = false, listType = '';
  let inTable = false, tableRows = [];
  let inBlockquote = false;

  function closeList() { if (inList) { result.push('</' + listType + '>'); inList = false; } }
  function closeTable() { if (inTable) { result.push(renderTable(tableRows)); tableRows = []; inTable = false; } }
  function closeBlockquote() { if (inBlockquote) { result.push('</blockquote>'); inBlockquote = false; } }

  for (let i = 0; i < lines.length; i++) {
    let line = lines[i];
    if (/^\x00CODEBLOCK\d+\x00$/.test(line.trim())) { closeList(); closeTable(); closeBlockquote(); result.push(line.trim()); continue; }
    const headingMatch = line.match(/^(#{1,6})\s+(.+)$/);
    if (headingMatch) { closeList(); closeTable(); closeBlockquote(); const level = headingMatch[1].length; result.push('<h' + level + '>' + renderInline(headingMatch[2]) + '</h' + level + '>'); continue; }
    if (/^(-{3,}|\*{3,}|_{3,})\s*$/.test(line)) { closeList(); closeTable(); closeBlockquote(); result.push('<hr>'); continue; }
    const bqMatch = line.match(/^>\s?(.*)$/);
    if (bqMatch) { closeList(); closeTable(); if (!inBlockquote) { result.push('<blockquote>'); inBlockquote = true; } result.push('<p>' + renderInline(bqMatch[1]) + '</p>'); continue; } else { closeBlockquote(); }
    if (/^\|.*\|\s*$/.test(line.trim())) { closeList(); if (/^\|[\s:|-]+\|$/.test(line.trim())) continue; if (!inTable) inTable = true; tableRows.push(line.trim()); continue; } else { closeTable(); }
    const olMatch = line.match(/^\d+\.\s+(.+)$/);
    if (olMatch) { if (!inList || listType !== 'ol') { closeList(); result.push('<ol>'); inList = true; listType = 'ol'; } result.push('<li>' + renderInline(olMatch[1]) + '</li>'); continue; }
    const ulMatch = line.match(/^[-*+]\s+(.+)$/);
    if (ulMatch) { if (!inList || listType !== 'ul') { closeList(); result.push('<ul>'); inList = true; listType = 'ul'; } result.push('<li>' + renderInline(ulMatch[1]) + '</li>'); continue; }
    if (line.trim() === '') { closeList(); closeTable(); closeBlockquote(); continue; }
    closeList(); closeTable(); closeBlockquote();
    result.push('<p>' + renderInline(line) + '</p>');
  }
  closeList(); closeTable(); closeBlockquote();

  let html = result.join('\n');
  html = html.replace(/\x00CODEBLOCK(\d+)\x00/g, (m, idx) => {
    const block = codeBlocks[parseInt(idx)];
    return wrapCodeBlock(block.lang, block.code);
  });
  return html;
}

function renderTable(rows) {
  if (rows.length < 1) return '';
  const parseRow = (row) => row.replace(/^\||\|$/g, '').split('|').map(c => c.trim());
  let html = '<table><thead><tr>';
  const headerCells = parseRow(rows[0]);
  for (const cell of headerCells) html += '<th>' + renderInline(cell) + '</th>';
  html += '</tr></thead><tbody>';
  for (let i = 1; i < rows.length; i++) {
    html += '<tr>';
    for (const cell of parseRow(rows[i])) html += '<td>' + renderInline(cell) + '</td>';
    html += '</tr>';
  }
  html += '</tbody></table>';
  return html;
}

function wrapCodeBlock(lang, code) {
  const langLabel = lang ? lang.toUpperCase() : 'CODE';
  return '<pre><div class="code-header"><span>' + escapeHtml(langLabel) + '</span>'
    + '<button class="copy-btn" onclick="copyCode(this)">Copy</button></div>'
    + '<code>' + escapeHtml(code) + '</code></pre>';
}

function renderInline(text) {
  let t = text;
  t = t.replace(new RegExp(BT + '([^' + BT + ']+)' + BT, 'g'), '<code>$1</code>');
  t = t.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
  t = t.replace(/__([^_]+)__/g, '<strong>$1</strong>');
  t = t.replace(/(?<!\*)\*([^*]+)\*(?!\*)/g, '<em>$1</em>');
  t = t.replace(/(?<!_)_([^_]+)_(?!_)/g, '<em>$1</em>');
  t = t.replace(/~~([^~]+)~~/g, '<del>$1</del>');
  t = t.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank" rel="noopener">$1</a>');
  return t;
}

function copyCode(btn) {
  const codeEl = btn.closest('pre').querySelector('code');
  navigator.clipboard.writeText(codeEl.textContent).then(() => {
    btn.textContent = 'Copied!';
    setTimeout(() => { btn.textContent = 'Copy'; }, 2000);
  }).catch(() => { btn.textContent = 'Failed'; setTimeout(() => { btn.textContent = 'Copy'; }, 2000); });
}

function addUserMessage(text) {
  const div = document.createElement('div');
  div.className = 'message user';
  div.innerHTML = '<div class="message-avatar">U</div><div class="message-content">' + escapeHtml(text) + '</div>';
  chatMessages.appendChild(div);
  autoScroll();
}

function addAgentMessage(text) {
  const div = document.createElement('div');
  div.className = 'message agent';
  div.innerHTML = '<div class="message-avatar">AI</div><div class="message-content md">' + renderMarkdown(text) + '</div>';
  chatMessages.appendChild(div);
  autoScroll();
  return div.querySelector('.message-content');
}

function startAgentStreaming() {
  const div = document.createElement('div');
  div.className = 'message agent';
  div.innerHTML = '<div class="message-avatar">AI</div><div class="message-content md"><div class="typing"><span></span><span></span><span></span></div></div>';
  chatMessages.appendChild(div);
  autoScroll();
  currentAgentContent = div.querySelector('.message-content');
  currentAgentContent._rawText = '';
  return currentAgentContent;
}

function appendContentChunk(chunk) {
  if (!currentAgentContent) return;
  currentAgentContent._rawText += chunk;
  if (renderTimer) return;
  renderTimer = setTimeout(() => {
    renderTimer = null;
    if (currentAgentContent && currentAgentContent._rawText) {
      currentAgentContent.innerHTML = renderMarkdown(currentAgentContent._rawText);
      autoScroll();
    }
  }, 50);
}

function finalizeAgentContent() {
  if (renderTimer) { clearTimeout(renderTimer); renderTimer = null; }
  if (currentAgentContent && currentAgentContent._rawText) {
    currentAgentContent.innerHTML = renderMarkdown(currentAgentContent._rawText);
    autoScroll();
  }
}

function ensureToolBadgesContainer() {
  if (!toolBadgesContainer) {
    toolBadgesContainer = document.createElement('div');
    toolBadgesContainer.className = 'tool-badges';
    chatMessages.appendChild(toolBadgesContainer);
  }
  return toolBadgesContainer;
}

function addToolBadge(toolName) {
  const container = ensureToolBadgesContainer();
  const badge = document.createElement('span');
  badge.className = 'tool-badge pending';
  badge.dataset.tool = toolName;
  badge.innerHTML = '<span class="dot-icon"></span>' + escapeHtml(toolName);
  container.appendChild(badge);
  autoScroll();
  return badge;
}

function completeToolBadge(badge, toolName, success) {
  if (!badge) return;
  badge.classList.remove('pending');
  badge.classList.add(success ? 'success' : 'error');
  autoScroll();
}

function resetToolBadges() { toolBadgesContainer = null; }

function addError(message) {
  const div = document.createElement('div');
  div.className = 'error-banner';
  div.textContent = message;
  chatMessages.appendChild(div);
  autoScroll();
}

function addSystemNotice(message) {
  const div = document.createElement('div');
  div.className = 'system-notice';
  div.innerHTML = message;
  chatMessages.appendChild(div);
  autoScroll();
}

async function sendMessage() {
  const text = input.value.trim();
  if (!text || isStreaming) return;

  isStreaming = true;
  sendBtn.disabled = true;
  sendBtn.textContent = '...';
  input.value = '';
  input.style.height = 'auto';

  addUserMessage(text);
  startAgentStreaming();
  resetToolBadges();

  const badgeMap = {};

  try {
    const response = await fetch(BASE_PATH + '/api/chat', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({message: text, sessionId: sessionId})
    });
    if (!response.ok) throw new Error('HTTP ' + response.status);

    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';
    let currentEvent = '';
    let currentData = '';
    let streamDone = false;

    while (!streamDone) {
      const {done, value} = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, {stream: true});
      const lines = buffer.split('\n');
      buffer = lines.pop();

      for (const line of lines) {
        const trimmed = line.replace(/\r$/, '');
        if (trimmed.startsWith('event:')) {
          currentEvent = trimmed.substring(6).trim();
        } else if (trimmed.startsWith('data:')) {
          const dataPart = trimmed.substring(5);
          if (currentData.length > 0) currentData += '\n';
          currentData += dataPart.startsWith(' ') ? dataPart.substring(1) : dataPart;
        } else if (trimmed === '') {
          if (currentEvent === 'content') {
            let chunk;
            try { chunk = JSON.parse(currentData); }
            catch(e) { chunk = currentData; }
            appendContentChunk(chunk);
          } else if (currentEvent === 'tool_start') {
            const name = currentData;
            badgeMap[name] = addToolBadge(name);
          } else if (currentEvent === 'tool_result') {
            const colonIdx = currentData.indexOf(':');
            const name = colonIdx > 0 ? currentData.substring(0, colonIdx).trim() : currentData;
            completeToolBadge(badgeMap[name], name, true);
          } else if (currentEvent === 'done') {
            streamDone = true;
          } else if (currentEvent === 'context_compressed') {
            try {
              const info = JSON.parse(currentData);
              addSystemNotice('Context auto-compressed: ' + info.originalTokens + ' to ~' + info.compressedTokens + ' tokens (' + info.removedRounds + ' old rounds removed)');
            } catch(e) {
              addSystemNotice('Context auto-compressed');
            }
          } else if (currentEvent === 'error') {
            addError(currentData);
          }
          currentEvent = '';
          currentData = '';
        }
      }
    }
    finalizeAgentContent();
  } catch (err) {
    addError('Connection error: ' + err.message);
    finalizeAgentContent();
  } finally {
    isStreaming = false;
    sendBtn.disabled = false;
    sendBtn.textContent = 'Send';
    input.focus();
  }
}
</script>
</body>
</html>`
