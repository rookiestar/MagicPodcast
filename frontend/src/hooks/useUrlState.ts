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
  const stateRef = useRef(state);

  // 跟踪是否是内部更新（避免 popstate 循环）
  const isInternalUpdate = useRef(false);

  useEffect(() => {
    stateRef.current = state;
  }, [state]);

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
    const newValue =
      typeof value === "function"
        ? (value as (prev: T) => T)(stateRef.current)
        : value;
    stateRef.current = newValue;
    setStateInternal(newValue);
    updateUrl(newValue);
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
        const nextValue =
          values.length > 0
            ? (values.map((v) => parseValue(v)) as T)
            : defaultValue;
        stateRef.current = nextValue;
        setStateInternal(nextValue);
      } else {
        const value = params.get(key);
        const nextValue = parseValue(value);
        stateRef.current = nextValue;
        setStateInternal(nextValue);
      }
    };

    window.addEventListener("popstate", handlePopState);
    return () => window.removeEventListener("popstate", handlePopState);
  }, [key, defaultValue, isArray, parseValue]);

  return [state, setState];
}
