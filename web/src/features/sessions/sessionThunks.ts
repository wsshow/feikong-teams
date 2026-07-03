import { createAsyncThunk } from "@reduxjs/toolkit";
import { listSessions, getSession } from "@/api/sessions";
import { chatActions, sessionsActions } from "@/app/store";

export const loadSessions = createAsyncThunk("sessions/load", async (_, { dispatch }) => {
  dispatch(sessionsActions.setSessionsLoading(true));
  const result = await listSessions();
  dispatch(sessionsActions.setSessions(result.sessions || []));
});

export const loadSessionDetail = createAsyncThunk("sessions/detail", async (sessionID: string, { dispatch }) => {
  const detail = await getSession(sessionID);
  dispatch(chatActions.clearMessages());
  for (const event of detail.events || []) {
    dispatch(chatActions.receiveEvent(event));
  }
  dispatch(chatActions.setQueue(detail.queue || []));
  if (detail.active_task) {
    dispatch(chatActions.setRunningSession(sessionID));
  } else {
    dispatch(chatActions.setProcessing(false));
  }
  dispatch(chatActions.setActiveSession(sessionID));
});
