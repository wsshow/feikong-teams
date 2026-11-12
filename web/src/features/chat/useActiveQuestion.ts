import { useEffect, useState, type RefObject } from "react";
import { chatBottomThreshold, chatMessageElementID } from "./dom";

export function useActiveQuestion(scrollRef: RefObject<HTMLDivElement | null>, questions: readonly { id: string }[]) {
  const [activeQuestionID, setActiveQuestionID] = useState("");

  useEffect(() => {
    const scroll = scrollRef.current;
    if (!scroll) {
      setActiveQuestionID("");
      return;
    }
    const anchors = questions.flatMap(({ id }) => {
      const element = document.getElementById(chatMessageElementID(id));
      return element && scroll.contains(element) ? [{ id, element }] : [];
    });
    let frame: number | undefined;

    const update = () => {
      frame = undefined;
      let activeID = anchors[0]?.id || "";
      const distanceFromBottom = scroll.scrollHeight - scroll.clientHeight - scroll.scrollTop;
      if (scroll.scrollTop > 0 && distanceFromBottom < chatBottomThreshold) {
        activeID = anchors[anchors.length - 1]?.id || "";
      } else if (scroll.scrollTop > 0) {
        // 以阅读区域中线所在的问题区间为准，长回答也继续归属于前一个问题。
        const readingPosition = scroll.getBoundingClientRect().top + scroll.clientHeight / 2;
        for (const { id, element } of anchors) {
          if (element.getBoundingClientRect().top > readingPosition) break;
          activeID = id;
        }
      }
      setActiveQuestionID(activeID);
    };
    const scheduleUpdate = () => {
      if (frame === undefined) frame = window.requestAnimationFrame(update);
    };
    const observer = new ResizeObserver(scheduleUpdate);
    observer.observe(scroll);
    if (scroll.firstElementChild) observer.observe(scroll.firstElementChild);
    scroll.addEventListener("scroll", scheduleUpdate, { passive: true });
    scheduleUpdate();

    return () => {
      scroll.removeEventListener("scroll", scheduleUpdate);
      observer.disconnect();
      if (frame !== undefined) window.cancelAnimationFrame(frame);
    };
  }, [questions, scrollRef]);

  return activeQuestionID;
}
