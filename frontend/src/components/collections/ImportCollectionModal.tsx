"use client";

import { useEffect, useRef, useState } from "react";
import { IconX } from "@tabler/icons-react";
import {
  collectionErrorMessage,
  collectionErrorCode,
  confirmCollectionImport,
  fetchCollectionDetail,
  previewCollection,
} from "@/lib/collections";
import type { CollectionPreview } from "@/types/collection";

interface ImportCollectionModalProps {
  isOpen: boolean;
  onClose: () => void;
  /** 新清单导入成功或用户主动打开已有清单时回调。 */
  onImported: (result: { duplicate: boolean; collectionID: number }) => void;
}

/**
 * 导入清单弹窗：粘贴链接 → 预览 → 确认保存。
 * 预览过期或失败时明确提示，不冒充成功。
 */
export default function ImportCollectionModal({
  isOpen,
  onClose,
  onImported,
}: ImportCollectionModalProps) {
  const [url, setUrl] = useState("");
  const [preview, setPreview] = useState<CollectionPreview | null>(null);
  const [previewing, setPreviewing] = useState(false);
  const [importing, setImporting] = useState(false);
  const [error, setError] = useState("");
  const dialogRef = useRef<HTMLDivElement>(null);
  const abortRef = useRef<AbortController | null>(null);

  useEffect(() => {
    if (!isOpen) return;
    const previous =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;
    dialogRef.current?.querySelector<HTMLInputElement>("input")?.focus();
    return () => previous?.focus();
  }, [isOpen]);

  useEffect(() => () => abortRef.current?.abort(), []);

  if (!isOpen) return null;

  const reset = () => {
    setUrl("");
    setPreview(null);
    setError("");
  };

  const close = () => {
    if (importing) return;
    if (previewing) {
      abortRef.current?.abort();
      setPreviewing(false);
    }
    reset();
    onClose();
  };

  const runPreview = async () => {
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    setPreviewing(true);
    setError("");
    setPreview(null);
    try {
      const result = await previewCollection(url.trim(), controller.signal);
      if (controller.signal.aborted) return;
      setPreview(result);
    } catch (caught) {
      if (controller.signal.aborted) return;
      setError(collectionErrorMessage(caught, "预览失败，可稍后重试。"));
    } finally {
      if (!controller.signal.aborted) setPreviewing(false);
    }
  };

  const runConfirm = async () => {
    if (!preview || preview.duplicate) return;
    setImporting(true);
    setError("");
    try {
      const result = await confirmCollectionImport(preview.preview_id);
      if (result.duplicate) {
        setPreview((current) =>
          current
            ? {
                ...current,
                duplicate: true,
                existing_collection_id: result.collection_id,
              }
            : current,
        );
        return;
      }
      reset();
      onClose();
      onImported({
        duplicate: result.duplicate,
        collectionID: result.collection_id,
      });
    } catch (caught) {
      const code = collectionErrorCode(caught);
      if (code === "PREVIEW_EXPIRED" || code === "PREVIEW_NOT_FOUND") {
        setError("预览已过期，请重新预览后再导入。");
        setPreview(null);
      } else {
        setError(collectionErrorMessage(caught, "导入失败，可稍后重试。"));
      }
    } finally {
      setImporting(false);
    }
  };

  const openExistingCollection = async () => {
    const collectionID = preview?.existing_collection_id;
    if (!preview?.duplicate || !collectionID) return;
    setImporting(true);
    setError("");
    try {
      await fetchCollectionDetail(collectionID);
      reset();
      onClose();
      onImported({ duplicate: true, collectionID });
    } catch (caught) {
      if (collectionErrorCode(caught) === "COLLECTION_NOT_FOUND") {
        setPreview((current) =>
          current
            ? { ...current, duplicate: false, existing_collection_id: null }
            : current,
        );
        setError("已有清单已删除，可以重新导入这份预览。");
      } else {
        setError(
          collectionErrorMessage(caught, "暂时无法打开已有清单，请稍后重试。"),
        );
      }
    } finally {
      setImporting(false);
    }
  };

  const changeURL = () => {
    if (importing) return;
    setPreview(null);
    setError("");
  };

  const handleKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
    if (event.key === "Escape") {
      event.preventDefault();
      event.stopPropagation();
      close();
      return;
    }
    if (event.key === "Tab") {
      const controls = Array.from(
        event.currentTarget.querySelectorAll<HTMLElement>(
          'button:not([disabled]), input:not([disabled]), a[href], [tabindex="0"]',
        ),
      ).filter((element) => element.getClientRects().length);
      const first = controls[0];
      const last = controls.at(-1);
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last?.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first?.focus();
      }
    }
  };

  return (
    <div className="editorial-modal-backdrop fixed inset-0 z-[60] flex items-center justify-center p-4">
      <div
        className="editorial-modal shadow-2xl w-full max-w-lg overflow-hidden flex flex-col max-h-[calc(100vh-2rem)]"
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby="import-collection-title"
        onKeyDown={handleKeyDown}
      >
        <div className="editorial-modal-header">
          <div className="editorial-modal-heading">
            <span className="editorial-modal-kicker">播客清单</span>
            <small>
              {preview?.duplicate
                ? "清单已存在"
                : preview
                  ? "确认导入"
                  : "粘贴链接"}
            </small>
          </div>
          <button
            type="button"
            onClick={close}
            className="editorial-modal-close"
            aria-label="关闭"
            disabled={importing}
          >
            <IconX aria-hidden="true" stroke={1.8} />
          </button>
        </div>

        <h2 id="import-collection-title" className="editorial-modal-title">
          导入清单
        </h2>

        <div className="editorial-modal-body overflow-y-auto">
          {!preview ? (
            <div className="flex flex-col gap-3">
              <label
                className="collection-form-label"
                htmlFor="collection-url-input"
              >
                小宇宙清单链接
              </label>
              <input
                id="collection-url-input"
                type="url"
                inputMode="url"
                value={url}
                onChange={(event) => setUrl(event.target.value)}
                placeholder="粘贴清单链接"
                className="collection-form-input"
                disabled={previewing}
                onKeyDown={(event) => {
                  if (event.key === "Enter" && url.trim() && !previewing) {
                    event.preventDefault();
                    void runPreview();
                  }
                }}
              />
              {previewing && (
                <p
                  className="collection-form-hint"
                  role="status"
                  aria-live="polite"
                >
                  正在读取清单，可稍候点击关闭取消…
                </p>
              )}
            </div>
          ) : (
            <div className="flex flex-col gap-3">
              <div>
                <h3 className="collection-preview-title">{preview.title}</h3>
                {!!preview.duplicate_item_count && (
                  <p className="collection-form-hint" role="status">
                    已读取 {preview.source_item_count} 个条目，合并 {preview.duplicate_item_count} 个重复条目，共 {preview.read_count} 集；不同推荐语已保留。
                  </p>
                )}
                <p className="collection-form-hint">
                  作者：{preview.author || "未提供"}
                  {preview.total_known
                    ? ` · 共 ${preview.read_count} 集`
                    : ` · 已读取 ${preview.read_count} 集`}
                </p>
                {preview.description && (
                  <p className="collection-preview-description">
                    {preview.description}
                  </p>
                )}
                {preview.duplicate && (
                  <p className="collection-duplicate-notice" role="alert">
                    这份清单已经导入过，无需重复导入；原有清单内容保持不变。
                  </p>
                )}
              </div>
              <ol className="collection-preview-items">
                {preview.items.map((item) => (
                  <li
                    key={item.external_episode_id}
                    className="collection-preview-item"
                  >
                    <span className="collection-preview-index">
                      {item.position + 1}
                    </span>
                    <span className="collection-preview-copy">
                      <strong>{item.episode_title}</strong>
                      <small>
                        {item.podcast_title}
                        {item.recommendation ? ` · ${item.recommendation}` : ""}
                      </small>
                    </span>
                  </li>
                ))}
              </ol>
              <p className="collection-form-hint">
                {preview.duplicate
                  ? "你可以打开已有清单，或修改链接查看另一份清单。"
                  : "将保存你正在预览的这份清单内容；不勾选任何单集，导入后仍需逐集决定是否收录。"}
              </p>
            </div>
          )}

          {error && (
            <p className="collection-form-error" role="alert">
              {error}
            </p>
          )}
        </div>

        <div className="editorial-modal-footer editorial-modal-actions">
          {preview ? (
            <>
              <button
                type="button"
                className="collection-btn-secondary"
                onClick={changeURL}
                disabled={importing}
              >
                修改链接
              </button>
              {preview.duplicate ? (
                <button
                  type="button"
                  className="collection-btn-primary"
                  onClick={() => void openExistingCollection()}
                  disabled={importing || !preview.existing_collection_id}
                >
                  打开已有清单
                </button>
              ) : (
                <button
                  type="button"
                  className="collection-btn-primary"
                  onClick={() => void runConfirm()}
                  disabled={importing}
                >
                  {importing ? "正在导入…" : "导入清单"}
                </button>
              )}
            </>
          ) : (
            <>
              <button
                type="button"
                className="collection-btn-secondary"
                onClick={close}
              >
                取消
              </button>
              <button
                type="button"
                className="collection-btn-primary"
                onClick={() => void runPreview()}
                disabled={previewing || !url.trim()}
              >
                {previewing ? "正在预览…" : "预览"}
              </button>
            </>
          )}
        </div>
      </div>
    </div>
  );
}
