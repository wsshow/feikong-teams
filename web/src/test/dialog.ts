import type { JSDOM } from "jsdom";

// JSDOM 尚未实现原生 dialog 的打开与关闭，交互与焦点另由浏览器验证。
export function installDialogStub(dom: JSDOM) {
  Object.assign(dom.window.HTMLDialogElement.prototype, {
    showModal(this: HTMLDialogElement) {
      this.setAttribute("open", "");
    },
    close(this: HTMLDialogElement) {
      this.removeAttribute("open");
    },
  });
}
