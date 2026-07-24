/**
 * Shared path builders for workflow job summary requests.
 * Keep keys identical between useWorkflowJobs, intent prefetch, and list prefetch.
 */

export function buildWorkflowJobsSummaryPath(
  workflowId: number,
  page: number = 1,
  pageSize: number = 10,
): string {
  return `/api/v1/workflows/${workflowId}/jobs?page=${page}&page_size=${pageSize}&view=summary`;
}
