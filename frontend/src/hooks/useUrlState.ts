import { useMemo, useCallback, useRef } from "react";
import { singleParam, updateQuery, useLocationHref } from "@/lib/navigation";

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
  initialHref?: string;
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
  options: UseUrlStateOptions<T> = {},
): [T, (value: T | ((prev: T) => T)) => void] {
  const href = useLocationHref() || options.initialHref || "";
  const config = useRef({ defaultValue, options });
  config.current = { defaultValue, options };
  const read = useCallback((params: URLSearchParams): T => {
    const { defaultValue: fallback, options: settings } = config.current;
    const parse = (value: string): unknown => {
      if (settings.parse) return settings.parse(value);
      if (typeof fallback === "number") return Number(value) || fallback;
      if (typeof fallback === "boolean") return value === "true";
      return value;
    };
    if (settings.isArray) {
      const values = params.getAll(key);
      return values.length ? values.map(parse) as T : fallback;
    }
    const value = singleParam(params, key);
    return value === null ? fallback : parse(value) as T;
  }, [key]);
  const state = useMemo(() => read(new URL(href || "/", "http://navigation.local").searchParams), [href, read]);
  const setState = useCallback((value: T | ((previous: T) => T)) => {
    const { options: settings } = config.current;
    const previous = read(new URLSearchParams(window.location.search));
    const next = typeof value === "function" ? (value as (previous: T) => T)(previous) : value;
    const serialize = (item: T) => settings.serialize ? settings.serialize(item) : String(item);
    const encoded = settings.isArray && Array.isArray(next)
      ? next.map(serialize)
      : next === null || next === undefined || next === "" ? null : serialize(next);
    updateQuery({ [key]: encoded }, settings.replace ?? true);
  }, [key, read]);
  return [state, setState];
}
