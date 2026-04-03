// select.js - 自定义下拉选择组件（匹配手绘素描主题）

/**
 * 将 <select class="config-select"> 转换为自定义下拉组件
 * 支持: value 读写、change 事件、disabled 状态
 * 使用 body portal 渲染下拉面板，避免被 overflow 裁剪
 */
function createCustomSelect(selectEl) {
    if (selectEl._customSelect) return selectEl._customSelect;

    const wrapper = document.createElement("div");
    wrapper.className = "fk-select";

    const trigger = document.createElement("div");
    trigger.className = "fk-select-trigger";
    trigger.setAttribute("tabindex", "0");

    const triggerText = document.createElement("span");
    triggerText.className = "fk-select-text";

    const arrowContainer = document.createElement("span");
    arrowContainer.className = "fk-select-arrow";
    arrowContainer.innerHTML = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="6 9 12 15 18 9" /></svg>';

    trigger.appendChild(triggerText);
    trigger.appendChild(arrowContainer);

    // 下拉面板挂载到 body 上避免 overflow 裁剪
    const dropdown = document.createElement("div");
    dropdown.className = "fk-select-dropdown";
    document.body.appendChild(dropdown);

    wrapper.appendChild(trigger);

    // 插入 DOM
    selectEl.parentNode.insertBefore(wrapper, selectEl);
    selectEl.style.display = "none";
    wrapper.appendChild(selectEl);

    function buildOptions() {
        dropdown.innerHTML = "";
        Array.from(selectEl.options).forEach((opt) => {
            const item = document.createElement("div");
            item.className = "fk-select-option" + (opt.selected ? " active" : "");
            item.dataset.value = opt.value;
            item.textContent = opt.textContent;
            if (opt.disabled) item.classList.add("disabled");
            dropdown.appendChild(item);
        });
        updateTriggerText();
    }

    function updateTriggerText() {
        const selected = selectEl.options[selectEl.selectedIndex];
        triggerText.textContent = selected ? selected.textContent : "";
    }

    function positionDropdown() {
        const rect = trigger.getBoundingClientRect();
        dropdown.style.position = "fixed";
        dropdown.style.left = rect.left + "px";
        dropdown.style.width = rect.width + "px";

        // 判断上下空间
        const spaceBelow = window.innerHeight - rect.bottom;
        const spaceAbove = rect.top;
        const dropdownHeight = Math.min(dropdown.scrollHeight, 200);

        if (spaceBelow >= dropdownHeight + 4 || spaceBelow >= spaceAbove) {
            dropdown.style.top = (rect.bottom + 4) + "px";
            dropdown.style.bottom = "auto";
            dropdown.classList.remove("fk-select-dropdown-above");
        } else {
            dropdown.style.top = "auto";
            dropdown.style.bottom = (window.innerHeight - rect.top + 4) + "px";
            dropdown.classList.add("fk-select-dropdown-above");
        }
    }

    function open() {
        if (wrapper.classList.contains("disabled")) return;
        // 关闭其他
        document.querySelectorAll(".fk-select.open").forEach((s) => {
            if (s !== wrapper) {
                s.classList.remove("open");
                s._dropdown && (s._dropdown.style.display = "none");
            }
        });
        const isOpen = wrapper.classList.toggle("open");
        if (isOpen) {
            dropdown.style.display = "block";
            positionDropdown();
            // 滚动到选中项
            const active = dropdown.querySelector(".active");
            if (active) active.scrollIntoView({ block: "nearest" });
        } else {
            dropdown.style.display = "none";
        }
    }

    function close() {
        wrapper.classList.remove("open");
        dropdown.style.display = "none";
    }

    function selectValue(val) {
        selectEl.value = val;
        selectEl.dispatchEvent(new Event("change", { bubbles: true }));
        dropdown.querySelectorAll(".fk-select-option").forEach((opt) => {
            opt.classList.toggle("active", opt.dataset.value === val);
        });
        updateTriggerText();
        close();
    }

    // 存储引用供外部关闭
    wrapper._dropdown = dropdown;

    // 事件
    trigger.addEventListener("click", (e) => {
        e.stopPropagation();
        open();
    });

    trigger.addEventListener("keydown", (e) => {
        if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            open();
        } else if (e.key === "Escape") {
            close();
        }
    });

    dropdown.addEventListener("click", (e) => {
        const opt = e.target.closest(".fk-select-option");
        if (opt && !opt.classList.contains("disabled")) {
            selectValue(opt.dataset.value);
        }
    });

    // 全局关闭
    document.addEventListener("click", (e) => {
        if (!wrapper.contains(e.target) && !dropdown.contains(e.target)) close();
    });

    // 滚动时重新定位
    window.addEventListener("scroll", () => {
        if (wrapper.classList.contains("open")) positionDropdown();
    }, true);

    buildOptions();
    dropdown.style.display = "none";

    // 暴露 API
    const api = { rebuild: buildOptions, close };
    selectEl._customSelect = api;

    // 监听 select 值外部变化
    const origDescriptor = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value");
    Object.defineProperty(selectEl, "value", {
        get() {
            return origDescriptor.get.call(this);
        },
        set(val) {
            origDescriptor.set.call(this, val);
            updateTriggerText();
            dropdown.querySelectorAll(".fk-select-option").forEach((opt) => {
                opt.classList.toggle("active", opt.dataset.value === val);
            });
        },
        configurable: true,
    });

    return api;
}

/**
 * 初始化页面上所有 .config-select
 */
function initAllCustomSelects(root) {
    (root || document).querySelectorAll("select.config-select").forEach((sel) => {
        createCustomSelect(sel);
    });
}
