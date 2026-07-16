import { get } from "./client";

export interface PreviewInfo {
  link_id?: string;
  path?: string;
  filename?: string;
  mime_type?: string;
}

export function getPreviewInfo(linkID: string) {
  return get<PreviewInfo>(`/api/fkteams/preview/${encodeURIComponent(linkID)}/info`, {
    authFailure: "ignore",
  });
}
