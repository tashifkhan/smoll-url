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
    rows.innerHTML = `<tr><td colspan="4" style="text-align:center; color: var(--text-dim); padding: 2rem;">NO_RECORDS_FOUND</td></tr>`;
    return;
  }
  
  const baseUrl = window.location.origin;

  for (const row of data) {
    const displayUrl = showFullUrl ? `${baseUrl}/${row.shortlink}` : row.shortlink;
    const copyUrl = `${baseUrl}/${row.shortlink}`;
    
    const tr = document.createElement("tr");
    tr.innerHTML = `
      <td>
        <div style="display: flex; align-items: center; justify-content: flex-start; gap: 0.75rem;">
          <a href="/${row.shortlink}" target="_blank" rel="noopener noreferrer" style="white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 250px; display: inline-block; vertical-align: middle;">${displayUrl}</a>
          <button class="btn btn-outline btn-sm copy-row-btn" data-url="${copyUrl}" type="button" style="flex-shrink: 0; padding: 0.3rem 0.4rem; display: flex; align-items: center; justify-content: center;" title="Copy to clipboard">${copyIconSvg}</button>
          <button class="btn btn-outline btn-sm edit-row-btn" data-shortlink="${row.shortlink}" type="button" style="flex-shrink: 0; padding: 0.3rem 0.4rem; display: flex; align-items: center; justify-content: center;" title="Edit shortcut">${pencilIconSvg}</button>
        </div>
      </td>
      <td><a href="${row.longlink}" target="_blank" rel="noopener noreferrer" style="white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 300px; display: inline-block; vertical-align: middle;">${row.longlink}</a></td>
      <td class="text-right">${row.hits}</td>
      <td class="text-right">${formatExpiry(row.expiry_time)}</td>
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
  rows.innerHTML = `<tr class="table-loader"><td colspan="4">SYNCING_DATA_RECORDS...</td></tr>`;
  
  try {
    const res = await fetch("/api/all");
    if (!res.ok) {
      rows.innerHTML = `<tr><td colspan="4" class="text-lime">ERR_FETCHING_RECORDS</td></tr>`;
      setLoading(refreshBtn, false);
      return;
    }

    currentTableData = await res.json();
    renderTableRows(currentTableData);
  } catch(e) {
    rows.innerHTML = `<tr><td colspan="4" class="text-lime">ERR_CONNECTION_DROPPED</td></tr>`;
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
