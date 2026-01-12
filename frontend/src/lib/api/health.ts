import { api } from './client'

export const healthApi = {
  check: async (): Promise<{ status: string; database: string }> => {
    const response = await api.get('/health')
    return response.data
  },
}
