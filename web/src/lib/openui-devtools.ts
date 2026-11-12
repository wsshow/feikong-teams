// react-lang 在开发环境自动导入调试浮窗，后者还会从 CDN 加载脚本。
// Vite 将该模块映射到本地空实现，避免聊天界面出现调试和部署推广浮窗。
export function OpenUIDevtools() {
  return null;
}
