/**
 * schedule.js - Scheduled task management drawer
 */

// ===== Init =====

FKTeamsChat.prototype.initSchedule = function () {
  this.scheduleDrawer = document.getElementById("schedule-drawer");
  this.scheduleDrawerClose = document.getElementById("schedule-drawer-close");
  this.scheduleList = document.getElementById("schedule-list");
  this.scheduleFilter = document.getElementById("schedule-filter");
  this.scheduleRefreshBtn = document.getElementById("schedule-refresh-btn");
  this.scheduleManageBtn = document.getElementById("schedule-manage-btn");
  this._scheduleSelected = null;

  if (this.scheduleManageBtn) {
    this.scheduleManageBtn.addEventListener("click", () => this.openScheduleDrawer());
  }
  if (this.scheduleDrawerClose) {
    this.scheduleDrawerClose.addEventListener("click", () => this.closeScheduleDrawer());
  }
  if (this.scheduleDrawer) {
    this.scheduleDrawer.addEventListener("click", (e) => {
      if (e.target === this.scheduleDrawer) this.closeScheduleDrawer();
    });
  }

  var self = this;
  this._scheduleEscHandler = function (e) {
    if (e.key === "Escape" && self.scheduleDrawer && self.scheduleDrawer.style.display !== "none") {
      if (self._scheduleSelected) {
        self._backToScheduleList();
      } else {
        self.closeScheduleDrawer();
      }
    }
  };
  document.addEventListener("keydown", this._scheduleEscHandler);

  if (this.scheduleFilter) {
    this.scheduleFilter.addEventListener("change", () => this.loadScheduleTasks());
  }
  if (this.scheduleRefreshBtn) {
    this.scheduleRefreshBtn.addEventListener("click", () => this.loadScheduleTasks());
  }
};

// ===== Drawer open / close =====

FKTeamsChat.prototype.openScheduleDrawer = function () {
  if (!this.scheduleDrawer) return;
  this.scheduleDrawer.style.display = "flex";
  this._scheduleSelected = null;
  this.loadScheduleTasks();
};

FKTeamsChat.prototype.closeScheduleDrawer = function () {
  if (!this.scheduleDrawer) return;
  var panel = document.getElementById("schedule-drawer-panel");
  var isMobile = window.innerWidth <= 768;
  panel.style.animation = isMobile
    ? "bottomSheetUp 0.2s ease reverse"
    : "drawerSlideIn 0.15s ease reverse";
  panel.style.opacity = "0";
  var self = this;
  setTimeout(function () {
    self.scheduleDrawer.style.display = "none";
    panel.style.animation = "";
    panel.style.opacity = "";
  }, 180);
};

// ===== Load tasks =====

FKTeamsChat.prototype.loadScheduleTasks = async function () {
  if (!this.scheduleList) return;
  this._scheduleSelected = null;
  this.scheduleList.innerHTML = '<div class="schedule-loading">loading...</div>';

  try {
    var status = this.scheduleFilter ? this.scheduleFilter.value : "";
    var url = status
      ? "/api/fkteams/schedules?status=" + encodeURIComponent(status)
      : "/api/fkteams/schedules";
    var response = await this.fetchWithAuth(url);
    if (!response.ok) { this.scheduleList.innerHTML = '<div class="schedule-empty">load failed</div>'; return; }
    var result = await response.json();
    if (result.code !== 0 || !result.data || !result.data.tasks) {
      this.scheduleList.innerHTML = '<div class="schedule-empty">no scheduled tasks</div>';
      return;
    }
    this._renderTaskList(result.data.tasks);
  } catch (e) {
    console.error("Error loading schedule tasks:", e);
    this.scheduleList.innerHTML = '<div class="schedule-empty">load failed</div>';
  }
};

// ===== Render task list =====

FKTeamsChat.prototype._renderTaskList = function (tasks) {
  if (!this.scheduleList) return;
  if (!tasks || tasks.length === 0) {
    this.scheduleList.innerHTML = '<div class="schedule-empty">no scheduled tasks</div>';
    return;
  }

  var statusOrder = { running: 0, pending: 1, failed: 2, completed: 3, cancelled: 4 };
  tasks.sort(function (a, b) {
    var o = (statusOrder[a.status] ?? 5) - (statusOrder[b.status] ?? 5);
    if (o !== 0) return o;
    return new Date(a.next_run_at) - new Date(b.next_run_at);
  });

  var self = this;
  var html = "";

  tasks.forEach(function (task) {
    var info = self.getScheduleStatusInfo(task.status);
    var desc = self.escapeHtml(task.task);
    if (desc.length > 100) desc = desc.substring(0, 100) + "...";

    var meta = "";
    if (task.cron_expr) {
      meta += '<span class="schedule-meta-tag schedule-meta-cron">' + self.escapeHtml(task.cron_expr) + '</span>';
    } else {
      meta += '<span class="schedule-meta-tag schedule-meta-once">one-time</span>';
    }
    meta += '<span class="schedule-meta-time">' + self.formatScheduleTime(task.next_run_at) + '</span>';

    html +=
      '<div class="schedule-item status-' + task.status + '" data-id="' + self.escapeHtml(task.id) + '">' +
        '<div class="schedule-item-header">' +
          '<span class="schedule-status-badge ' + info.cls + '"><span class="status-dot"></span>' + info.label + '</span>' +
          '<span class="schedule-item-id">' + self.escapeHtml(task.id.substring(0, 8)) + '</span>' +
        '</div>' +
        '<div class="schedule-item-task">' + desc + '</div>' +
        '<div class="schedule-item-meta">' + meta + '</div>' +
      '</div>';
  });

  this.scheduleList.innerHTML = html;

  // bind clicks
  this.scheduleList.querySelectorAll(".schedule-item").forEach(function (el) {
    el.addEventListener("click", function () {
      self._showTaskDetail(el.dataset.id);
    });
  });
};

// ===== Show task detail (replaces list) =====

FKTeamsChat.prototype._showTaskDetail = function (taskId) {
  if (!this.scheduleList) return;

  // find task data from the DOM
  var itemEl = this.scheduleList.querySelector('[data-id="' + this.escapeHtml(taskId) + '"]');
  var taskDesc = itemEl ? itemEl.querySelector(".schedule-item-task").textContent : "";
  var statusCls = "";
  if (itemEl) {
    for (var i = 0; i < itemEl.classList.length; i++) {
      if (itemEl.classList[i].indexOf("status-") === 0) { statusCls = itemEl.classList[i]; break; }
    }
  }

  this._scheduleSelected = taskId;
  this.scheduleList.innerHTML =
    '<div class="schedule-detail-view" id="schedule-detail-view">' +
      '<div class="schedule-detail-back" id="schedule-detail-back">' +
        '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="15 18 9 12 15 6"/></svg>' +
        '<span>back</span>' +
      '</div>' +
      '<div class="schedule-detail-task">' + this.escapeHtml(taskDesc) + '</div>' +
      '<div class="schedule-detail-body" id="schedule-detail-body">' +
        '<div class="schedule-detail-loading">loading result...</div>' +
      '</div>' +
    '</div>';

  var self = this;
  document.getElementById("schedule-detail-back").addEventListener("click", function () {
    self._backToScheduleList();
  });

  this._loadTaskDetail(taskId);
};

FKTeamsChat.prototype._backToScheduleList = function () {
  this._scheduleSelected = null;
  this.loadScheduleTasks();
};

// ===== Load detail =====

FKTeamsChat.prototype._loadTaskDetail = async function (taskId) {
  var body = document.getElementById("schedule-detail-body");
  if (!body) return;

  var self = this;
  var results = await Promise.all([
    this._fetchTaskResult(taskId),
    this._fetchTaskHistory(taskId)
  ]);
  var resultHtml = results[0];
  var historyData = results[1];

  var html = "";
  if (resultHtml) {
    html +=
      '<div class="schedule-result">' +
        '<div class="schedule-result-header">latest result</div>' +
        '<div class="schedule-result-content markdown-body">' + resultHtml + '</div>' +
      '</div>';
  }
  if (historyData) {
    html +=
      '<div class="schedule-history">' +
        '<div class="schedule-history-header">history (' + historyData.count + ')</div>' +
        '<div class="schedule-history-timeline">' + historyData.entries + '</div>' +
        '<div class="schedule-history-viewer" id="shv-' + self.escapeHtml(taskId) + '" style="display:none"></div>' +
      '</div>';
  }
  if (!html) {
    html = '<div class="schedule-detail-empty">no results yet</div>';
  }

  body.innerHTML = html;

  // bind history entries
  body.querySelectorAll(".schedule-history-entry").forEach(function (entry) {
    entry.addEventListener("click", function (e) {
      e.stopPropagation();
      self._loadHistoryContent(taskId, entry.dataset.filename);
    });
  });
};

FKTeamsChat.prototype._fetchTaskResult = async function (taskId) {
  try {
    var resp = await this.fetchWithAuth("/api/fkteams/schedules/" + taskId + "/result");
    if (!resp.ok) return "";
    var r = await resp.json();
    if (r.code !== 0 || !r.data || !r.data.result) return "";
    return this.renderMarkdown(r.data.result);
  } catch (e) { return ""; }
};

FKTeamsChat.prototype._fetchTaskHistory = async function (taskId) {
  try {
    var resp = await this.fetchWithAuth("/api/fkteams/schedules/" + taskId + "/history");
    if (!resp.ok) return "";
    var r = await resp.json();
    if (r.code !== 0 || !r.data || !r.data.history) return "";
    var entries = r.data.history;
    if (!entries || entries.length === 0) return "";

    var self = this;
    var items = entries.map(function (e) {
      return '<div class="schedule-history-entry" data-filename="' + self.escapeHtml(e.filename) + '">' +
        '<span class="schedule-history-entry-time">' + self.escapeHtml(e.time) + '</span>' +
      '</div>';
    }).join("");

    return { count: entries.length, entries: items };
  } catch (e) { return ""; }
};

// ===== History content viewer =====

FKTeamsChat.prototype._loadHistoryContent = async function (taskId, filename) {
  var viewer = document.getElementById("shv-" + this.escapeHtml(taskId));
  if (!viewer) return;

  if (viewer.dataset.currentFile === filename && viewer.style.display !== "none") {
    viewer.style.display = "none";
    viewer.dataset.currentFile = "";
    viewer.parentElement.querySelectorAll(".schedule-history-entry.active").forEach(function (el) { el.classList.remove("active"); });
    return;
  }

  viewer.style.display = "block";
  viewer.innerHTML = '<div class="schedule-detail-loading">loading...</div>';
  viewer.dataset.currentFile = filename;

  var timeline = viewer.parentElement.querySelector(".schedule-history-timeline");
  if (timeline) {
    timeline.querySelectorAll(".schedule-history-entry.active").forEach(function (el) { el.classList.remove("active"); });
    var active = timeline.querySelector('[data-filename="' + this.escapeHtml(filename) + '"]');
    if (active) active.classList.add("active");
  }

  var self = this;
  try {
    var resp = await this.fetchWithAuth("/api/fkteams/schedules/" + taskId + "/history/" + filename);
    if (!resp.ok) throw new Error("fetch failed");
    var r = await resp.json();
    if (r.code !== 0 || !r.data || !r.data.content) {
      viewer.innerHTML = '<div class="schedule-detail-error">no content</div>';
      return;
    }

    viewer.innerHTML =
      '<div class="schedule-history-viewer-header">' +
        '<span>' + self.escapeHtml(filename) + '</span>' +
        '<button class="schedule-history-viewer-close">&#10005;</button>' +
      '</div>' +
      '<div class="schedule-result-content markdown-body">' + self.renderMarkdown(r.data.content) + '</div>';

    viewer.querySelector(".schedule-history-viewer-close").addEventListener("click", function (e) {
      e.stopPropagation();
      viewer.style.display = "none";
      viewer.dataset.currentFile = "";
      if (timeline) timeline.querySelectorAll(".schedule-history-entry.active").forEach(function (el) { el.classList.remove("active"); });
    });
  } catch (e) {
    viewer.innerHTML = '<div class="schedule-detail-error">load failed</div>';
  }
};

// ===== Cancel =====

FKTeamsChat.prototype.cancelScheduleTask = async function (taskId) {
  try {
    var response = await this.fetchWithAuth("/api/fkteams/schedules/" + taskId + "/cancel", { method: "POST" });
    var result = await response.json();
    if (result.code === 0) {
      this.showNotification("task cancelled", "success");
      this.loadScheduleTasks();
    } else {
      this.showNotification(result.message || "cancel failed", "error");
    }
  } catch (e) {
    this.showNotification("cancel failed", "error");
  }
};

// ===== Helpers =====

FKTeamsChat.prototype.getScheduleStatusInfo = function (status) {
  var map = {
    pending:   { label: "pending",   cls: "status-pending" },
    running:   { label: "running",   cls: "status-running" },
    completed: { label: "done",      cls: "status-completed" },
    failed:    { label: "failed",    cls: "status-failed" },
    cancelled: { label: "cancelled", cls: "status-cancelled" }
  };
  return map[status] || { label: status, cls: "" };
};

FKTeamsChat.prototype.formatScheduleTime = function (timeStr) {
  if (!timeStr) return "-";
  var d = new Date(timeStr);
  if (isNaN(d.getTime())) return timeStr;
  var pad = function (n) { return n.toString().padStart(2, "0"); };
  return d.getFullYear() + "-" + pad(d.getMonth() + 1) + "-" + pad(d.getDate()) +
    " " + pad(d.getHours()) + ":" + pad(d.getMinutes());
};
