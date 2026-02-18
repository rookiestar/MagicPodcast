import { useState, useEffect, useCallback, useRef } from "react";

/**
 * URL 状态同步 Hook
 *
 * 将组件状态与 URL 查询参数同步，支持：
 * - 初始化时从 URL 读取状态
 * - 状态变更时更新 URL
 * - 浏览器前进/后退时恢复状态
 *
 * @template T - 状态类型
 * @param key - URL 查询参数名
 * @param defaultValue - 默认值
 * @param options - 配置选项
 * @returns [当前值, 设置值函数]
 *
 * @example
 * const [sortBy, setSortBy] = useUrlState<string>("sort_by", "recent_update");
 * const [tagIds, setTagIds] = useUrlState<number[]>("tag_id", [], { isArray: true });
 */
interface UseUrlStateOptions<T> {
  /** 是否为数组类型（如多个同名参数） */
  isArray?: boolean;
  /** 值转换函数：从 URL 字符串转换为目标类型 */
  parse?: (value: string) => T;
  /** 值转换函数：从目标类型转换为 URL 字符串 */
  serialize?: (value: T) => string;
  /** 是否使用 replaceState（默认 true），false 则使用 pushState */
  replace?: boolean;
}

export function useUrlState<T>(
  key: string,
  defaultValue: T,
  options: UseUrlStateOptions<T> = {}
): [T, (value: T | ((prev: T) => T)) => void] {
  const {
    isArray = false,
    parse,
    serialize,
    replace = true,
  } = options;

  // 内部解析函数
  const parseValue = useCallback((str: string | null): T => {
    if (str === null) return defaultValue;

    if (parse) {
      return parse(str);
    }

    // 默认类型推断
    if (typeof defaultValue === "number") {
      return (parseInt(str, 10) || defaultValue) as T;
    }
    if (typeof defaultValue === "boolean") {
      return (str === "true") as T;
    }

    return str as T;
  }, [defaultValue, parse]);

  // 从 URL 读取初始值
  const getInitialValue = useCallback((): T => {
    if (typeof window === "undefined") return defaultValue;

    const params = new URLSearchParams(window.location.search);

    if (isArray) {
      const values = params.getAll(key);
      if (values.length === 0) return defaultValue;
      return values.map((v) => parseValue(v)) as T;
    }

    const value = params.get(key);
    return parseValue(value);
  }, [key, defaultValue, isArray, parseValue]);

  const [state, setStateInternal] = useState<T>(getInitialValue);

  // 跟踪是否是内部更新（避免 popstate 循环）
  const isInternalUpdate = useRef(false);

  // 更新 URL
  const updateUrl = useCallback((newValue: T) => {
    const url = new URL(window.location.href);

    // 移除现有参数
    url.searchParams.delete(key);

    // 添加新参数
    if (isArray && Array.isArray(newValue)) {
      if (newValue.length > 0) {
        newValue.forEach((item) => {
          const strValue = serialize ? serialize(item) : String(item);
          url.searchParams.append(key, strValue);
        });
      }
    } else if (newValue !== null && newValue !== undefined && newValue !== "") {
      const strValue = serialize ? serialize(newValue) : String(newValue);
      url.searchParams.set(key, strValue);
    }

    // 更新浏览器历史
    if (replace) {
      window.history.replaceState({}, "", url.toString());
    } else {
      window.history.pushState({}, "", url.toString());
    }
  }, [key, isArray, serialize, replace]);

  // 包装 setState，同步更新 URL
  const setState = useCallback((value: T | ((prev: T) => T)) => {
    isInternalUpdate.current = true;
    setStateInternal((prev) => {
      const newValue = typeof value === "function" ? (value as (prev: T) => T)(prev) : value;
      updateUrl(newValue);
      return newValue;
    });
    // 延迟重置，确保 popstate 不会立即触发
    setTimeout(() => {
      isInternalUpdate.current = false;
    }, 0);
  }, [updateUrl]);

  // 监听浏览器前进/后退
  useEffect(() => {
    const handlePopState = () => {
      if (isInternalUpdate.current) return;

      const params = new URLSearchParams(window.location.search);

      if (isArray) {
        const values = params.getAll(key);
        setStateInternal(values.length > 0 ? values.map((v) => parseValue(v)) as T : defaultValue);
      } else {
        const value = params.get(key);
        setStateInternal(parseValue(value));
      }
    };

    window.addEventListener("popstate", handlePopState);
    return () => window.removeEventListener("popstate", handlePopState);
  }, [key, defaultValue, isArray, parseValue]);

  return [state, setState];
}

/**
 * 多参数 URL 状态同步 Hook
 *
 * 一次性管理多个 URL 参数，适合需要协调多个状态的场景
 *
 * @template T - 状态对象类型
 * @param schema - 状态模式定义（包含默认值）
 * @returns { state, setState, resetState }
 *
 * @example
 * const { state, setState } = useUrlStates({
 *   sort_by: "recent_update",
 *   tag_id: [] as number[],
 * });
 */
interface UrlStatesResult<T extends Record<string, unknown>> {
  state: T;
  setState: (updates: Partial<T>) => void;
  resetState: () => void;
}

export function useUrlStates<T extends Record<string, unknown>>(
  schema: T
): UrlStatesResult<T> {
  type Key = keyof T;

  // 从 URL 读取所有初始值
  const getInitialState = useCallback((): T => {
    if (typeof window === "undefined") return schema;

    const params = new URLSearchParams(window.location.search);
    const result = { ...schema };

    Object.keys(schema).forEach((key) => {
      const k = key as Key;
      const defaultValue = schema[k];

      if (Array.isArray(defaultValue)) {
        const values = params.getAll(key);
        if (values.length > 0) {
          // 尝试解析为数字数组
          if (typeof defaultValue[0] === "number") {
            (result as Record<string, unknown>)[key] = values.map((v) => parseInt(v, 10));
          } else {
            (result as Record<string, unknown>)[key] = values;
          }
        }
      } else {
        const value = params.get(key);
        if (value !== null) {
          if (typeof defaultValue === "number") {
            (result as Record<string, unknown>)[key] = parseInt(value, 10) || defaultValue;
          } else if (typeof defaultValue === "boolean") {
            (result as Record<string, unknown>)[key] = value === "true";
          } else {
            (result as Record<string, unknown>)[key] = value;
          }
        }
      }
    });

    return result;
  }, [schema]);

  const [state, setStateInternal] = useState<T>(getInitialState);
  const isInternalUpdate = useRef(false);

  // 更新 URL
  const updateUrl = useCallback((newState: T) => {
    const url = new URL(window.location.href);

    // 先移除所有管理的参数
    Object.keys(schema).forEach((key) => {
      url.searchParams.delete(key);
    });

    // 添加新参数
    Object.entries(newState).forEach(([key, value]) => {
      if (Array.isArray(value)) {
        if (value.length > 0) {
          value.forEach((item) => {
            url.searchParams.append(key, String(item));
          });
        }
      } else if (value !== null && value !== undefined && value !== "") {
        url.searchParams.set(key, String(value));
      }
    });

    window.history.replaceState({}, "", url.toString());
  }, [schema]);

  const setState = useCallback((updates: Partial<T>) => {
    isInternalUpdate.current = true;
    setStateInternal((prev) => {
      const newState = { ...prev, ...updates };
      updateUrl(newState);
      return newState;
    });
    setTimeout(() => {
      isInternalUpdate.current = false;
    }, 0);
  }, [updateUrl]);

  const resetState = useCallback(() => {
    isInternalUpdate.current = true;
    setStateInternal(schema);
    updateUrl(schema);
    setTimeout(() => {
      isInternalUpdate.current = false;
    }, 0);
  }, [schema, updateUrl]);

  // 监听浏览器前进/后退
  useEffect(() => {
    const handlePopState = () => {
      if (isInternalUpdate.current) return;
      setStateInternal(getInitialState());
    };

    window.addEventListener("popstate", handlePopState);
    return () => window.removeEventListener("popstate", handlePopState);
  }, [getInitialState]);

  return { state, setState, resetState };
}

export default useUrlState;
