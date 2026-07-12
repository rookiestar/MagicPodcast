import { api, handleVoidResponse } from "./client";
import type { ApiResponse } from "@/types";

export const cacheApi = {
  clear: async (confirmationText: string): Promise<void> => {
    const response = await api.post<ApiResponse<void>>("/api/v1/cache/clear", {
      confirmation_text: confirmationText,
    });
    handleVoidResponse(response);
  },
};
