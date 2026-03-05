import { useState, useEffect, useRef, useCallback } from 'react';
import { api } from '../services/api';

interface UseDraftSessionResult {
  sessionId: string | null;
  loading: boolean;
  error: string | null;
}

/**
 * Manages a playground session for speculative decoding with a draft model.
 * Serializes create/delete operations and supports cancellation to prevent
 * race conditions when the user rapidly changes model or draft selection.
 */
export function useDraftSession(selectedModel: string, draftModel: string): UseDraftSessionResult {
  const [sessionId, setSessionId] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const sessionRef = useRef<string | null>(null);
  const genRef = useRef(0);

  const deleteCurrentSession = useCallback(async () => {
    const sid = sessionRef.current;
    if (sid) {
      sessionRef.current = null;
      setSessionId(null);
      try {
        await api.deletePlaygroundSession(sid);
      } catch {
        // Session may already be gone.
      }
    }
  }, []);

  useEffect(() => {
    const gen = ++genRef.current;
    let cancelled = false;

    (async () => {
      await deleteCurrentSession();

      if (!draftModel || !selectedModel) {
        setError(null);
        return;
      }

      setLoading(true);
      setError(null);
      try {
        const resp = await api.createPlaygroundSession({
          model_id: selectedModel,
          template_mode: 'builtin',
          config: {
            draft_model_id: draftModel,
            nseq_max: 1,
            incremental_cache: true,
          },
        });

        if (cancelled || genRef.current !== gen) {
          api.deletePlaygroundSession(resp.session_id).catch(() => {});
          return;
        }

        setSessionId(resp.session_id);
        sessionRef.current = resp.session_id;
      } catch (err: any) {
        if (genRef.current === gen) {
          setError(err?.message || 'Failed to create draft session');
        }
      } finally {
        if (genRef.current === gen) {
          setLoading(false);
        }
      }
    })();

    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [draftModel, selectedModel]);

  // Cleanup on unmount.
  useEffect(() => {
    return () => {
      const sid = sessionRef.current;
      if (sid) {
        api.deletePlaygroundSession(sid).catch(() => {});
      }
    };
  }, []);

  return { sessionId, loading, error };
}
