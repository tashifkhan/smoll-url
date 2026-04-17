const loginView = document.getElementById("login-view");
const appView = document.getElementById("app-view");

const createForm = document.getElementById("create-form");
const editForm = document.getElementById("edit-form");
const loginForm = document.getElementById("login-form");
const refreshBtn = document.getElementById("refresh");
const logoutBtn = document.getElementById("logout");

const createResult = document.getElementById("create-result");
const editResult = document.getElementById("edit-result");
const sessionResult = document.getElementById("session-result");
const rows = document.getElementById("rows");
const createShortOutput = document.getElementById("create-short-output");
const createdShortURL = document.getElementById("created-short-url");
const copyShortURLBtn = document.getElementById("copy-short-url");
const copyResult = document.getElementById("copy-result");

const copyIconSvg = `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="pointer-events: none;"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path></svg>`;
const checkIconSvg = `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="pointer-events: none;"><polyline points="20 6 9 17 4 12"></polyline></svg>`;
const pencilIconSvg = `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="pointer-events: none;"><path d="M17 3a2.828 2.828 0 1 1 4 4L7.5 20.5 2 22l1.5-5.5L17 3z"></path></svg>`;
const barChartIconSvg = `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="pointer-events: none;"><line x1="18" y1="20" x2="18" y2="10"></line><line x1="12" y1="20" x2="12" y2="4"></line><line x1="6" y1="20" x2="6" y2="14"></line></svg>`;

function setText(el, text) {
  el.textContent = text;
}

function setCreatedShortURL(url) {
  if (!url) {
    createShortOutput.hidden = true;
    createdShortURL.removeAttribute("href");
    createdShortURL.textContent = "";
    setText(copyResult, "");
    return;
  }

  createShortOutput.hidden = false;
  createdShortURL.href = url;
  createdShortURL.textContent = url;
  setText(copyResult, "");
}

function setLoading(btn, isLoading) {
  if (isLoading) {
    btn.classList.add("is-loading");
    btn.setAttribute("disabled", "true");
  } else {
    btn.classList.remove("is-loading");
    btn.removeAttribute("disabled");
  }
}

async function jsonOrText(res) {
  const ctype = res.headers.get("content-type") || "";
  if (ctype.includes("application/json")) {
    return res.json();
  }
  return res.text();
}

function formatExpiry(ts) {
  if (!ts || ts <= 0) return "never";
  return new Date(ts * 1000).toLocaleString();
}

async function checkAuth() {
  try {
    const res = await fetch("/api/whoami");
    const role = await res.text();
    if (res.ok && (role === "admin" || role === "public")) {
      loginView.style.display = "none";
      appView.style.display = "block";
      logoutBtn.style.display = role === "admin" ? "inline-block" : "none";
      await refreshTable();
    } else {
      loginView.style.display = "block";
      appView.style.display = "none";
      logoutBtn.style.display = "none";
    }
  } catch (e) {
    loginView.style.display = "block";
    appView.style.display = "none";
    logoutBtn.style.display = "none";
  }
}

let currentTableData = [];
let showFullUrl = false;
const toggleFullUrlBtn = document.getElementById("toggle-full-url");

if (toggleFullUrlBtn) {
  toggleFullUrlBtn.addEventListener("click", () => {
    showFullUrl = !showFullUrl;
    toggleFullUrlBtn.textContent = showFullUrl ? "[ FULL_URL: ON ]" : "[ FULL_URL: OFF ]";
    toggleFullUrlBtn.style.color = showFullUrl ? "var(--lime-accent)" : "var(--text-dim)";
    if (currentTableData.length > 0) {
      renderTableRows(currentTableData);
    }
  });
}

function renderTableRows(data) {
  rows.innerHTML = "";
  
  if (data.length === 0) {
    rows.innerHTML = `<tr><td colspan="5" style="text-align:center; color: var(--text-dim); padding: 2rem;">NO_RECORDS_FOUND</td></tr>`;
    return;
  }
  
  const baseUrl = window.location.origin;

  for (const row of data) {
    const displayUrl = showFullUrl ? `${baseUrl}/${row.shortlink}` : row.shortlink;
    const copyUrl = `${baseUrl}/${row.shortlink}`;
    
    const tr = document.createElement("tr");
    tr.innerHTML = `
      <td>
        <a href="/${row.shortlink}" target="_blank" rel="noopener noreferrer" style="white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 250px; display: inline-block; vertical-align: middle;">${displayUrl}</a>
      </td>
      <td><a href="${row.longlink}" target="_blank" rel="noopener noreferrer" style="white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 300px; display: inline-block; vertical-align: middle;">${row.longlink}</a></td>
      <td class="text-right">${row.hits}</td>
      <td class="text-right">${formatExpiry(row.expiry_time)}</td>
      <td class="text-right">
        <div class="row-actions" style="justify-content: flex-end; gap: 0.4rem;">
          <button class="btn btn-outline btn-sm copy-row-btn" data-url="${copyUrl}" type="button" style="padding: 0.3rem 0.4rem; display: flex; align-items: center; justify-content: center;" title="Copy to clipboard">${copyIconSvg}</button>
          <button class="btn btn-outline btn-sm edit-row-btn" data-shortlink="${row.shortlink}" type="button" style="padding: 0.3rem 0.4rem; display: flex; align-items: center; justify-content: center;" title="Edit shortcut">${pencilIconSvg}</button>
          <button class="btn btn-outline btn-sm analytics-row-btn" data-shortlink="${row.shortlink}" type="button" style="padding: 0.3rem 0.4rem; display: flex; align-items: center; justify-content: center;" title="View analytics">${barChartIconSvg}</button>
        </div>
      </td>
    `;
    rows.appendChild(tr);
  }
}

if (rows) {
  rows.addEventListener("click", async (e) => {
    const editBtn = e.target.closest(".edit-row-btn");
    if (editBtn) {
      const shortlink = editBtn.getAttribute("data-shortlink") || "";
      const selected = currentTableData.find((item) => item.shortlink === shortlink);
      if (!selected) {
        setText(editResult, "ERR_RECORD_NOT_FOUND");
        return;
      }

      document.getElementById("edit-shortlink").value = selected.shortlink;
      document.getElementById("edit-original-shortlink").value = selected.shortlink;
      document.getElementById("edit-longlink").value = selected.longlink;
      document.getElementById("edit-reset-hits").checked = false;
      setText(editResult, "");
      editResult.style.display = "none";
      
      document.getElementById("edit-modal").style.display = "flex";
      setTimeout(() => document.getElementById("edit-longlink").focus(), 50);
      return;
    }

    const analyticsBtn = e.target.closest(".analytics-row-btn");
    if (analyticsBtn) {
      const shortlink = analyticsBtn.getAttribute("data-shortlink") || "";
      openAnalytics(shortlink);
      return;
    }

    const btn = e.target.closest(".copy-row-btn");
    if (btn) {
      const url = btn.getAttribute("data-url");
      try {
        await navigator.clipboard.writeText(url);
        const originalHtml = copyIconSvg;
        btn.innerHTML = checkIconSvg;
        btn.style.borderColor = "var(--lime-accent)";
        btn.style.color = "var(--lime-accent)";
        setTimeout(() => {
          btn.innerHTML = originalHtml;
          btn.style.borderColor = "";
          btn.style.color = "";
        }, 2000);
      } catch (err) {
        console.error("Failed to copy", err);
      }
    }
  });
}

async function refreshTable() {
  setLoading(refreshBtn, true);
  rows.innerHTML = `<tr class="table-loader"><td colspan="5">SYNCING_DATA_RECORDS...</td></tr>`;
  
  try {
    const res = await fetch("/api/all");
    if (!res.ok) {
      rows.innerHTML = `<tr><td colspan="5" class="text-lime">ERR_FETCHING_RECORDS</td></tr>`;
      setLoading(refreshBtn, false);
      return;
    }

    currentTableData = await res.json();
    renderTableRows(currentTableData);
  } catch(e) {
    rows.innerHTML = `<tr><td colspan="5" class="text-lime">ERR_CONNECTION_DROPPED</td></tr>`;
  }
  setLoading(refreshBtn, false);
}

createForm.addEventListener("submit", async (e) => {
  e.preventDefault();
  const btn = createForm.querySelector('button[type="submit"]');
  setLoading(btn, true);
  setText(createResult, "Processing...");
  setCreatedShortURL("");

  const longlink = document.getElementById("longlink").value.trim();
  const shortlink = document.getElementById("shortlink").value.trim();
  const expiry = Number.parseInt(document.getElementById("expiry").value, 10) || 0;

  const payload = { longlink, shortlink, expiry_delay: expiry };

  try {
    const res = await fetch("/api/new", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });

    const out = await jsonOrText(res);
    if (!res.ok) {
      if (typeof out === "string") {
        setText(createResult, "Error: " + out);
      } else {
        setText(createResult, "Error: " + (out.reason || "Could not create short URL"));
      }
      setLoading(btn, false);
      return;
    }

    let shortURL = "";
    if (typeof out === "string") {
      shortURL = `${window.location.origin}/${out}`;
    } else {
      shortURL = out.shorturl;
    }

    setText(createResult, "SUCCESS: LINK_CREATED");
    setCreatedShortURL(shortURL);

    document.getElementById("longlink").value = "";
    document.getElementById("shortlink").value = "";
    await refreshTable();
  } catch (e) {
    setText(createResult, "Error connecting to server.");
    setCreatedShortURL("");
  }
  setLoading(btn, false);
});

editForm.addEventListener("submit", async (e) => {
  e.preventDefault();
  const btn = editForm.querySelector('button[type="submit"]');
  setLoading(btn, true);
  setText(editResult, "Applying edits...");
  editResult.style.display = "block";

  const originalShortlink = document.getElementById("edit-original-shortlink").value.trim();
  const shortlink = document.getElementById("edit-shortlink").value.trim();
  const longlink = document.getElementById("edit-longlink").value.trim();
  const resetHits = document.getElementById("edit-reset-hits").checked;

  const payload = { original_shortlink: originalShortlink, shortlink, longlink, reset_hits: resetHits };

  try {
    const res = await fetch("/api/edit", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });

    const out = await jsonOrText(res);
    if (!res.ok) {
      if (typeof out === "string") {
        setText(editResult, "Error: " + out);
      } else {
        setText(editResult, "Error: " + (out.reason || "Could not edit short URL"));
      }
      setLoading(btn, false);
      return;
    }

    setText(editResult, "SUCCESS: LINK_UPDATED");
    setTimeout(() => {
      document.getElementById("edit-modal").style.display = "none";
    }, 500);
    await refreshTable();
  } catch (e) {
    setText(editResult, "Error connecting to server.");
  }

  setLoading(btn, false);
});

copyShortURLBtn.addEventListener("click", async () => {
  const targetURL = createdShortURL.textContent.trim();
  if (!targetURL) {
    setText(copyResult, "NOTHING_TO_COPY");
    return;
  }

  try {
    await navigator.clipboard.writeText(targetURL);
    setText(copyResult, "COPIED_TO_CLIPBOARD");
    copyShortURLBtn.innerHTML = checkIconSvg;
    copyShortURLBtn.style.borderColor = "var(--lime-accent)";
    copyShortURLBtn.style.color = "var(--lime-accent)";
    setTimeout(() => {
      copyShortURLBtn.innerHTML = copyIconSvg;
      copyShortURLBtn.style.borderColor = "";
      copyShortURLBtn.style.color = "";
    }, 2000);
  } catch (e) {
    setText(copyResult, "COPY_FAILED");
  }
});

loginForm.addEventListener("submit", async (e) => {
  e.preventDefault();
  const btn = loginForm.querySelector('button[type="submit"]');
  setLoading(btn, true);
  setText(sessionResult, "Authenticating...");
  
  const passwordInput = document.getElementById("password");
  const password = passwordInput.value;
  
  try {
    const res = await fetch("/api/login", { method: "POST", body: password });
    const out = await jsonOrText(res);
    if (!res.ok) {
      if (typeof out === "string") {
        setText(sessionResult, "AUTH_FAILED: " + out);
      } else {
        setText(sessionResult, "AUTH_FAILED: " + (out.reason || "Login failed"));
      }
      passwordInput.value = "";
      setLoading(btn, false);
      return;
    }
    
    setText(sessionResult, "AUTH_SUCCESS. Initializing system...");
    passwordInput.value = "";
    await checkAuth();
  } catch (e) {
    setText(sessionResult, "AUTH_ERROR: Connection failed.");
  }
  setLoading(btn, false);
});

logoutBtn.addEventListener("click", async () => {
  setLoading(logoutBtn, true);
  try {
    await fetch("/api/logout", { method: "DELETE" });
    await checkAuth();
  } catch (e) {
    console.error("Logout failed", e);
  }
  setLoading(logoutBtn, false);
});

refreshBtn.addEventListener("click", async () => {
  await refreshTable();
});

// Initialize app state
checkAuth();

// ---- Analytics ----

let currentAnalyticsSlug = '';
let currentAnalyticsDays = 30;

function flagEmoji(code) {
  if (!code || code === 'Unknown') return '🌐';
  try {
    return [...code.toUpperCase()].map(c =>
      String.fromCodePoint(c.charCodeAt(0) + 127397)
    ).join('');
  } catch (e) {
    return '🌐';
  }
}

function escapeHTML(value) {
  return String(value)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/\"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

function renderBarChart(items, colorVar, labelWidth) {
  if (!items || items.length === 0) {
    return '<p class="no-data-msg">NO_DATA_AVAILABLE</p>';
  }
  const max = Math.max(...items.map(i => i.count));
  const maxLen = labelWidth || 16;
  return items.map(item => {
    const pct = max > 0 ? ((item.count / max) * 100).toFixed(1) : 0;
    const raw = String(item.label || "");
    const label = raw.length > maxLen ? raw.slice(0, maxLen) + '…' : raw;
    const rawEscaped = escapeHTML(raw);
    const labelEscaped = escapeHTML(label);
    return `<div class="stat-bar-row">
      <span class="stat-bar-label" title="${rawEscaped}">${labelEscaped}</span>
      <div class="stat-bar-track">
        <div class="stat-bar-fill" style="width:${pct}%;background:var(${colorVar});color:var(${colorVar})"></div>
      </div>
      <span class="stat-bar-value">${item.count.toLocaleString()}</span>
    </div>`;
  }).join('');
}

function renderTimeline(timeline) {
  if (!timeline || timeline.length === 0) {
    return '<p class="no-data-msg">NO_DATA_AVAILABLE</p>';
  }
  const max = Math.max(...timeline.map(t => t.count));
  const bars = timeline.map(t => {
    const h = max > 0 ? Math.max(2, (t.count / max) * 100) : 2;
    return `<div class="timeline-bar-col">
      <div class="timeline-tooltip">${t.date}<br>${t.count.toLocaleString()} click${t.count !== 1 ? 's' : ''}</div>
      <div class="timeline-bar" style="height:${h}%"></div>
    </div>`;
  }).join('');

  const first = timeline[0]?.date || '';
  const mid = timeline[Math.floor(timeline.length / 2)]?.date || '';
  const last = timeline[timeline.length - 1]?.date || '';

  return `<div class="timeline-chart">${bars}</div>
    <div class="timeline-labels"><span>${first}</span><span>${mid}</span><span>${last}</span></div>`;
}

function renderAnalyticsContent(data, days) {
  const countryItems = data.countries.map(c => ({
    label: `${flagEmoji(c.label)} ${c.label}`,
    count: c.count,
  }));

  return `
    <div class="analytics-summary">
      <div class="summary-stat">
        <span class="summary-label">TOTAL_CLICKS</span>
        <span class="summary-value text-lime">${data.total_clicks.toLocaleString()}</span>
      </div>
      <div class="summary-stat">
        <span class="summary-label">COUNTRIES_SEEN</span>
        <span class="summary-value text-cyan">${data.countries.length}</span>
      </div>
      <div class="summary-stat">
        <span class="summary-label">TIME_RANGE</span>
        <span class="summary-value">${days === 0 ? 'ALL' : days + 'D'}</span>
      </div>
    </div>

    <div class="analytics-section">
      <h3 class="analytics-section-title">>::  CLICK_TIMELINE</h3>
      ${renderTimeline(data.timeline)}
    </div>

    <div class="analytics-grid-2">
      <div class="analytics-section">
        <h3 class="analytics-section-title">>::  TOP_COUNTRIES</h3>
        ${renderBarChart(countryItems, '--cyan-primary', 14)}
      </div>
      <div>
        <div class="analytics-section">
          <h3 class="analytics-section-title">>::  DEVICE_TYPES</h3>
          ${renderBarChart(data.devices, '--lime-accent', 14)}
        </div>
        <div class="analytics-section">
          <h3 class="analytics-section-title">>::  BROWSERS</h3>
          ${renderBarChart(data.browsers, '--lime-accent', 14)}
        </div>
      </div>
    </div>

    <div class="analytics-section">
      <h3 class="analytics-section-title">>::  TOP_REFERRERS</h3>
      ${renderBarChart(data.referrers, '--cyan-primary', 42)}
    </div>
  `;
}

async function loadAnalytics() {
  const content = document.getElementById('analytics-content');
  content.innerHTML = '<div class="analytics-loading">FETCHING_TELEMETRY_DATA...</div>';
  try {
    const res = await fetch(`/api/analytics?slug=${encodeURIComponent(currentAnalyticsSlug)}&days=${currentAnalyticsDays}`);
    if (!res.ok) throw new Error('fetch failed');
    const data = await res.json();
    content.innerHTML = renderAnalyticsContent(data, currentAnalyticsDays);
  } catch (e) {
    content.innerHTML = '<div class="analytics-error">ERR_ANALYTICS_UNAVAILABLE</div>';
  }
}

function openAnalytics(slug) {
  currentAnalyticsSlug = slug;
  currentAnalyticsDays = 30;
  document.getElementById('analytics-slug-title').textContent = slug;
  document.querySelectorAll('.period-btn').forEach(btn => {
    btn.classList.toggle('active', Number(btn.dataset.days) === currentAnalyticsDays);
  });
  document.getElementById('analytics-modal').style.display = 'flex';
  loadAnalytics();
}

document.querySelectorAll('.period-btn').forEach(btn => {
  btn.addEventListener('click', () => {
    currentAnalyticsDays = Number(btn.dataset.days);
    document.querySelectorAll('.period-btn').forEach(b => {
      b.classList.toggle('active', b === btn);
    });
    loadAnalytics();
  });
});

document.getElementById('close-analytics-modal').addEventListener('click', () => {
  document.getElementById('analytics-modal').style.display = 'none';
});

document.getElementById('analytics-modal').addEventListener('click', (e) => {
  if (e.target === document.getElementById('analytics-modal')) {
    document.getElementById('analytics-modal').style.display = 'none';
  }
});

const closeEditModalBtn = document.getElementById("close-edit-modal");
if (closeEditModalBtn) {
  closeEditModalBtn.addEventListener("click", () => {
    document.getElementById("edit-modal").style.display = "none";
  });
}
document.getElementById("edit-modal").addEventListener("click", (e) => {
  if (e.target === document.getElementById("edit-modal")) {
    document.getElementById("edit-modal").style.display = "none";
  }
});
