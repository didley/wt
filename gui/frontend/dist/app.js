// wt GUI — vanilla JS over the Wails-bound Go App (window.go.main.App).
"use strict";

const api = () => window.go.main.App;
const $ = (id) => document.getElementById(id);

let repo = null; // current RepoView
let recents = [];

// ---------- boot ----------

window.addEventListener("DOMContentLoaded", async () => {
  wireStaticHandlers();
  try {
    recents = await api().RecentRepos();
    renderRecents();
    if (recents.length > 0) await loadRepo(recents[0]);
  } catch (e) {
    toast(String(e), true);
  }
});

function wireStaticHandlers() {
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
    await action(() => api().MoveStrays(repo.mainPath));
  });
  $("create-branch").addEventListener("input", updateCreateHint);
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
async function action(fn) {
  try {
    const msg = await fn();
    if (msg) toast(msg, false);
  } catch (e) {
    toast(String(e), true);
  }
  if (repo) await loadRepo(repo.mainPath);
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

  const section = $("worktrees");
  section.replaceChildren();
  for (const wt of repo.worktrees) section.appendChild(card(wt));
}

function card(wt) {
  const el = document.createElement("div");
  el.className = "card" + (wt.stray ? " stray" : "");

  const row = document.createElement("div");
  row.className = "card-row";

  const name = document.createElement("span");
  name.className = "wt-name";
  name.textContent = wt.name;
  row.appendChild(name);

  if (wt.isMain) row.appendChild(badge("main checkout", "main"));
  if (wt.stray) row.appendChild(badge("outside .worktrees", "stray"));

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
    await action(() => api().CreateWorktree(repo.mainPath, branch, base));
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

// ---------- remove dialog ----------

function openRemoveDialog(wt) {
  const dlg = $("dlg-remove");
  $("remove-name").textContent = wt.name;

  // Friction point #2, same message as the CLI: the branch survives.
  $("remove-branch-note").textContent = wt.branch
    ? `The branch "${wt.branch}" is NOT deleted — it stays in the repository and can be checked out again from any worktree.`
    : "This worktree is on a detached HEAD; no branch is affected.";

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
    await action(() => api().RemoveWorktree(repo.mainPath, wt.path, act, del, force));
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
    await action(() => api().RenameWorktree(repo.mainPath, wt.path, newName, renameBranch));
  };
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
