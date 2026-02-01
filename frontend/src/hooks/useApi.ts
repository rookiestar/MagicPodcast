import { useState, useCallback, useEffect, useRef } from "react";

interface ApiState<T> {
  data: T | null;
  error: Error | null;
  loading: boolean;
}

interface ApiOptions {
  retry?: number;
  retryDelay?: number;
  onSuccess?: (data: any) => void;
  onError?: (error: Error) => void;
}

/**
 * useApi Hook - 通用API调用状态管理
 *
 * @param apiFn - API调用函数
 * @param options - 配置选项
 * @returns { data, error, loading, execute, reset }
 *
 * @example
 * const { data, error, loading, execute } = useApi(() => fetchPodcasts(1))
 * await execute()
 */
export function useApi<T>(
  apiFn: () => Promise<T>,
  options: ApiOptions = {},
): ApiState<T> & {
  execute: () => Promise<T | null>;
  reset: () => void;
  isMounted: React.MutableRefObject<boolean>;
} {
  const { retry = 0, retryDelay = 1000, onSuccess, onError } = options;

  const [state, setState] = useState<ApiState<T>>({
    data: null,
    error: null,
    loading: false,
  });

  const isMounted = useRef(true);
  const retryCount = useRef(0);

  // 清理函数
  useEffect(() => {
    return () => {
      isMounted.current = false;
    };
  }, []);

  // 执行API调用
  const execute = useCallback(async (): Promise<T | null> => {
    setState((prev) => ({ ...prev, loading: true, error: null }));

    try {
      const data = await apiFn();

      if (isMounted.current) {
        setState({
          data,
          error: null,
          loading: false,
        });

        retryCount.current = 0;
        onSuccess?.(data);
      }

      return data;
    } catch (error) {
      const err = error instanceof Error ? error : new Error(String(error));

      // 重试逻辑
      if (retryCount.current < retry) {
        retryCount.current++;
        await new Promise((resolve) => setTimeout(resolve, retryDelay));
        return execute();
      }

      if (isMounted.current) {
        setState({
          data: null,
          error: err,
          loading: false,
        });

        retryCount.current = 0;
        onError?.(err);
      }

      return null;
    }
  }, [apiFn, retry, retryDelay, onSuccess, onError]);

  // 重置状态
  const reset = useCallback(() => {
    setState({
      data: null,
      error: null,
      loading: false,
    });
    retryCount.current = 0;
  }, []);

  return {
    ...state,
    execute,
    reset,
    isMounted,
  };
}

/**
 * useApiLazy Hook - 懒加载版本的useApi，不会自动执行
 *
 * @param apiFn - API调用函数
 * @param options - 配置选项
 * @returns { data, error, loading, execute, reset }
 *
 * @example
 * const { data, error, loading, execute } = useApiLazy(() => fetchPodcasts(page))
 * // 手动调用 execute()
 */
export function useApiLazy<T>(
  apiFn: () => Promise<T>,
  options: ApiOptions = {},
): ApiState<T> & {
  execute: () => Promise<T | null>;
  reset: () => void;
} {
  const { execute, ...state } = useApi(apiFn, options);

  return {
    ...state,
    // 覆盖execute，不自动执行
    execute,
  };
}

/**
 * useApiAuto Hook - 自动执行版本的useApi，组件挂载时自动执行
 *
 * @param apiFn - API调用函数
 * @param deps - 依赖数组，变化时重新执行
 * @param options - 配置选项
 * @returns { data, error, loading, execute, reset }
 *
 * @example
 * const { data, error, loading, execute } = useApiAuto(
 *   () => fetchPodcasts(page),
 *   [page]
 * )
 */
export function useApiAuto<T>(
  apiFn: () => Promise<T>,
  deps: any[] = [],
  options: ApiOptions = {},
): ApiState<T> & {
  execute: () => Promise<T | null>;
  reset: () => void;
} {
  const { data, error, loading, execute, reset, isMounted } = useApi(
    apiFn,
    options,
  );

  useEffect(() => {
    execute();

    return () => {
      // 组件卸载时取消正在进行的请求（如果支持）
      isMounted.current = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);

  return {
    data,
    error,
    loading,
    execute,
    reset,
  };
}

/**
 * useApiMutation Hook - 用于mutation操作（POST、PUT、DELETE）
 *
 * @param apiFn - API调用函数
 * @param options - 配置选项
 * @returns { data, error, loading, execute, reset }
 *
 * @example
 * const { data, error, loading, execute } = useApiMutation(
 *   (data) => createWorkflow(data)
 * )
 * await execute(workflowData)
 */
export function useApiMutation<T, P = any>(
  apiFn: (params: P) => Promise<T>,
  options: ApiOptions = {},
): ApiState<T> & {
  execute: (params: P) => Promise<T | null>;
  reset: () => void;
} {
  const [state, setState] = useState<ApiState<T>>({
    data: null,
    error: null,
    loading: false,
  });

  const execute = useCallback(
    async (params: P): Promise<T | null> => {
      setState((prev) => ({ ...prev, loading: true, error: null }));

      try {
        const data = await apiFn(params);

        setState({
          data,
          error: null,
          loading: false,
        });

        onSuccess?.(data);
        return data;
      } catch (error) {
        const err = error instanceof Error ? error : new Error(String(error));

        setState({
          data: null,
          error: err,
          loading: false,
        });

        onError?.(err);
        return null;
      }
    },
    [apiFn, onSuccess, onError],
  );

  const reset = useCallback(() => {
    setState({
      data: null,
      error: null,
      loading: false,
    });
  }, []);

  return {
    ...state,
    execute,
    reset,
  };
}
