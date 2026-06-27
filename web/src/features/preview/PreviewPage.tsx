import { useEffect, useState } from "react";
import { getPreviewInfo } from "@/api/preview";
import { Button } from "@/components/ui/button";
import { Panel, PanelBody, PanelHeader } from "@/components/ui/panel";

export function PreviewPage() {
  const linkID = decodeURIComponent(location.pathname.split("/").filter(Boolean).pop() || "");
  const encodedLinkID = encodeURIComponent(linkID);
  const [path, setPath] = useState("");

  useEffect(() => {
    if (linkID) getPreviewInfo(linkID).then((info) => setPath(info.path || info.filename || ""));
  }, [linkID]);

  return (
    <div className="h-screen bg-muted/30 p-4">
      <Panel className="mx-auto flex h-full max-w-6xl flex-col">
        <PanelHeader className="flex items-center justify-between">
          <div>
            <div className="font-semibold">文件预览</div>
            <div className="text-sm text-muted-foreground">{path || linkID}</div>
          </div>
          <Button onClick={() => window.open(`/api/fkteams/preview/${encodedLinkID}?download=1`, "_blank")}>下载</Button>
        </PanelHeader>
        <PanelBody className="min-h-0 flex-1 p-0">
          <iframe className="h-full w-full border-0" src={`/api/fkteams/preview/${encodedLinkID}/render/`} title="文件预览" />
        </PanelBody>
      </Panel>
    </div>
  );
}
