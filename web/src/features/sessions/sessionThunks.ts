import { createAsyncThunk } from "@reduxjs/toolkit";
import { listSessions, getSession, openAssistant, getAssistant } from "@/api/sessions";
import { assistantSessionID } from "@/lib/assistant";
import { chatActions, sessionsActions } from "@/app/store";

let sessionsRequest: ReturnType<typeof listSessions> | undefined;
const sessionDetailRequests = new Map<string, ReturnType<typeof getSession>>();
let assistantRequestVersion = 0;

export const refreshAssistant = createAsyncThunk("sessions/assistant", async (_, { dispatch }) => {
  const version = ++assistantRequestVersion;
  const assistant = await getAssistant();
  if (version === assistantRequestVersion) dispatch(sessionsActions.upsertSession(assistant));
});

export const loadSessions = createAsyncThunk(
  "sessions/load",
  async (_, { dispatch }) => {
    const requestStartedAt = Date.now();
    dispatch(sessionsActions.beginSessionsRequest(requestStartedAt));
    sessionsRequest = listSessions();
    try {
      const result = await sessionsRequest;
      dispatch(sessionsActions.setSessions({ items: result.sessions || [], requestStartedAt }));
      dispatch(chatActions.syncRunningSessions({
        sessionIDs: (result.sessions || []).filter((session) => session.active_task).map((session) => session.session_id),
        requestStartedAt,
      }));
    } catch (error) {
      dispatch(sessionsActions.setSessionsLoading(false));
      throw error;
    } finally {
      sessionsRequest = undefined;
    }
  },
  { condition: () => sessionsRequest === undefined },
);

export const loadSessionDetail = createAsyncThunk("sessions/detail", async (sessionID: string, { dispatch }) => {
  const pending = sessionDetailRequests.get(sessionID);
  if (pending) return pending;

  const request = (async () => {
    if (sessionID === assistantSessionID) {
      const assistant = await openAssistant();
      dispatch(sessionsActions.upsertSession(assistant));
    }
    return getSession(sessionID);
  })();
  sessionDetailRequests.set(sessionID, request);
  void request.then(
    () => clearSessionDetailRequest(sessionID, request),
    () => clearSessionDetailRequest(sessionID, request),
  );
  return request;
});

function clearSessionDetailRequest(sessionID: string, request: ReturnType<typeof getSession>) {
  if (sessionDetailRequests.get(sessionID) === request) sessionDetailRequests.delete(sessionID);
}
