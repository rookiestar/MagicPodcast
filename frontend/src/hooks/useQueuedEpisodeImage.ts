import { useEffect, useMemo, useState } from "react";
import type { EpisodeImagePriority } from "@/lib/episodeDisplay";
import { getEpisodeImageLoadDelay } from "@/lib/episodeDisplay";
import { imageLoadQueue } from "@/lib/imageLoader";

interface UseQueuedEpisodeImageOptions {
  episodeId: number;
  src: string;
  priority: EpisodeImagePriority;
  index: number;
}

export function useQueuedEpisodeImage({
  episodeId,
  src,
  priority,
  index,
}: UseQueuedEpisodeImageOptions) {
  const [imageLoaded, setImageLoaded] = useState(false);
  const [imageError, setImageError] = useState(false);

  const taskId = useMemo(
    () => `episode-${episodeId}-${priority}-${index}-${src}`,
    [episodeId, index, priority, src],
  );

  useEffect(() => {
    setImageLoaded(false);
    setImageError(false);

    if (!src) {
      return;
    }

    let cancelled = false;
    const delay = getEpisodeImageLoadDelay(priority, index);

    const timeoutId = window.setTimeout(() => {
      if (cancelled) {
        return;
      }

      imageLoadQueue.add({
        id: taskId,
        src,
        priority,
        retryCount: 0,
        onSuccess: () => {
          if (!cancelled) {
            setImageLoaded(true);
          }
        },
        onError: () => {
          if (!cancelled) {
            setImageError(true);
            setImageLoaded(false);
          }
        },
      });
    }, delay);

    return () => {
      cancelled = true;
      window.clearTimeout(timeoutId);
      imageLoadQueue.cancel(taskId);
    };
  }, [index, priority, src, taskId]);

  return {
    imageLoaded,
    imageError,
  };
}
