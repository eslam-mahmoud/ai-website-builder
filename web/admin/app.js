/* CMS Admin Dashboard — block-builder edition (no build step, vanilla JS). */
"use strict";

const S = {
  access: localStorage.getItem("cms_access") || "",
  refresh: localStorage.getItem("cms_refresh") || "",
  me: null,
  memberships: [],
  tenantId: localStorage.getItem("cms_tenant") || "",
  sectionTypes: [],      // block types of the current tenant
  view: "websites",
  arg: null,
  website: null,
  websiteTab: "pages",
  page: null,
  sections: [],
  sectionEdits: {},      // section id -> draft content object
  selectedSection: null, // section id shown in the inspector
  settingsDraft: null,
  pollTimer: null,
  previewOn: false,
};

const app = document.getElementById("app");

/* ---------- utilities ---------- */

function esc(s) {
  return String(s ?? "").replace(/[&<>"']/g, c =>
    ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
}

function toast(msg, isError) {
  document.querySelectorAll(".toast").forEach(t => t.remove());
  const t = document.createElement("div");
  t.className = "toast" + (isError ? " error" : "");
  t.textContent = msg;
  document.body.appendChild(t);
  setTimeout(() => t.remove(), isError ? 6000 : 3500);
}

function fmtDate(s) { return s ? new Date(s).toLocaleString() : ""; }

function slugify(s) {
  return s.toLowerCase().replace(/[^a-z0-9]+/g, "_").replace(/^_+|_+$/g, "").slice(0, 40);
}

function setTokens(access, refresh) {
  S.access = access; S.refresh = refresh;
  localStorage.setItem("cms_access", access);
  localStorage.setItem("cms_refresh", refresh);
}

function clearSession() {
  S.access = S.refresh = ""; S.me = null;
  localStorage.removeItem("cms_access");
  localStorage.removeItem("cms_refresh");
}

async function api(path, opts = {}, retry = true) {
  const headers = opts.headers || {};
  if (S.access) headers["Authorization"] = "Bearer " + S.access;
  if (opts.body && !(opts.body instanceof FormData)) {
    headers["Content-Type"] = "application/json";
    opts = { ...opts, body: JSON.stringify(opts.body) };
  }
  const res = await fetch("/api" + path, { ...opts, headers });
  if (res.status === 401 && retry && S.refresh) {
    const ok = await tryRefresh();
    if (ok) return api(path, opts, false);
    clearSession(); render();
    throw new Error("session expired, please log in again");
  }
  let data = null;
  try { data = await res.json(); } catch { /* empty body */ }
  if (!res.ok) throw new Error((data && data.error) || res.statusText);
  return data;
}

async function tryRefresh() {
  try {
    const res = await fetch("/api/auth/refresh", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ refresh_token: S.refresh }),
    });
    if (!res.ok) return false;
    const data = await res.json();
    setTokens(data.access_token, data.refresh_token);
    return true;
  } catch { return false; }
}

function tenantPath(rest) { return `/tenants/${S.tenantId}${rest}`; }

function curRole() {
  if (S.me?.is_platform_admin) return "tenant_admin";
  const m = S.memberships.find(m => m.tenant_id === S.tenantId);
  return m ? m.role : "viewer";
}

async function loadSectionTypes() {
  S.sectionTypes = await api(tenantPath("/section-types"));
}

/* ---------- boot ---------- */

async function boot() {
  if (!S.access) { render(); return; }
  try {
    const me = await api("/me");
    S.me = me.user;
    S.memberships = me.memberships;
    if (S.me.is_platform_admin) {
      const tenants = await api("/tenants");
      S.memberships = tenants.filter(t => t.status === "active")
        .map(t => ({ tenant_id: t.id, tenant_name: t.name, role: "tenant_admin" }));
    }
    if (!S.memberships.find(m => m.tenant_id === S.tenantId)) {
      S.tenantId = S.memberships[0] ? S.memberships[0].tenant_id : "";
    }
    S.view = S.tenantId ? "websites" : (S.me.is_platform_admin ? "tenants" : "websites");
  } catch (e) {
    if (S.access) { clearSession(); }
  }
  render();
}

function nav(view, arg) {
  if (S.pollTimer) { clearInterval(S.pollTimer); S.pollTimer = null; }
  S.view = view; S.arg = arg || null;
  render();
}

/* ---------- root render ---------- */

function render() {
  if (!S.me) { renderLogin(); return; }
  const items = [
    ["websites", "Websites"],
    ["media", "Media library"],
  ];
  if (curRole() === "tenant_admin") items.push(["blocktypes", "Block types"]);
  items.push(["users", "Users"], ["audit", "Audit log"]);
  if (S.me.is_platform_admin) items.push(["tenants", "Tenants"]);

  app.innerHTML = `
  <div class="layout">
    <aside class="sidebar">
      <span class="brand">⚡ CMS Admin</span>
      <div class="tenant-select">
        <select id="tenant-switch">
          ${S.memberships.map(m => `<option value="${m.tenant_id}"
            ${m.tenant_id === S.tenantId ? "selected" : ""}>${esc(m.tenant_name)}</option>`).join("")}
          ${S.memberships.length === 0 ? `<option value="">(no tenants)</option>` : ""}
        </select>
      </div>
      <nav>
        ${items.map(([v, label]) =>
          `<a href="#" data-nav="${v}" class="${S.view === v || (v === "websites" && ["website", "page"].includes(S.view)) ? "active" : ""}">${label}</a>`).join("")}
      </nav>
      <div class="user-box">
        ${esc(S.me.name)}<br>${esc(S.me.email)}
        ${S.me.is_platform_admin ? '<br><span class="badge ok">platform admin</span>' : ""}
        <br><button class="btn small" id="logout-btn">Log out</button>
      </div>
    </aside>
    <main class="content" id="main"></main>
  </div>`;

  document.getElementById("tenant-switch").onchange = e => {
    S.tenantId = e.target.value;
    S.sectionTypes = [];
    localStorage.setItem("cms_tenant", S.tenantId);
    nav("websites");
  };
  document.getElementById("logout-btn").onclick = async () => {
    try { await api("/auth/logout", { method: "POST", body: { refresh_token: S.refresh } }); } catch {}
    clearSession(); render();
  };
  app.querySelectorAll("[data-nav]").forEach(a => a.onclick = e => {
    e.preventDefault(); nav(a.dataset.nav);
  });

  const main = document.getElementById("main");
  if (!S.tenantId && S.view !== "tenants") {
    main.innerHTML = `<div class="empty">No tenant selected.
      ${S.me.is_platform_admin ? 'Create one under <b>Tenants</b>.' : "Ask your administrator for access."}</div>`;
    return;
  }
  ({
    websites: renderWebsites,
    website: renderWebsite,
    page: renderPageBuilder,
    blocktypes: renderBlockTypes,
    media: renderMediaView,
    users: renderUsers,
    audit: renderAudit,
    tenants: renderTenants,
  }[S.view] || renderWebsites)(main);
}

/* ---------- login ---------- */

function renderLogin() {
  app.innerHTML = `
  <div class="login-wrap"><div class="login-box">
    <h1>⚡ CMS Admin</h1>
    <form id="login-form">
      <label>Email <input name="email" type="email" required autofocus></label>
      <label>Password <input name="password" type="password" required></label>
      <button class="btn primary mt" style="width:100%">Log in</button>
    </form>
  </div></div>`;
  document.getElementById("login-form").onsubmit = async e => {
    e.preventDefault();
    const f = new FormData(e.target);
    try {
      const res = await fetch("/api/auth/login", {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email: f.get("email"), password: f.get("password") }),
      });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || "login failed");
      setTokens(data.access_token, data.refresh_token);
      await boot();
    } catch (err) { toast(err.message, true); }
  };
}

/* ---------- websites ---------- */

async function renderWebsites(main) {
  main.innerHTML = `<div class="page-head"><h1>Websites</h1>
    <div class="actions"><button class="btn primary" id="new-website">+ New website</button></div></div>
    <div id="list">Loading…</div>`;
  document.getElementById("new-website").onclick = async () => {
    const name = prompt("Website name:");
    if (!name) return;
    try {
      const site = await api(tenantPath("/websites"), { method: "POST", body: { name } });
      nav("website", site.id);
    } catch (e) { toast(e.message, true); }
  };
  try {
    const sites = await api(tenantPath("/websites"));
    document.getElementById("list").innerHTML = sites.length === 0
      ? `<div class="empty">No websites yet. Create the first one.</div>`
      : `<div class="table-wrap"><table>
          <tr><th>Name</th><th>Domain</th><th>Status</th><th></th></tr>
          ${sites.map(s => `<tr>
            <td><a href="#" data-site="${s.id}"><b>${esc(s.name)}</b></a></td>
            <td>${esc(s.domain) || '<span class="muted">—</span>'}</td>
            <td><span class="badge ${s.status}">${s.status}</span></td>
            <td><button class="btn small" data-site="${s.id}">Open</button></td>
          </tr>`).join("")}
        </table></div>`;
    main.querySelectorAll("[data-site]").forEach(el => el.onclick = e => {
      e.preventDefault(); nav("website", el.dataset.site);
    });
  } catch (e) { document.getElementById("list").innerHTML = `<div class="empty">${esc(e.message)}</div>`; }
}

/* ---------- website detail ---------- */

async function renderWebsite(main) {
  main.innerHTML = "Loading…";
  try { S.website = await api(tenantPath(`/websites/${S.arg}`)); }
  catch (e) { main.innerHTML = `<div class="empty">${esc(e.message)}</div>`; return; }
  const w = S.website;
  S.settingsDraft = { ...(w.settings || {}) };

  main.innerHTML = `
    <div class="page-head">
      <h1>${esc(w.name)}</h1>
      <div class="actions">
        <button class="btn" id="preview-btn">Preview</button>
        <button class="btn primary" id="publish-btn">Publish</button>
      </div>
    </div>
    <div id="publish-status"></div>
    <div class="tabs">
      ${["pages", "settings", "deployments"].map(t =>
        `<button data-tab="${t}" class="${S.websiteTab === t ? "active" : ""}">${t[0].toUpperCase() + t.slice(1)}</button>`).join("")}
    </div>
    <div id="tab-body">Loading…</div>`;

  main.querySelectorAll("[data-tab]").forEach(b => b.onclick = () => {
    S.websiteTab = b.dataset.tab;
    main.querySelectorAll("[data-tab]").forEach(x => x.classList.toggle("active", x === b));
    renderWebsiteTab();
  });

  document.getElementById("preview-btn").onclick = async () => {
    try {
      const res = await api(tenantPath(`/websites/${w.id}/preview`), { method: "POST" });
      window.open(res.url, "_blank");
    } catch (e) { toast(e.message, true); }
  };
  document.getElementById("publish-btn").onclick = async () => {
    if (!confirm("Publish the current content?")) return;
    try {
      const d = await api(tenantPath(`/websites/${w.id}/publish`), { method: "POST" });
      toast("Publish started");
      watchDeployment(d.id);
    } catch (e) { toast(e.message, true); }
  };

  renderWebsiteTab();
}

function watchDeployment(id) {
  const box = document.getElementById("publish-status");
  if (S.pollTimer) clearInterval(S.pollTimer);
  const tick = async () => {
    try {
      const d = await api(tenantPath(`/deployments/${id}`));
      if (!box.isConnected) { clearInterval(S.pollTimer); return; }
      box.innerHTML = `<div class="panel">Deployment <code>${d.id.slice(0, 8)}</code>
        <span class="badge ${d.status}">${d.status}</span>
        ${d.github_repo ? ` → <code>${esc(d.github_repo)}</code>` : ""}
        ${d.error_message ? `<div class="muted mt">${esc(d.error_message)}</div>` : ""}</div>`;
      if (d.status === "succeeded" || d.status === "failed") {
        clearInterval(S.pollTimer); S.pollTimer = null;
        toast(d.status === "succeeded" ? "Published ✓" : "Publish failed", d.status === "failed");
        if (S.websiteTab === "deployments") renderWebsiteTab();
      }
    } catch { clearInterval(S.pollTimer); S.pollTimer = null; }
  };
  tick();
  S.pollTimer = setInterval(tick, 3000);
}

function renderWebsiteTab() {
  const body = document.getElementById("tab-body");
  if (!body) return;
  ({ pages: renderPagesTab, settings: renderSettingsTab, deployments: renderDeploymentsTab }
    [S.websiteTab])(body);
}

/* ---- pages tab ---- */

async function renderPagesTab(body) {
  body.innerHTML = "Loading…";
  let pages;
  try { pages = await api(tenantPath(`/websites/${S.website.id}/pages`)); }
  catch (e) { body.innerHTML = `<div class="empty">${esc(e.message)}</div>`; return; }

  body.innerHTML = `
    <div class="row" style="margin-bottom:1rem">
      <button class="btn primary" id="add-page">+ Add page</button>
      <span class="muted">Tip: a page with slug <code>home</code> becomes the homepage.</span>
    </div>
    ${pages.length === 0 ? `<div class="empty">No pages yet.</div>` : `
    <div class="table-wrap"><table>
      <tr><th style="width:70px">Order</th><th>Title</th><th>Slug</th><th>Visibility</th><th></th></tr>
      ${pages.map((p, i) => `<tr>
        <td>
          <button class="btn small" data-move="up" data-i="${i}" ${i === 0 ? "disabled" : ""}>↑</button>
          <button class="btn small" data-move="down" data-i="${i}" ${i === pages.length - 1 ? "disabled" : ""}>↓</button>
        </td>
        <td><a href="#" data-page="${p.id}"><b>${esc(p.title)}</b></a></td>
        <td><code>/${esc(p.slug)}</code></td>
        <td><span class="badge ${p.status}">${p.status}</span></td>
        <td class="row">
          <button class="btn small" data-page="${p.id}">Open builder</button>
          <button class="btn small" data-toggle="${p.id}" data-status="${p.status}">
            ${p.status === "visible" ? "Hide" : "Show"}</button>
          <button class="btn small danger" data-del="${p.id}">Delete</button>
        </td>
      </tr>`).join("")}
    </table></div>`}`;

  document.getElementById("add-page").onclick = () => pageFormModal(null);
  body.querySelectorAll("[data-page]").forEach(el => el.onclick = e => {
    e.preventDefault(); nav("page", el.dataset.page);
  });
  body.querySelectorAll("[data-del]").forEach(el => el.onclick = async () => {
    if (!confirm("Delete this page and its blocks?")) return;
    try { await api(tenantPath(`/pages/${el.dataset.del}`), { method: "DELETE" }); renderPagesTab(body); }
    catch (e) { toast(e.message, true); }
  });
  body.querySelectorAll("[data-toggle]").forEach(el => el.onclick = async () => {
    const status = el.dataset.status === "visible" ? "hidden" : "visible";
    try { await api(tenantPath(`/pages/${el.dataset.toggle}`), { method: "PATCH", body: { status } }); renderPagesTab(body); }
    catch (e) { toast(e.message, true); }
  });
  body.querySelectorAll("[data-move]").forEach(el => el.onclick = async () => {
    const i = +el.dataset.i, j = el.dataset.move === "up" ? i - 1 : i + 1;
    const ids = pages.map(p => p.id);
    [ids[i], ids[j]] = [ids[j], ids[i]];
    try {
      await api(tenantPath(`/websites/${S.website.id}/pages/order`), { method: "PUT", body: { page_ids: ids } });
      renderPagesTab(body);
    } catch (e) { toast(e.message, true); }
  });
}

function pageFormModal(page) {
  const m = modal(`
    <h2>${page ? "Edit page" : "New page"}</h2>
    <form id="page-form">
      <label>Title <input name="title" required value="${esc(page?.title)}"></label>
      <label>Slug <span class="hint">(lowercase-with-hyphens; "home" = homepage)</span>
        <input name="slug" required pattern="[a-z0-9]+(-[a-z0-9]+)*" value="${esc(page?.slug)}"></label>
      <label>SEO title <input name="seo_title" value="${esc(page?.seo_title)}"></label>
      <label>SEO description <textarea name="seo_description">${esc(page?.seo_description)}</textarea></label>
      <div class="row mt"><button class="btn primary">Save</button>
        <button type="button" class="btn" data-close>Cancel</button></div>
    </form>`);
  m.querySelector("#page-form").onsubmit = async e => {
    e.preventDefault();
    const f = new FormData(e.target);
    const bodyData = Object.fromEntries(f.entries());
    try {
      if (page) await api(tenantPath(`/pages/${page.id}`), { method: "PATCH", body: bodyData });
      else await api(tenantPath(`/websites/${S.website.id}/pages`), { method: "POST", body: bodyData });
      m.remove();
      if (S.view === "page") render(); else renderWebsiteTab();
    } catch (err) { toast(err.message, true); }
  };
}

/* ---- settings tab ---- */

function renderSettingsTab(body) {
  const st = S.settingsDraft;
  const socials = ["facebook", "instagram", "twitter", "linkedin", "youtube", "tiktok"];
  body.innerHTML = `
  <form id="settings-form">
    <div class="panel"><h2>General</h2>
      <div class="grid-2">
        <label>Website name <input id="w-name" value="${esc(S.website.name)}"></label>
        <label>Custom domain <span class="hint">(e.g. example.com)</span>
          <input id="w-domain" value="${esc(S.website.domain)}"></label>
        <label>Primary color <input type="color" id="s-primary_color" style="height:38px"
          value="${esc(st.primary_color || "#2563eb")}"></label>
        <label>Logo <div class="image-field" id="logo-field"></div></label>
      </div>
    </div>
    <div class="panel"><h2>Contact</h2>
      <div class="grid-2">
        <label>Phone <input id="s-contact_phone" value="${esc(st.contact_phone)}"></label>
        <label>Email <input id="s-contact_email" value="${esc(st.contact_email)}"></label>
        <label>WhatsApp number <span class="hint">(international format)</span>
          <input id="s-whatsapp_number" value="${esc(st.whatsapp_number)}"></label>
        <label>Business address <input id="s-address" value="${esc(st.address)}"></label>
      </div>
    </div>
    <div class="panel"><h2>Social links</h2>
      <div class="grid-2">
        ${socials.map(k => `<label>${k[0].toUpperCase() + k.slice(1)}
          <input data-social="${k}" value="${esc(st.social_links?.[k])}"></label>`).join("")}
      </div>
    </div>
    <div class="panel"><h2>Footer &amp; SEO defaults</h2>
      <label>Footer text <textarea id="s-footer_text">${esc(st.footer_text)}</textarea></label>
      <div class="grid-2">
        <label>Default SEO title <input id="s-seo_title" value="${esc(st.seo_title)}"></label>
        <label>Default SEO description <input id="s-seo_description" value="${esc(st.seo_description)}"></label>
      </div>
    </div>
    <div class="panel"><h2>Deployment</h2>
      <div class="grid-2">
        <label>GitHub repository <span class="hint">(owner/name — auto-created on first publish if empty)</span>
          <input id="s-github_repo" value="${esc(st.github_repo)}"></label>
        <label>Cloudflare Pages project <span class="hint">(connect the repo in Cloudflare once)</span>
          <input id="s-cloudflare_project" value="${esc(st.cloudflare_project)}"></label>
      </div>
    </div>
    <button class="btn primary">Save settings</button>
  </form>`;

  renderImageField(document.getElementById("logo-field"), st.logo_media_id, id => {
    st.logo_media_id = id || "";
    renderSettingsTab(body);
  });

  document.getElementById("settings-form").onsubmit = async e => {
    e.preventDefault();
    const val = id => document.getElementById(id).value.trim();
    const settings = {
      logo_media_id: st.logo_media_id || "",
      primary_color: val("s-primary_color"),
      contact_phone: val("s-contact_phone"),
      contact_email: val("s-contact_email"),
      whatsapp_number: val("s-whatsapp_number"),
      address: val("s-address"),
      footer_text: val("s-footer_text"),
      seo_title: val("s-seo_title"),
      seo_description: val("s-seo_description"),
      github_repo: val("s-github_repo"),
      cloudflare_project: val("s-cloudflare_project"),
      social_links: {},
    };
    body.querySelectorAll("[data-social]").forEach(i => {
      if (i.value.trim()) settings.social_links[i.dataset.social] = i.value.trim();
    });
    try {
      S.website = await api(tenantPath(`/websites/${S.website.id}`), {
        method: "PATCH",
        body: { name: val("w-name"), domain: val("w-domain"), settings },
      });
      S.settingsDraft = { ...(S.website.settings || {}) };
      toast("Settings saved");
    } catch (err) { toast(err.message, true); }
  };
}

/* ---- deployments tab ---- */

async function renderDeploymentsTab(body) {
  body.innerHTML = "Loading…";
  let deps;
  try { deps = await api(tenantPath(`/websites/${S.website.id}/deployments`)); }
  catch (e) { body.innerHTML = `<div class="empty">${esc(e.message)}</div>`; return; }
  body.innerHTML = deps.length === 0 ? `<div class="empty">No deployments yet — hit Publish.</div>` : `
    <div class="table-wrap"><table>
      <tr><th>When</th><th>Status</th><th>Commit</th><th>Repo</th><th>Error</th><th></th></tr>
      ${deps.map(d => `<tr>
        <td>${fmtDate(d.created_at)}</td>
        <td><span class="badge ${d.status}">${d.status}</span></td>
        <td><code>${esc((d.git_commit_hash || "").slice(0, 8)) || "—"}</code></td>
        <td>${d.github_repo ? `<a href="https://github.com/${esc(d.github_repo)}" target="_blank">${esc(d.github_repo)}</a>` : '<span class="muted">local</span>'}</td>
        <td class="muted" style="max-width:260px">${esc(d.error_message)}</td>
        <td>${d.status === "succeeded" ? `<button class="btn small" data-rb="${d.id}">Rollback to this</button>` : ""}</td>
      </tr>`).join("")}
    </table></div>`;
  body.querySelectorAll("[data-rb]").forEach(el => el.onclick = async () => {
    if (!confirm("Republish this snapshot?")) return;
    try {
      const d = await api(tenantPath(`/deployments/${el.dataset.rb}/rollback`), { method: "POST" });
      toast("Rollback started");
      watchDeployment(d.id);
    } catch (e) { toast(e.message, true); }
  });
}

/* ---------- page builder ---------- */

async function renderPageBuilder(main) {
  main.innerHTML = "Loading…";
  try {
    if (S.sectionTypes.length === 0) await loadSectionTypes();
    S.sections = await api(tenantPath(`/pages/${S.arg}/sections`));
    const pages = await api(tenantPath(`/websites/${S.website.id}/pages`));
    S.page = pages.find(p => p.id === S.arg);
    if (!S.page) throw new Error("page not found");
  } catch (e) { main.innerHTML = `<div class="empty">${esc(e.message)}</div>`; return; }

  S.sectionEdits = {};
  S.sections.forEach(s => S.sectionEdits[s.id] = JSON.parse(JSON.stringify(s.content || {})));
  if (!S.sections.find(s => s.id === S.selectedSection)) S.selectedSection = null;

  main.innerHTML = `
    <div class="page-head">
      <h1><a href="#" id="back-link">← ${esc(S.website.name)}</a> / ${esc(S.page.title)}</h1>
      <div class="actions">
        <button class="btn" id="page-meta">Page settings</button>
        <button class="btn ${S.previewOn ? "primary" : ""}" id="toggle-preview">
          ${S.previewOn ? "Hide preview" : "Live preview"}</button>
      </div>
    </div>
    <div class="builder">
      <aside class="palette">
        <h3>Blocks</h3>
        <div id="palette-list"></div>
        <div class="hint-box">Drag a block into the page, or click to add it at the end.
          Drag blocks in the page to reorder.</div>
      </aside>
      <div class="canvas" id="canvas"></div>
      <aside class="inspector" id="inspector"></aside>
    </div>
    <div class="preview-pane" id="preview-pane" ${S.previewOn ? "" : "hidden"}>
      <div class="pp-head">Live preview <span class="spacer"></span>
        <button class="btn small" id="refresh-preview">Refresh</button></div>
      <iframe id="preview-frame"></iframe>
    </div>`;

  document.getElementById("back-link").onclick = e => { e.preventDefault(); nav("website", S.website.id); };
  document.getElementById("page-meta").onclick = () => pageFormModal(S.page);
  document.getElementById("toggle-preview").onclick = () => {
    S.previewOn = !S.previewOn;
    document.getElementById("preview-pane").hidden = !S.previewOn;
    document.getElementById("toggle-preview").textContent = S.previewOn ? "Hide preview" : "Live preview";
    document.getElementById("toggle-preview").classList.toggle("primary", S.previewOn);
    if (S.previewOn) refreshPreview();
  };
  document.getElementById("refresh-preview").onclick = refreshPreview;

  renderPalette();
  renderCanvas();
  renderInspector();
  if (S.previewOn) refreshPreview();
}

async function refreshPreview() {
  const frame = document.getElementById("preview-frame");
  if (!frame || !S.previewOn) return;
  try {
    const res = await api(tenantPath(`/websites/${S.website.id}/preview`), { method: "POST" });
    const path = (S.page.slug === "home" || S.page.slug === "") ? "" : S.page.slug + "/";
    frame.src = res.url + path;
  } catch (e) { toast("Preview failed: " + e.message, true); }
}

function renderPalette() {
  const box = document.getElementById("palette-list");
  box.innerHTML = S.sectionTypes.map(t => `
    <div class="palette-item" draggable="true" data-type="${esc(t.type_key)}">
      <span>${esc(t.icon || "▦")}</span> ${esc(t.label)}
    </div>`).join("") || `<div class="muted">No block types defined.</div>`;
  box.querySelectorAll(".palette-item").forEach(el => {
    el.ondragstart = e => e.dataTransfer.setData("text/plain", "new:" + el.dataset.type);
    el.onclick = () => addBlock(el.dataset.type, S.sections.length);
  });
}

function blockSummary(sec) {
  const spec = S.sectionTypes.find(t => t.type_key === sec.section_type);
  const content = S.sectionEdits[sec.id] || {};
  for (const f of (spec?.fields || [])) {
    if (["heading", "text", "textarea"].includes(f.type) && content[f.key]) {
      return String(content[f.key]).slice(0, 70);
    }
  }
  return "";
}

function renderCanvas() {
  const canvas = document.getElementById("canvas");
  canvas.innerHTML = "";
  if (S.sections.length === 0) {
    canvas.innerHTML = `<div class="empty">Drag blocks here to build the page.</div>`;
  }
  S.sections.forEach((sec, i) => {
    const spec = S.sectionTypes.find(t => t.type_key === sec.section_type);
    const card = document.createElement("div");
    card.className = "block-card" + (sec.id === S.selectedSection ? " selected" : "");
    card.draggable = true;
    card.dataset.id = sec.id;
    card.dataset.index = i;
    card.innerHTML = `
      <span class="grip">⠿</span>
      <span class="b-label">${esc(spec?.icon || "▦")} ${esc(spec ? spec.label : sec.section_type)}</span>
      <span class="b-summary">${esc(blockSummary(sec))}</span>
      <span class="badge ${sec.status}">${sec.status}</span>`;
    card.onclick = () => { S.selectedSection = sec.id; renderCanvas(); renderInspector(); };
    card.ondragstart = e => {
      e.dataTransfer.setData("text/plain", "move:" + sec.id);
      card.classList.add("dragging");
    };
    card.ondragend = () => card.classList.remove("dragging");
    canvas.appendChild(card);
  });

  let indicator = null;
  const clearIndicator = () => { indicator?.remove(); indicator = null; };
  const dropIndex = e => {
    const cards = [...canvas.querySelectorAll(".block-card:not(.dragging)")];
    for (let i = 0; i < cards.length; i++) {
      const r = cards[i].getBoundingClientRect();
      if (e.clientY < r.top + r.height / 2) return i;
    }
    return cards.length;
  };
  canvas.ondragover = e => {
    e.preventDefault();
    canvas.classList.add("drag-over");
    clearIndicator();
    indicator = document.createElement("div");
    indicator.className = "drop-indicator";
    const cards = [...canvas.querySelectorAll(".block-card:not(.dragging)")];
    const idx = dropIndex(e);
    if (idx >= cards.length) canvas.appendChild(indicator);
    else canvas.insertBefore(indicator, cards[idx]);
  };
  canvas.ondragleave = e => {
    if (!canvas.contains(e.relatedTarget)) { canvas.classList.remove("drag-over"); clearIndicator(); }
  };
  canvas.ondrop = async e => {
    e.preventDefault();
    canvas.classList.remove("drag-over");
    const idx = dropIndex(e);
    clearIndicator();
    const data = e.dataTransfer.getData("text/plain");
    if (data.startsWith("new:")) {
      await addBlock(data.slice(4), idx);
    } else if (data.startsWith("move:")) {
      const id = data.slice(5);
      const from = S.sections.findIndex(s => s.id === id);
      if (from === -1) return;
      const ids = S.sections.map(s => s.id);
      ids.splice(from, 1);
      ids.splice(idx > from ? idx - 1 : idx, 0, id);
      try {
        await api(tenantPath(`/pages/${S.page.id}/sections/order`), {
          method: "PUT", body: { section_ids: ids },
        });
        S.sections.sort((a, b) => ids.indexOf(a.id) - ids.indexOf(b.id));
        renderCanvas();
        if (S.previewOn) refreshPreview();
      } catch (err) { toast(err.message, true); }
    }
  };
}

async function addBlock(typeKey, index) {
  try {
    const sec = await api(tenantPath(`/pages/${S.page.id}/sections`), {
      method: "POST", body: { section_type: typeKey, content: {} },
    });
    S.sections.push(sec);
    S.sectionEdits[sec.id] = {};
    if (index < S.sections.length - 1) {
      const ids = S.sections.map(s => s.id);
      ids.splice(ids.indexOf(sec.id), 1);
      ids.splice(index, 0, sec.id);
      await api(tenantPath(`/pages/${S.page.id}/sections/order`), {
        method: "PUT", body: { section_ids: ids },
      });
      S.sections.sort((a, b) => ids.indexOf(a.id) - ids.indexOf(b.id));
    }
    S.selectedSection = sec.id;
    renderCanvas();
    renderInspector();
  } catch (e) { toast(e.message, true); }
}

function renderInspector() {
  const box = document.getElementById("inspector");
  const sec = S.sections.find(s => s.id === S.selectedSection);
  if (!sec) {
    box.innerHTML = `<div class="placeholder">Select a block to edit its content.</div>`;
    return;
  }
  const spec = S.sectionTypes.find(t => t.type_key === sec.section_type);
  box.innerHTML = `
    <div class="ins-head">
      <strong>${esc(spec?.icon || "")} ${esc(spec ? spec.label : sec.section_type)}</strong>
      <button class="btn small" id="ins-toggle">${sec.status === "visible" ? "Hide" : "Show"}</button>
      <button class="btn small danger" id="ins-delete">Delete</button>
    </div>
    <div id="ins-fields"></div>
    <div class="row mt">
      <button class="btn primary" id="ins-save" style="flex:1">Save block</button>
    </div>`;

  const content = S.sectionEdits[sec.id];
  const fieldsBox = box.querySelector("#ins-fields");
  (spec?.fields || []).forEach(f => fieldsBox.appendChild(fieldEditor(f, content)));

  box.querySelector("#ins-save").onclick = async () => {
    try {
      await api(tenantPath(`/sections/${sec.id}`), { method: "PATCH", body: { content } });
      toast("Block saved");
      renderCanvas();
      if (S.previewOn) refreshPreview();
    } catch (e) { toast(e.message, true); }
  };
  box.querySelector("#ins-toggle").onclick = async () => {
    try {
      const updated = await api(tenantPath(`/sections/${sec.id}`), {
        method: "PATCH", body: { status: sec.status === "visible" ? "hidden" : "visible" },
      });
      sec.status = updated.status;
      renderCanvas(); renderInspector();
      if (S.previewOn) refreshPreview();
    } catch (e) { toast(e.message, true); }
  };
  box.querySelector("#ins-delete").onclick = async () => {
    if (!confirm("Delete this block?")) return;
    try {
      await api(tenantPath(`/sections/${sec.id}`), { method: "DELETE" });
      S.sections = S.sections.filter(s => s.id !== sec.id);
      S.selectedSection = null;
      renderCanvas(); renderInspector();
      if (S.previewOn) refreshPreview();
    } catch (e) { toast(e.message, true); }
  };
}

/* fieldEditor renders one schema field bound to obj[f.key]. */
function fieldEditor(f, obj) {
  const wrap = document.createElement("div");
  const label = document.createElement("label");
  label.textContent = f.label;
  wrap.appendChild(label);

  if (f.type === "list") {
    const listBox = document.createElement("div");
    if (!Array.isArray(obj[f.key])) obj[f.key] = [];
    const renderList = () => {
      listBox.innerHTML = "";
      obj[f.key].forEach((item, idx) => {
        const itemBox = document.createElement("div");
        itemBox.className = "list-item";
        const rm = document.createElement("button");
        rm.type = "button"; rm.className = "btn small danger remove-item"; rm.textContent = "✕";
        rm.onclick = () => { obj[f.key].splice(idx, 1); renderList(); };
        itemBox.appendChild(rm);
        (f.fields || []).forEach(sub => itemBox.appendChild(fieldEditor(sub, item)));
        listBox.appendChild(itemBox);
      });
      const add = document.createElement("button");
      add.type = "button"; add.className = "btn small"; add.textContent = "+ Add item";
      add.onclick = () => { obj[f.key].push({}); renderList(); };
      listBox.appendChild(add);
    };
    renderList();
    wrap.appendChild(listBox);
    return wrap;
  }

  if (f.type === "image") {
    const field = document.createElement("div");
    field.className = "image-field";
    renderImageField(field, obj[f.key], id => { obj[f.key] = id || ""; });
    wrap.appendChild(field);
    return wrap;
  }

  if (f.type === "button") {
    if (typeof obj[f.key] !== "object" || obj[f.key] === null) obj[f.key] = { text: "", link: "" };
    const btnObj = obj[f.key];
    const text = document.createElement("input");
    text.placeholder = "Button text";
    text.value = btnObj.text ?? "";
    text.oninput = () => btnObj.text = text.value;
    const link = document.createElement("input");
    link.placeholder = "https://… or /page-slug/";
    link.style.marginTop = ".35rem";
    link.value = btnObj.link ?? "";
    link.oninput = () => btnObj.link = link.value;
    wrap.append(text, link);
    return wrap;
  }

  if (f.type === "contact_info") {
    const note = document.createElement("div");
    note.className = "muted";
    note.textContent = "Shows the website's contact details (phone, email, WhatsApp, address) from Settings.";
    wrap.appendChild(note);
    return wrap;
  }

  if (f.type === "bool") {
    const input = document.createElement("input");
    input.type = "checkbox";
    input.style.width = "auto";
    input.checked = !!obj[f.key];
    input.onchange = () => obj[f.key] = input.checked;
    label.prepend(input, " ");
    label.style.display = "flex";
    label.style.alignItems = "center";
    label.style.gap = ".4rem";
    return wrap;
  }

  let input;
  if (f.type === "textarea") input = document.createElement("textarea");
  else if (f.type === "select") {
    input = document.createElement("select");
    (f.options || []).forEach(o => {
      const opt = document.createElement("option");
      opt.value = o; opt.textContent = o;
      input.appendChild(opt);
    });
  } else {
    input = document.createElement("input");
    if (f.type === "url") input.placeholder = "https://… or /page-slug/";
  }
  input.value = obj[f.key] ?? "";
  input.oninput = () => obj[f.key] = input.value;
  wrap.appendChild(input);
  return wrap;
}

/* renderImageField shows a thumbnail + choose/clear; onPick(idOrNull). */
async function renderImageField(container, mediaId, onPick) {
  container.innerHTML = "";
  if (mediaId) {
    const img = document.createElement("img");
    img.alt = "";
    api(tenantPath("/media")).then(items => {
      const m = items.find(x => x.id === mediaId);
      if (m) img.src = m.public_url;
    }).catch(() => {});
    container.appendChild(img);
  }
  const choose = document.createElement("button");
  choose.type = "button"; choose.className = "btn small";
  choose.textContent = mediaId ? "Change" : "Choose image";
  choose.onclick = () => mediaPicker(m => { onPick(m.id); renderImageField(container, m.id, onPick); });
  container.appendChild(choose);
  if (mediaId) {
    const clear = document.createElement("button");
    clear.type = "button"; clear.className = "btn small danger"; clear.textContent = "Clear";
    clear.onclick = () => { onPick(null); renderImageField(container, null, onPick); };
    container.appendChild(clear);
  }
}

/* ---------- block types (schema manager) ---------- */

const FIELD_TYPE_LABELS = {
  heading: "Heading", text: "Text (one line)", textarea: "Text (multi-line)",
  image: "Image", button: "Button", url: "URL", select: "Dropdown",
  bool: "Checkbox", contact_info: "Site contact details", list: "List of items",
};

async function renderBlockTypes(main) {
  main.innerHTML = `<div class="page-head"><h1>Block types</h1>
    <div class="actions"><button class="btn primary" id="new-type">+ New block type</button></div></div>
    <p class="muted">Block types define the structured fields your pages are built from.
      Changes apply on the next publish.</p>
    <div id="types-box">Loading…</div>`;
  document.getElementById("new-type").onclick = () => typeEditorModal(null, () => renderBlockTypes(main));
  const box = document.getElementById("types-box");
  try {
    await loadSectionTypes();
    box.innerHTML = S.sectionTypes.length === 0
      ? `<div class="empty">No block types yet.</div>`
      : `<div class="table-wrap"><table>
        <tr><th></th><th>Label</th><th>Key</th><th>Layout</th><th>Fields</th><th></th></tr>
        ${S.sectionTypes.map(t => `<tr>
          <td>${esc(t.icon)}</td>
          <td><b>${esc(t.label)}</b></td>
          <td><code>${esc(t.type_key)}</code></td>
          <td>${esc(t.layout?.variant || "default")}</td>
          <td class="muted">${t.fields.map(f => esc(f.key)).join(", ")}</td>
          <td class="row">
            <button class="btn small" data-edit="${t.id}">Edit</button>
            <button class="btn small danger" data-arch="${t.id}">Archive</button>
          </td>
        </tr>`).join("")}
      </table></div>`;
    box.querySelectorAll("[data-edit]").forEach(b => b.onclick = () =>
      typeEditorModal(S.sectionTypes.find(t => t.id === b.dataset.edit), () => renderBlockTypes(main)));
    box.querySelectorAll("[data-arch]").forEach(b => b.onclick = async () => {
      if (!confirm("Archive this block type? It will disappear from the palette.")) return;
      try {
        await api(tenantPath(`/section-types/${b.dataset.arch}`), { method: "DELETE" });
        renderBlockTypes(main);
      } catch (e) { toast(e.message, true); }
    });
  } catch (e) { box.innerHTML = `<div class="empty">${esc(e.message)}</div>`; }
}

function typeEditorModal(type, done) {
  const draft = type
    ? JSON.parse(JSON.stringify({ label: type.label, icon: type.icon, layout: type.layout || {}, fields: type.fields }))
    : { label: "", icon: "▦", layout: { variant: "default" }, fields: [] };

  const m = modal(`
    <h2>${type ? "Edit block type" : "New block type"}</h2>
    <div class="grid-2">
      <label>Label <input id="bt-label" value="${esc(draft.label)}"></label>
      <label>Key <span class="hint">(used internally${type ? ", cannot change" : ""})</span>
        <input id="bt-key" ${type ? "disabled" : ""} value="${esc(type?.type_key)}"></label>
      <label>Icon <span class="hint">(emoji)</span> <input id="bt-icon" value="${esc(draft.icon)}"></label>
      <label>Layout
        <select id="bt-variant">
          ${["default", "banner", "cards", "gallery", "accordion", "cta"].map(v =>
            `<option value="${v}" ${draft.layout.variant === v ? "selected" : ""}>${v}</option>`).join("")}
        </select></label>
      <label>Background
        <select id="bt-bg">
          ${["", "alt", "primary"].map(v =>
            `<option value="${v}" ${(draft.layout.background || "") === v ? "selected" : ""}>${v || "default"}</option>`).join("")}
        </select></label>
      <label>Text alignment
        <select id="bt-align">
          ${["", "left", "center", "right"].map(v =>
            `<option value="${v}" ${(draft.layout.align || "") === v ? "selected" : ""}>${v || "default"}</option>`).join("")}
        </select></label>
    </div>
    <h3>Fields</h3>
    <div id="bt-fields"></div>
    <button type="button" class="btn small" id="bt-add-field">+ Add field</button>
    <div class="row mt">
      <button class="btn primary" id="bt-save">${type ? "Save changes" : "Create block type"}</button>
      <button type="button" class="btn" data-close>Cancel</button>
    </div>`);

  const labelInput = m.querySelector("#bt-label");
  const keyInput = m.querySelector("#bt-key");
  if (!type) labelInput.oninput = () => { keyInput.value = slugify(labelInput.value); };

  const fieldsBox = m.querySelector("#bt-fields");
  const renderFields = () => {
    fieldsBox.innerHTML = "";
    draft.fields.forEach((f, i) => fieldsBox.appendChild(schemaFieldRow(draft.fields, f, i, renderFields, true)));
    if (draft.fields.length === 0) fieldsBox.innerHTML = `<p class="muted">No fields yet.</p>`;
  };
  m.querySelector("#bt-add-field").onclick = () => {
    draft.fields.push({ key: "", label: "", type: "text" });
    renderFields();
  };
  renderFields();

  m.querySelector("#bt-save").onclick = async () => {
    const body = {
      type_key: type ? type.type_key : keyInput.value.trim(),
      label: labelInput.value.trim(),
      icon: m.querySelector("#bt-icon").value.trim(),
      layout: {
        variant: m.querySelector("#bt-variant").value,
        background: m.querySelector("#bt-bg").value,
        align: m.querySelector("#bt-align").value,
      },
      fields: draft.fields,
    };
    try {
      if (type) await api(tenantPath(`/section-types/${type.id}`), { method: "PATCH", body });
      else await api(tenantPath("/section-types"), { method: "POST", body });
      m.remove();
      toast("Block type saved");
      done && done();
    } catch (e) { toast(e.message, true); }
  };
}

/* schemaFieldRow renders one field definition row (recursive for lists). */
function schemaFieldRow(arr, f, i, rerender, allowList) {
  const row = document.createElement("div");
  row.className = "schema-field";
  const types = Object.keys(FIELD_TYPE_LABELS).filter(t => allowList || t !== "list");
  row.innerHTML = `
    <div class="sf-row">
      <label>Label <input data-k="label" value="${esc(f.label)}"></label>
      <label>Key <input data-k="key" value="${esc(f.key)}"></label>
      <label>Type
        <select data-k="type">
          ${types.map(t => `<option value="${t}" ${f.type === t ? "selected" : ""}>${FIELD_TYPE_LABELS[t]}</option>`).join("")}
        </select></label>
      <div class="row">
        <button type="button" class="btn small" data-a="up" ${i === 0 ? "disabled" : ""}>↑</button>
        <button type="button" class="btn small" data-a="down" ${i === arr.length - 1 ? "disabled" : ""}>↓</button>
        <button type="button" class="btn small danger" data-a="rm">✕</button>
      </div>
    </div>
    <div class="sf-extra"></div>`;

  const labelIn = row.querySelector('[data-k="label"]');
  const keyIn = row.querySelector('[data-k="key"]');
  labelIn.oninput = () => {
    f.label = labelIn.value;
    if (!f._keyTouched) { f.key = slugify(labelIn.value); keyIn.value = f.key; }
  };
  keyIn.oninput = () => { f.key = keyIn.value; f._keyTouched = true; };
  row.querySelector('[data-k="type"]').onchange = e => { f.type = e.target.value; renderExtra(); };
  row.querySelectorAll("[data-a]").forEach(b => b.onclick = () => {
    if (b.dataset.a === "rm") arr.splice(i, 1);
    else {
      const j = b.dataset.a === "up" ? i - 1 : i + 1;
      [arr[i], arr[j]] = [arr[j], arr[i]];
    }
    rerender();
  });

  const extra = row.querySelector(".sf-extra");
  const renderExtra = () => {
    extra.innerHTML = "";
    if (f.type === "select") {
      const opts = document.createElement("input");
      opts.placeholder = "Options, comma-separated (e.g. left, center, right)";
      opts.value = (f.options || []).join(", ");
      opts.oninput = () => f.options = opts.value.split(",").map(s => s.trim()).filter(Boolean);
      extra.appendChild(opts);
    } else if (f.type === "list") {
      if (!Array.isArray(f.fields)) f.fields = [];
      const sub = document.createElement("div");
      const renderSub = () => {
        sub.innerHTML = "<label class='muted'>Item fields</label>";
        f.fields.forEach((sf, si) => sub.appendChild(schemaFieldRow(f.fields, sf, si, renderSub, false)));
        const add = document.createElement("button");
        add.type = "button"; add.className = "btn small"; add.textContent = "+ Add item field";
        add.onclick = () => { f.fields.push({ key: "", label: "", type: "text" }); renderSub(); };
        sub.appendChild(add);
      };
      renderSub();
      extra.appendChild(sub);
    }
  };
  renderExtra();
  return row;
}

/* ---------- media ---------- */

function mediaCardHTML(m, selectable) {
  const isImage = m.file_type.startsWith("image/");
  return `<div class="media-card${selectable ? " selectable" : ""}" data-id="${m.id}">
    ${isImage ? `<img src="${esc(m.public_url)}" alt="${esc(m.alt_text)}">`
      : `<div class="file-icon">📄</div>`}
    <div class="meta"><div class="name" title="${esc(m.file_name)}">${esc(m.file_name)}</div>
      <div class="muted">${(m.file_size / 1024).toFixed(0)} KB</div></div>
  </div>`;
}

async function renderMediaView(main) {
  main.innerHTML = `<div class="page-head"><h1>Media library</h1>
    <div class="actions">
      <label class="btn primary" style="margin:0">Upload
        <input type="file" id="upload-input" accept="image/*,.pdf" style="display:none">
      </label>
    </div></div>
    <div id="media-box">Loading…</div>`;
  document.getElementById("upload-input").onchange = e => uploadFile(e.target.files[0], () => renderMediaView(main));
  const box = document.getElementById("media-box");
  try {
    const items = await api(tenantPath("/media"));
    box.innerHTML = items.length === 0 ? `<div class="empty">No media yet — upload images to use in blocks.</div>`
      : `<div class="media-grid">${items.map(m => mediaCardHTML(m, false)).join("")}</div>`;
    box.querySelectorAll(".media-card").forEach(card => {
      card.onclick = () => {
        const m = items.find(x => x.id === card.dataset.id);
        mediaDetailModal(m, () => renderMediaView(main));
      };
    });
  } catch (e) { box.innerHTML = `<div class="empty">${esc(e.message)}</div>`; }
}

async function uploadFile(file, done) {
  if (!file) return;
  const fd = new FormData();
  fd.append("file", file);
  try {
    await api(tenantPath("/media"), { method: "POST", body: fd });
    toast("Uploaded " + file.name);
    done && done();
  } catch (e) { toast(e.message, true); }
}

function mediaDetailModal(m, done) {
  const mod = modal(`
    <h2>${esc(m.file_name)}</h2>
    ${m.file_type.startsWith("image/") ? `<img src="${esc(m.public_url)}" style="max-width:100%;max-height:300px;border-radius:.5rem">` : ""}
    <p class="muted">${esc(m.file_type)} · ${(m.file_size / 1024).toFixed(0)} KB<br>
      <a href="${esc(m.public_url)}" target="_blank">${esc(m.public_url)}</a></p>
    <label>Alt text <input id="alt-input" value="${esc(m.alt_text)}"></label>
    <div class="row mt">
      <button class="btn primary" id="save-alt">Save</button>
      <button class="btn danger" id="del-media">Delete</button>
      <span class="spacer"></span>
      <button class="btn" data-close>Close</button>
    </div>`);
  mod.querySelector("#save-alt").onclick = async () => {
    try {
      await api(tenantPath(`/media/${m.id}`), { method: "PATCH", body: { alt_text: mod.querySelector("#alt-input").value } });
      toast("Saved"); mod.remove(); done && done();
    } catch (e) { toast(e.message, true); }
  };
  mod.querySelector("#del-media").onclick = async () => {
    if (!confirm("Delete this file? Blocks referencing it will lose the image.")) return;
    try { await api(tenantPath(`/media/${m.id}`), { method: "DELETE" }); mod.remove(); done && done(); }
    catch (e) { toast(e.message, true); }
  };
}

function mediaPicker(onPick) {
  const mod = modal(`
    <h2>Choose media</h2>
    <div class="row" style="margin-bottom:1rem">
      <label class="btn primary" style="margin:0">Upload new
        <input type="file" id="picker-upload" accept="image/*" style="display:none">
      </label>
      <span class="spacer"></span><button class="btn" data-close>Cancel</button>
    </div>
    <div id="picker-grid">Loading…</div>`);
  const load = async () => {
    try {
      const items = await api(tenantPath("/media"));
      const images = items.filter(m => m.file_type.startsWith("image/"));
      const grid = mod.querySelector("#picker-grid");
      grid.innerHTML = images.length === 0 ? `<div class="empty">No images uploaded yet.</div>`
        : `<div class="media-grid">${images.map(m => mediaCardHTML(m, true)).join("")}</div>`;
      grid.querySelectorAll(".media-card").forEach(card => card.onclick = () => {
        onPick(images.find(x => x.id === card.dataset.id));
        mod.remove();
      });
    } catch (e) { toast(e.message, true); }
  };
  mod.querySelector("#picker-upload").onchange = e => uploadFile(e.target.files[0], load);
  load();
}

/* ---------- users ---------- */

async function renderUsers(main) {
  main.innerHTML = `<div class="page-head"><h1>Users</h1>
    <div class="actions"><button class="btn primary" id="invite-btn">+ Invite user</button></div></div>
    <div id="users-box">Loading…</div>`;
  document.getElementById("invite-btn").onclick = () => {
    const mod = modal(`
      <h2>Invite user</h2>
      <form id="invite-form">
        <label>Name <input name="name"></label>
        <label>Email <input name="email" type="email" required></label>
        <label>Role
          <select name="role">
            <option value="viewer">Viewer — read only</option>
            <option value="editor" selected>Editor — edit content</option>
            <option value="tenant_admin">Tenant admin — full control</option>
          </select></label>
        <div class="row mt"><button class="btn primary">Invite</button>
          <button type="button" class="btn" data-close>Cancel</button></div>
      </form>`);
    mod.querySelector("#invite-form").onsubmit = async e => {
      e.preventDefault();
      const f = new FormData(e.target);
      try {
        const res = await api(tenantPath("/users"), {
          method: "POST",
          body: { name: f.get("name"), email: f.get("email"), role: f.get("role") },
        });
        mod.remove();
        if (res.temporary_password) {
          modal(`<h2>User created</h2>
            <p>Share these credentials securely — the password is shown only once:</p>
            <p>Email: <code>${esc(f.get("email"))}</code><br>
            Temporary password: <code>${esc(res.temporary_password)}</code></p>
            <button class="btn primary" data-close>Done</button>`);
        } else toast("User added to tenant");
        renderUsers(main);
      } catch (err) { toast(err.message, true); }
    };
  };

  const box = document.getElementById("users-box");
  try {
    const users = await api(tenantPath("/users"));
    box.innerHTML = users.length === 0 ? `<div class="empty">No users assigned to this tenant.</div>` : `
      <div class="table-wrap"><table>
        <tr><th>Name</th><th>Email</th><th>Role</th><th></th></tr>
        ${users.map(u => `<tr>
          <td>${esc(u.user_name)}</td><td>${esc(u.user_email)}</td>
          <td><select data-role="${u.user_id}">
            ${["viewer", "editor", "tenant_admin"].map(r =>
              `<option value="${r}" ${u.role === r ? "selected" : ""}>${r}</option>`).join("")}
          </select></td>
          <td><button class="btn small danger" data-remove="${u.user_id}">Remove</button></td>
        </tr>`).join("")}
      </table></div>`;
    box.querySelectorAll("[data-role]").forEach(sel => sel.onchange = async () => {
      try {
        await api(tenantPath(`/users/${sel.dataset.role}`), { method: "PATCH", body: { role: sel.value } });
        toast("Role updated");
      } catch (e) { toast(e.message, true); renderUsers(main); }
    });
    box.querySelectorAll("[data-remove]").forEach(btn => btn.onclick = async () => {
      if (!confirm("Remove this user from the tenant?")) return;
      try { await api(tenantPath(`/users/${btn.dataset.remove}`), { method: "DELETE" }); renderUsers(main); }
      catch (e) { toast(e.message, true); }
    });
  } catch (e) { box.innerHTML = `<div class="empty">${esc(e.message)}</div>`; }
}

/* ---------- audit ---------- */

async function renderAudit(main) {
  main.innerHTML = `<h1>Audit log</h1><div id="audit-box">Loading…</div>`;
  const box = document.getElementById("audit-box");
  try {
    const logs = await api(tenantPath("/audit-logs"));
    box.innerHTML = logs.length === 0 ? `<div class="empty">No activity recorded yet.</div>` : `
      <div class="table-wrap"><table>
        <tr><th>When</th><th>User</th><th>Action</th><th>Entity</th><th>Details</th></tr>
        ${logs.map(l => `<tr>
          <td>${fmtDate(l.created_at)}</td>
          <td>${esc(l.user_email) || '<span class="muted">system</span>'}</td>
          <td><code>${esc(l.action)}</code></td>
          <td class="muted">${esc(l.entity_type)}</td>
          <td class="muted" style="max-width:280px;overflow-wrap:anywhere">
            ${esc(JSON.stringify(l.metadata)) === "{}" ? "" : esc(JSON.stringify(l.metadata))}</td>
        </tr>`).join("")}
      </table></div>`;
  } catch (e) { box.innerHTML = `<div class="empty">${esc(e.message)}</div>`; }
}

/* ---------- tenants (platform admin) ---------- */

async function renderTenants(main) {
  main.innerHTML = `<div class="page-head"><h1>Tenants</h1>
    <div class="actions"><button class="btn primary" id="new-tenant">+ New tenant</button></div></div>
    <div id="tenants-box">Loading…</div>`;
  document.getElementById("new-tenant").onclick = async () => {
    const name = prompt("Tenant (client/business) name:");
    if (!name) return;
    try {
      const t = await api("/tenants", { method: "POST", body: { name } });
      S.memberships.push({ tenant_id: t.id, tenant_name: t.name, role: "tenant_admin" });
      S.tenantId = t.id;
      S.sectionTypes = [];
      localStorage.setItem("cms_tenant", t.id);
      nav("websites");
    } catch (e) { toast(e.message, true); }
  };
  const box = document.getElementById("tenants-box");
  try {
    const tenants = await api("/tenants");
    box.innerHTML = `<div class="table-wrap"><table>
      <tr><th>Name</th><th>Status</th><th>Created</th><th></th></tr>
      ${tenants.map(t => `<tr>
        <td><b>${esc(t.name)}</b></td>
        <td><span class="badge ${t.status}">${t.status}</span></td>
        <td>${fmtDate(t.created_at)}</td>
        <td><button class="btn small" data-status="${t.status === "active" ? "disabled" : "active"}"
          data-id="${t.id}">${t.status === "active" ? "Disable" : "Enable"}</button></td>
      </tr>`).join("")}
    </table></div>`;
    box.querySelectorAll("[data-id]").forEach(btn => btn.onclick = async () => {
      try {
        await api(`/tenants/${btn.dataset.id}`, { method: "PATCH", body: { status: btn.dataset.status } });
        renderTenants(main);
      } catch (e) { toast(e.message, true); }
    });
  } catch (e) { box.innerHTML = `<div class="empty">${esc(e.message)}</div>`; }
}

/* ---------- modal helper ---------- */

function modal(html) {
  const backdrop = document.createElement("div");
  backdrop.className = "modal-backdrop";
  backdrop.innerHTML = `<div class="modal">${html}</div>`;
  backdrop.onclick = e => { if (e.target === backdrop) backdrop.remove(); };
  backdrop.querySelectorAll("[data-close]").forEach(b => b.onclick = () => backdrop.remove());
  document.body.appendChild(backdrop);
  return backdrop;
}

boot();
