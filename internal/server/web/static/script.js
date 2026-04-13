const loginView = document.getElementById("login-view");
const appView = document.getElementById("app-view");

const createForm = document.getElementById("create-form");
const loginForm = document.getElementById("login-form");
const refreshBtn = document.getElementById("refresh");
const logoutBtn = document.getElementById("logout");

const createResult = document.getElementById("create-result");
const sessionResult = document.getElementById("session-result");
const rows = document.getElementById("rows");

function setText(el, text) {
  el.textContent = text;
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

    const data = await res.json();
    rows.innerHTML = "";
    
    if (data.length === 0) {
      rows.innerHTML = `<tr><td colspan="4" style="text-align:center; color: var(--text-dim); padding: 2rem;">NO_RECORDS_FOUND</td></tr>`;
    }
    
    for (const row of data) {
      const tr = document.createElement("tr");
      tr.innerHTML = `
        <td><a href="/${row.shortlink}" target="_blank" rel="noopener noreferrer">${row.shortlink}</a></td>
        <td><a href="${row.longlink}" target="_blank" rel="noopener noreferrer">${row.longlink}</a></td>
        <td class="text-right">${row.hits}</td>
        <td class="text-right">${formatExpiry(row.expiry_time)}</td>
      `;
      rows.appendChild(tr);
    }
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

    if (typeof out === "string") {
      setText(createResult, `SUCCESS: ${window.location.origin}/${out}`);
    } else {
      setText(createResult, `SUCCESS: ${out.shorturl}`);
    }

    document.getElementById("longlink").value = "";
    document.getElementById("shortlink").value = "";
    await refreshTable();
  } catch (e) {
    setText(createResult, "Error connecting to server.");
  }
  setLoading(btn, false);
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
