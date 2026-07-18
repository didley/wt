// wt GUI — vanilla JS over the Wails-bound Go App (window.go.main.App).
"use strict";

const api = () => window.go.main.App;
const $ = (id) => document.getElementById(id);

let repo = null; // current RepoView
let recents = [];
let selected = new Set(); // paths of worktrees checked for bulk removal
let appVersion = "dev";
let appOS = "linux";

// ---------- boot ----------

window.addEventListener("DOMContentLoaded", async () => {
  wireStaticHandlers();
  try {
    appVersion = await api().Version();
    appOS = await api().OS();
  } catch {
    // leave the defaults
  }
  try {
    recents = await api().RecentRepos();
    renderRecents();
    if (recents.length > 0) await loadRepo(recents[0]);
  } catch (e) {
    toast(String(e), true);
  }
  startAutoRefresh();
});

// ---------- auto refresh ----------
//
// git runs on the host (flatpak-spawn), so an in-sandbox file watcher can't
// see the repo. Instead, poll the same git-backed view and re-render only
// when it actually changed. Paused while hidden or while a dialog is open.

const REFRESH_MS = 2500;

function startAutoRefresh() {
  setInterval(refreshIfChanged, REFRESH_MS);
  window.addEventListener("focus", refreshIfChanged);
}

async function refreshIfChanged() {
  if (!repo || document.hidden || document.querySelector("dialog[open]")) return;
  let fresh;
  try {
    fresh = await api().LoadRepo(repo.mainPath);
  } catch {
    return; // transient (e.g. repo mid-move); next tick will retry
  }
  if (!repo || fresh.mainPath !== repo.mainPath) return; // user switched repos meanwhile
  if (JSON.stringify(fresh) === JSON.stringify(repo)) return;
  repo = fresh;
  renderRepo();
}

function wireStaticHandlers() {
  $("about-btn").addEventListener("click", openAboutDialog);
  // A click that lands on the <dialog> element itself (not its content) is
  // a click on the backdrop, since the dialog fills the content box.
  $("dlg-about").addEventListener("click", (ev) => {
    if (ev.target === $("dlg-about")) $("dlg-about").close();
  });
  $("about-repo-link").addEventListener("click", () => api().OpenURL("https://github.com/didley/wt"));
  $("about-author-link").addEventListener("click", () => api().OpenURL("https://github.com/didley"));
  $("open-repo").addEventListener("click", async () => {
    try {
      const dir = await api().OpenRepoDialog();
      if (dir) await loadRepo(dir);
    } catch (e) {
      toast(String(e), true);
    }
  });
  $("new-worktree").addEventListener("click", openCreateDialog);
  $("move-strays").addEventListener("click", async () => {
    await action(() => api().MoveStrays(repo.mainPath), "Moving worktrees…");
  });
  $("prune-stale").addEventListener("click", async () => {
    await action(() => api().PruneStale(repo.mainPath), "Pruning stale entries…");
  });
  $("create-branch").addEventListener("input", updateCreateHint);
  $("bulk-clear").addEventListener("click", () => {
    selected.clear();
    renderRepo();
  });
  $("bulk-remove").addEventListener("click", openRemoveBulkDialog);
}

// ---------- data / actions ----------

async function loadRepo(path) {
  try {
    repo = await api().LoadRepo(path);
  } catch (e) {
    toast(String(e), true);
    return;
  }
  recents = await api().RecentRepos();
  renderRecents();
  renderRepo();
}

// Runs a mutating call, toasts its message or error, reloads the repo view.
// Shows a busy overlay for the duration so slow git operations (removing
// several worktrees, moving strays, etc.) don't look like nothing happened.
async function action(fn, label = "Working…") {
  setBusy(true, label);
  try {
    const msg = await fn();
    if (msg) toast(msg, false);
  } catch (e) {
    toast(String(e), true);
  }
  if (repo) await loadRepo(repo.mainPath);
  setBusy(false);
}

function setBusy(isBusy, label) {
  $("busy-overlay").hidden = !isBusy;
  if (label) $("busy-label").textContent = label;
}

// ---------- rendering ----------

function renderRecents() {
  const nav = $("recents");
  nav.replaceChildren();
  for (const path of recents) {
    const btn = document.createElement("button");
    btn.className = "recent" + (repo && repo.mainPath === path ? " active" : "");
    btn.title = path;

    const name = document.createElement("span");
    name.className = "name";
    name.textContent = path.split("/").pop();
    btn.appendChild(name);

    const forget = document.createElement("button");
    forget.className = "forget";
    forget.textContent = "✕";
    forget.title = "Remove from recent repos (the repo itself is untouched)";
    forget.addEventListener("click", async (ev) => {
      ev.stopPropagation();
      recents = await api().ForgetRepo(path);
      renderRecents();
    });
    btn.appendChild(forget);

    btn.addEventListener("click", () => loadRepo(path));
    nav.appendChild(btn);
  }
}

function renderRepo() {
  $("empty-state").hidden = true;
  $("repo-view").hidden = false;
  $("repo-name").textContent = repo.name;
  $("repo-path").textContent = repo.mainPath;

  const banner = $("stray-banner");
  banner.hidden = repo.strayCount === 0;
  if (repo.strayCount > 0) {
    $("stray-text").textContent =
      repo.strayCount === 1
        ? "1 worktree lives outside the .worktrees directory"
        : `${repo.strayCount} worktrees live outside the .worktrees directory`;
    $("stray-dir").textContent = repo.worktreesDir;
  }

  const pruneBanner = $("prunable-banner");
  pruneBanner.hidden = repo.prunableCount === 0;
  if (repo.prunableCount > 0) {
    $("prunable-text").textContent =
      repo.prunableCount === 1
        ? "1 worktree entry is stale (its directory is gone)"
        : `${repo.prunableCount} worktree entries are stale (their directories are gone)`;
  }

  const section = $("worktrees");
  // Keep "show changed files" panels open across auto-refresh re-renders.
  const expanded = new Set(
    [...section.querySelectorAll(".card > details[open]")].map((d) => d.closest(".card").dataset.path)
  );
  // section.replaceChildren() below rebuilds every card from scratch, which
  // can leave the scroll container's position stuck after items are removed
  // (see #main's overflow-anchor: none) — restore it explicitly.
  const main = $("main");
  const scrollTop = main.scrollTop;
  section.replaceChildren();
  const livePaths = new Set(repo.worktrees.map((wt) => wt.path));
  for (const path of selected) if (!livePaths.has(path)) selected.delete(path);
  for (const wt of repo.worktrees) section.appendChild(card(wt, expanded.has(wt.path)));
  updateBulkBar();
  main.scrollTop = Math.min(scrollTop, main.scrollHeight - main.clientHeight);
}

function updateBulkBar() {
  const bar = $("bulk-bar");
  bar.hidden = selected.size === 0;
  $("main").classList.toggle("bulk-active", selected.size > 0);
  $("bulk-count").textContent = selected.size === 1 ? "1 worktree selected" : `${selected.size} worktrees selected`;
}

function card(wt, expand) {
  const el = document.createElement("div");
  el.className = "card" + (wt.stray ? " stray" : "");
  el.dataset.path = wt.path;

  const row = document.createElement("div");
  row.className = "card-row";

  if (!wt.isMain) {
    const check = document.createElement("input");
    check.type = "checkbox";
    check.checked = selected.has(wt.path);
    check.title = "Select for bulk removal";
    check.addEventListener("change", () => {
      if (check.checked) selected.add(wt.path);
      else selected.delete(wt.path);
      updateBulkBar();
    });
    row.appendChild(check);
  }

  const name = document.createElement("span");
  name.className = "wt-name";
  name.textContent = wt.name;
  row.appendChild(name);

  if (wt.isMain) row.appendChild(badge("main checkout", "main"));
  if (wt.stray) row.appendChild(badge("outside .worktrees", "stray"));
  if (wt.locked) {
    const lockBadge = badge("locked", "locked");
    if (wt.lockReason) lockBadge.title = wt.lockReason;
    row.appendChild(lockBadge);
  }

  const branch = document.createElement("span");
  branch.className = "branch-chip mono";
  branch.textContent = wt.detached ? "detached HEAD" : wt.branch;
  row.appendChild(branch);

  const state = document.createElement("span");
  state.className = "state " + (wt.dirty ? "dirty" : "clean");
  state.textContent = wt.state;
  row.appendChild(state);
  el.appendChild(row);

  if (wt.changes.length > 0) {
    const det = document.createElement("details");
    det.open = !!expand;
    const sum = document.createElement("summary");
    sum.textContent = "show changed files";
    det.appendChild(sum);
    det.appendChild(changeList(wt.changes));
    el.appendChild(det);
  }

  const actions = document.createElement("div");
  actions.className = "card-actions";
  actions.appendChild(btn("Open", () => api().OpenPath(wt.path).catch((e) => toast(String(e), true))));
  actions.appendChild(
    btn("Copy path", async () => {
      await api().CopyPath(wt.path);
      toast("Path copied.", false);
    })
  );
  if (!wt.isMain) {
    actions.appendChild(btn("Rename…", () => openRenameDialog(wt)));
    if (wt.locked) {
      actions.appendChild(
        btn("Unlock", async () => {
          await action(() => api().UnlockWorktree(repo.mainPath, wt.path), "Unlocking worktree…");
        })
      );
    } else {
      actions.appendChild(btn("Lock…", () => openLockDialog(wt)));
    }
    const rm = btn("Remove…", () => openRemoveDialog(wt));
    rm.classList.add("danger-hover");
    actions.appendChild(rm);
  }
  el.appendChild(actions);
  return el;
}

function changeList(changes) {
  const ul = document.createElement("ul");
  ul.className = "changes";
  for (const c of changes) {
    const li = document.createElement("li");
    const kind = document.createElement("span");
    kind.className = "kind";
    kind.textContent = c.kind;
    const p = document.createElement("span");
    p.className = "mono";
    p.textContent = c.path;
    li.append(kind, p);
    ul.appendChild(li);
  }
  return ul;
}

function badge(text, cls) {
  const b = document.createElement("span");
  b.className = "badge " + cls;
  b.textContent = text;
  return b;
}

function btn(label, onClick) {
  const b = document.createElement("button");
  b.className = "btn";
  b.textContent = label;
  b.addEventListener("click", onClick);
  return b;
}

// ---------- create dialog ----------

function openCreateDialog() {
  const dlg = $("dlg-create");
  $("create-branch").value = "";
  $("create-base").value = repo.defaultBranch;
  const dl = $("branch-options");
  dl.replaceChildren();
  for (const b of repo.availableBranches) {
    const opt = document.createElement("option");
    opt.value = b;
    dl.appendChild(opt);
  }
  updateCreateHint();
  dlg.returnValue = "cancel";
  dlg.onclose = async () => {
    if (dlg.returnValue !== "ok") return;
    const branch = $("create-branch").value.trim();
    const base = $("create-base").value.trim();
    await action(() => api().CreateWorktree(repo.mainPath, branch, base), "Creating worktree…");
  };
  dlg.showModal();
}

function updateCreateHint() {
  const branch = $("create-branch").value.trim();
  const exists = repo && repo.availableBranches.includes(branch);
  $("create-hint").textContent = exists
    ? `Branch "${branch}" already exists — it will be checked out into the new worktree (base ref is ignored).`
    : branch
      ? `A new branch "${branch}" will be created from the base ref below.`
      : "";
  $("create-base-label").style.display = exists ? "none" : "";
}

// ---------- lock dialog ----------

function openLockDialog(wt) {
  const dlg = $("dlg-lock");
  $("lock-name").textContent = wt.name;
  $("lock-reason").value = "";

  dlg.returnValue = "cancel";
  dlg.onclose = async () => {
    if (dlg.returnValue !== "ok") return;
    const reason = $("lock-reason").value.trim();
    await action(() => api().LockWorktree(repo.mainPath, wt.path, reason), "Locking worktree…");
  };
  dlg.showModal();
}

// ---------- remove dialog ----------

function openRemoveDialog(wt) {
  const dlg = $("dlg-remove");
  $("remove-name").textContent = wt.name;

  // Friction point #2, same message as the CLI: the branch survives.
  $("remove-branch-note").textContent = wt.branch
    ? `The branch "${wt.branch}" is NOT deleted — it stays in the repository and can be checked out again from any worktree.`
    : "This worktree is on a detached HEAD; no branch is affected.";

  const lockedBox = $("remove-locked");
  lockedBox.hidden = !wt.locked;
  if (!lockedBox.hidden) {
    $("remove-locked-text").textContent = wt.lockReason
      ? `This worktree is locked (${wt.lockReason}).`
      : "This worktree is locked.";
    $("remove-force-locked").checked = false;
  }

  const dirtyBox = $("remove-dirty");
  dirtyBox.hidden = !wt.dirty || wt.prunable;
  if (!dirtyBox.hidden) {
    const ul = $("remove-changes");
    ul.replaceChildren(...changeList(wt.changes).children);
    dlg.querySelector('input[name="remove-action"][value="stash"]').checked = true;
  }

  const hasBranch = !!wt.branch;
  $("remove-branch-opts").hidden = !hasBranch;
  if (hasBranch) {
    $("remove-branch2").textContent = wt.branch;
    $("remove-del-branch").checked = false;
    $("remove-force-branch").checked = false;
    $("remove-force-wrap").hidden = true;
    $("remove-del-branch").onchange = (ev) => {
      $("remove-force-wrap").hidden = !ev.target.checked;
      if (!ev.target.checked) $("remove-force-branch").checked = false;
    };
  }

  dlg.returnValue = "cancel";
  dlg.onclose = async () => {
    if (dlg.returnValue !== "ok") return;
    const dirty = !dirtyBox.hidden;
    const act = dirty ? dlg.querySelector('input[name="remove-action"]:checked').value : "";
    const del = hasBranch && $("remove-del-branch").checked;
    const force = del && $("remove-force-branch").checked;
    const forceLocked = wt.locked && $("remove-force-locked").checked;
    await action(() => api().RemoveWorktree(repo.mainPath, wt.path, act, del, force, forceLocked), "Removing worktree…");
  };
  dlg.showModal();
}

// ---------- remove (bulk) dialog ----------

function openRemoveBulkDialog() {
  const targets = repo.worktrees.filter((wt) => selected.has(wt.path));
  if (targets.length === 0) return;

  const dlg = $("dlg-remove-bulk");
  $("remove-bulk-count").textContent = targets.length;

  const list = $("remove-bulk-list");
  list.replaceChildren();
  for (const wt of targets) {
    const li = document.createElement("li");
    const name = document.createElement("span");
    name.className = "mono";
    name.textContent = wt.name;
    const branch = document.createElement("span");
    branch.className = "kind";
    branch.textContent = wt.detached ? "detached HEAD" : wt.branch;
    li.append(name, branch);
    list.appendChild(li);
  }

  const lockedTargets = targets.filter((wt) => wt.locked);
  const lockedBox = $("remove-bulk-locked");
  lockedBox.hidden = lockedTargets.length === 0;
  if (!lockedBox.hidden) {
    $("remove-bulk-locked-text").textContent =
      lockedTargets.length === 1
        ? `${lockedTargets[0].name} is locked${lockedTargets[0].lockReason ? ` (${lockedTargets[0].lockReason})` : ""}.`
        : `${lockedTargets.length} of these are locked.`;
    $("remove-bulk-force-locked").checked = false;
  }

  const dirtyTargets = targets.filter((wt) => wt.dirty && !wt.prunable);
  const dirtyBox = $("remove-bulk-dirty");
  dirtyBox.hidden = dirtyTargets.length === 0;
  if (!dirtyBox.hidden) {
    const wrap = $("remove-bulk-changes");
    wrap.replaceChildren();
    for (const wt of dirtyTargets) {
      const name = document.createElement("p");
      name.className = "mono small";
      name.textContent = wt.name;
      wrap.append(name, changeList(wt.changes));
    }
    dlg.querySelector('input[name="remove-bulk-action"][value="stash"]').checked = true;
  }

  const branchTargets = targets.filter((wt) => !!wt.branch);
  $("remove-bulk-branch-opts").hidden = branchTargets.length === 0;
  $("remove-bulk-del-branch").checked = false;
  $("remove-bulk-force-branch").checked = false;
  $("remove-bulk-force-wrap").hidden = true;
  $("remove-bulk-del-branch").onchange = (ev) => {
    $("remove-bulk-force-wrap").hidden = !ev.target.checked;
    if (!ev.target.checked) $("remove-bulk-force-branch").checked = false;
  };

  dlg.returnValue = "cancel";
  dlg.onclose = async () => {
    if (dlg.returnValue !== "ok") return;
    const dirty = !dirtyBox.hidden;
    const act = dirty ? dlg.querySelector('input[name="remove-bulk-action"]:checked').value : "";
    const del = branchTargets.length > 0 && $("remove-bulk-del-branch").checked;
    const force = del && $("remove-bulk-force-branch").checked;
    const forceLocked = lockedTargets.length > 0 && $("remove-bulk-force-locked").checked;
    const paths = targets.map((wt) => wt.path);
    const label = targets.length === 1 ? "Removing worktree…" : `Removing ${targets.length} worktrees…`;
    await action(() => api().RemoveWorktrees(repo.mainPath, paths, act, del, force, forceLocked), label);
    selected.clear();
  };
  dlg.showModal();
}

// ---------- rename dialog ----------

function openRenameDialog(wt) {
  const dlg = $("dlg-rename");
  $("rename-old").textContent = wt.name;
  $("rename-new").value = wt.name;
  $("rename-branch-too").checked = false;
  const hasBranch = !!wt.branch;
  $("rename-branch-name").textContent = hasBranch ? wt.branch : "";
  $("rename-branch-too").parentElement.style.display = hasBranch ? "" : "none";

  dlg.returnValue = "cancel";
  dlg.onclose = async () => {
    if (dlg.returnValue !== "ok") return;
    const newName = $("rename-new").value.trim();
    if (!newName || newName === wt.name) return;
    const renameBranch = hasBranch && $("rename-branch-too").checked;
    await action(() => api().RenameWorktree(repo.mainPath, wt.path, newName, renameBranch), "Renaming worktree…");
  };
  dlg.showModal();
}

// ---------- about dialog ----------

function openAboutDialog() {
  const dlg = $("dlg-about");
  $("about-version").textContent = appVersion === "dev" ? "dev build" : `v${appVersion}`;
  $("about-cli-mac").hidden = appOS !== "darwin";
  $("about-cli-linux").hidden = appOS === "darwin";
  dlg.showModal();
}

// ---------- toasts ----------

function toast(message, isError) {
  const el = document.createElement("div");
  el.className = "toast" + (isError ? " error" : "");
  el.textContent = message.replace(/^Error: /, "");
  $("toasts").appendChild(el);
  setTimeout(() => el.remove(), isError ? 9000 : 5000);
}
