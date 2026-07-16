import { createAsyncThunk } from "@reduxjs/toolkit";
import { listSessions, getSession } from "@/api/sessions";
import { sessionsActions } from "@/app/store";

let sessionsRequest: ReturnType<typeof listSessions> | undefined;
const sessionDetailRequests = new Map<string, ReturnType<typeof getSession>>();

export const loadSessions = createAsyncThunk(
  "sessions/load",
  async (_, { dispatch }) => {
    dispatch(sessionsActions.setSessionsLoading(true));
    sessionsRequest = listSessions();
    try {
      const result = await sessionsRequest;
      dispatch(sessionsActions.setSessions(result.sessions || []));
    } catch (error) {
      dispatch(sessionsActions.setSessionsLoading(false));
      throw error;
    } finally {
      sessionsRequest = undefined;
    }
  },
  { condition: () => sessionsRequest === undefined },
);

export const loadSessionDetail = createAsyncThunk("sessions/detail", async (sessionID: string) => {
  const pending = sessionDetailRequests.get(sessionID);
  if (pending) return pending;

  const request = getSession(sessionID);
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
