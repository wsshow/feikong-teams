export function chatMessageElementID(messageID: string) {
  return `chat-message-${messageID.replace(/[^a-zA-Z0-9_-]/g, "_")}`;
}
