import { useEffect, useState } from 'react';

export type AsyncState<T> = { loading: boolean; error?: string; data?: T };

export function useAsync<T>(loader: () => Promise<T>, deps: unknown[] = []): AsyncState<T> {
  const [state, setState] = useState<AsyncState<T>>({ loading: true });

  useEffect(() => {
    let mounted = true;
    setState({ loading: true });
    loader()
      .then((data) => mounted && setState({ loading: false, data }))
      .catch((error) => mounted && setState({ loading: false, error: error instanceof Error ? error.message : String(error) }));
    return () => { mounted = false; };
  }, deps);

  return state;
}
