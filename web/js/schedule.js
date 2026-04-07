/**
 * schedule.js - 定时任务管理
 */

// ===== 初始化 =====

FKTeamsChat.prototype.initSchedule = function () {
  this.scheduleModal = document.getElementById("schedule-modal");
  this.scheduleModalClose = document.getElementById("schedule-modal-close");
  this.scheduleList = document.getElementById("schedule-list");
  this.scheduleFilter = document.getElementById("schedule-filter");
  this.scheduleRefreshBtn = document.getElementById("schedule-refresh-btn");
  this.scheduleManageBtn = document.getElementById("schedule-manage-btn");

  if (this.scheduleManageBtn) {
    this.scheduleManageBtn.addEventListener("click", () =>
      this.openScheduleModal(),
    );
  }
  if (this.scheduleModalClose) {
    this.scheduleModalClose.addEventListener("click", () =>
      this.closeScheduleModal(),
    );
  }

  if (this.scheduleFilter) {
    this.scheduleFilter.addEventListener("change", () =>
      this.loadScheduleTasks(),
    );
  }
  if (this.scheduleRefreshBtn) {
    this.scheduleRefreshBtn.addEventListener("click", () =>
      this.loadScheduleTasks(),
    );
  }
};

// ===== 弹窗控制 =====

FKTeamsChat.prototype.openScheduleModal = function () {
  if (!this.scheduleModal) return;
  this.scheduleModal.style.display = "flex";
  this.loadScheduleTasks();
};

FKTeamsChat.prototype.closeScheduleModal = function () {
  if (!this.scheduleModal) return;
  this.scheduleModal.style.display = "none";
};

// ===== 加载任务列表 =====

FKTeamsChat.prototype.loadScheduleTasks = async function () {
  if (!this.scheduleList) return;
  this.scheduleList.innerHTML = '<div class="schedule-loading">加载中...</div>';

  try {
    const status = this.scheduleFilter ? this.scheduleFilter.value : "";
    const url = status
      ? `/api/fkteams/schedules?status=${status}`
      : "/api/fkteams/schedules";
    const response = await this.fetchWithAuth(url);
    if (!response.ok) {
      this.scheduleList.innerHTML =
        '<div class="schedule-empty">加载失败</div>';
      return;
    }

    const result = await response.json();
    if (result.code !== 0 || !result.data || !result.data.tasks) {
      this.scheduleList.innerHTML =
        '<div class="schedule-empty">暂无定时任务</div>';
      return;
    }

    this.renderScheduleTasks(result.data.tasks);
  } catch (error) {
    console.error("Error loading schedule tasks:", error);
    this.scheduleList.innerHTML = '<div class="schedule-empty">加载失败</div>';
  }
};

// ===== 渲染任务列表 =====

FKTeamsChat.prototype.renderScheduleTasks = function (tasks) {
  if (!this.scheduleList) return;

  if (!tasks || tasks.length === 0) {
    this.scheduleList.innerHTML =
      '<div class="schedule-empty">暂无定时任务</div>';
    return;
  }

  // 排序：pending/running 在前，按下次执行时间排序
  const statusOrder = {
    running: 0,
    pending: 1,
    failed: 2,
    completed: 3,
    cancelled: 4,
  };
  tasks.sort((a, b) => {
    const orderDiff =
      (statusOrder[a.status] ?? 5) - (statusOrder[b.status] ?? 5);
    if (orderDiff !== 0) return orderDiff;
    return new Date(a.next_run_at) - new Date(b.next_run_at);
  });

  this.scheduleList.innerHTML = "";

  tasks.forEach((task) => {
    const item = document.createElement("div");
    item.className = `schedule-item schedule-status-${task.status}`;

    const statusInfo = this.getScheduleStatusInfo(task.status);
    const taskDesc = this.escapeHtml(task.task);
    const truncatedDesc =
      taskDesc.length > 80 ? taskDesc.substring(0, 80) + "..." : taskDesc;

    let metaHtml = "";
    if (task.cron_expr) {
      metaHtml += `<span class="schedule-meta-tag schedule-meta-cron">cron: ${this.escapeHtml(task.cron_expr)}</span>`;
    } else {
      metaHtml += `<span class="schedule-meta-tag schedule-meta-once">一次性</span>`;
    }
    metaHtml += `<span class="schedule-meta-time">下次: ${this.formatScheduleTime(task.next_run_at)}</span>`;
    if (task.last_run_at) {
      metaHtml += `<span class="schedule-meta-time">上次: ${this.formatScheduleTime(task.last_run_at)}</span>`;
    }

    let actionsHtml = "";
    if (task.status === "pending") {
      actionsHtml = `
                <button class="schedule-action-btn cancel-schedule-btn" data-id="${this.escapeHtml(task.id)}" title="取消任务">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <circle cx="12" cy="12" r="10" />
                        <line x1="15" y1="9" x2="9" y2="15" />
                        <line x1="9" y1="9" x2="15" y2="15" />
                    </svg>
                </button>`;
    }

    item.innerHTML = `
            <div class="schedule-item-header">
                <span class="schedule-status-badge ${statusInfo.cls}">${statusInfo.label}</span>
                <span class="schedule-item-id">${this.escapeHtml(task.id)}</span>
            </div>
            <div class="schedule-item-task" title="${taskDesc}">${truncatedDesc}</div>
            <div class="schedule-item-meta">${metaHtml}</div>
            <div class="schedule-item-actions">${actionsHtml}</div>
        `;

    // 绑定取消按钮
    const cancelBtn = item.querySelector(".cancel-schedule-btn");
    if (cancelBtn) {
      cancelBtn.addEventListener("click", () =>
        this.cancelScheduleTask(task.id),
      );
    }

    this.scheduleList.appendChild(item);
  });
};

// ===== 取消任务 =====

FKTeamsChat.prototype.cancelScheduleTask = async function (taskId) {
  try {
    const response = await this.fetchWithAuth(
      `/api/fkteams/schedules/${taskId}/cancel`,
      { method: "POST" },
    );
    const result = await response.json();
    if (result.code === 0) {
      this.showNotification("任务已取消", "success");
      this.loadScheduleTasks();
    } else {
      this.showNotification(result.message || "取消失败", "error");
    }
  } catch (error) {
    console.error("Error cancelling task:", error);
    this.showNotification("取消任务失败", "error");
  }
};

// ===== 辅助函数 =====

FKTeamsChat.prototype.getScheduleStatusInfo = function (status) {
  const map = {
    pending: { label: "等待中", cls: "status-pending" },
    running: { label: "执行中", cls: "status-running" },
    completed: { label: "已完成", cls: "status-completed" },
    failed: { label: "已失败", cls: "status-failed" },
    cancelled: { label: "已取消", cls: "status-cancelled" },
  };
  return map[status] || { label: status, cls: "" };
};

FKTeamsChat.prototype.formatScheduleTime = function (timeStr) {
  if (!timeStr) return "-";
  const d = new Date(timeStr);
  if (isNaN(d.getTime())) return timeStr;
  const pad = (n) => n.toString().padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
};
