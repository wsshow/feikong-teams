import { useCallback, useEffect, useState } from "react";

const openDisclosureIDs = new Set<string>();

export function useDisclosureState(id: string) {
  const [open, setOpen] = useState(() => openDisclosureIDs.has(id));

  useEffect(() => {
    setOpen(openDisclosureIDs.has(id));
  }, [id]);

  const toggleOpen = useCallback(() => {
    setOpen((current) => {
      const next = !current;
      if (next) openDisclosureIDs.add(id);
      else openDisclosureIDs.delete(id);
      return next;
    });
  }, [id]);

  return [open, toggleOpen] as const;
}
