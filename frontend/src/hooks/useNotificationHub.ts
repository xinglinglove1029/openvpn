import { useCallback, useEffect, useState } from 'react';
import { useAuth } from '@/store/auth';
import {
  realtimeHub,
  type ConnectionState,
  type IncomingNotification,
  type UnreadSnapshot,
} from '@/lib/notificationHub';

const EMPTY_SNAPSHOT: UnreadSnapshot = { unread: 0, lastReadId: 0, maxId: 0 };

export function useNotificationHub() {
  const { user } = useAuth();
  const [state, setState] = useState<ConnectionState>(realtimeHub.getState());
  const [snapshot, setSnapshot] = useState<UnreadSnapshot>(EMPTY_SNAPSHOT);
  const [latest, setLatest] = useState<IncomingNotification | null>(null);

  useEffect(() => {
    if (!user?.username) {
      realtimeHub.disconnect();
      setSnapshot(EMPTY_SNAPSHOT);
      setLatest(null);
      return;
    }

    const offState = realtimeHub.onState(setState);
    const offUnread = realtimeHub.onUnread((next) => setSnapshot(next));
    const offNotify = realtimeHub.onNotification((item) => setLatest(item));

    realtimeHub.connect();
    void realtimeHub.refreshUnread();

    return () => {
      offState();
      offUnread();
      offNotify();
    };
  }, [user?.username]);

  const refresh = useCallback(() => realtimeHub.refreshUnread(), []);
  const markRead = useCallback((lastReadId?: number) => realtimeHub.markRead(lastReadId), []);

  return {
    state,
    snapshot,
    latest,
    refresh,
    markRead,
    subscribe: realtimeHub.subscribe.bind(realtimeHub),
  };
}
