import { wsService } from './websocket.js'

export const comparisonService = {
  compareRuns: ({ runIds, labels, metricsEnabled }) =>
    wsService.request('REQ_COMPARE_RUNS', {
      run_ids: runIds,
      labels,
      metrics_enabled: metricsEnabled
    }),

  createComparison: (payload) =>
    wsService.request('REQ_CREATE_COMPARISON', payload),

  listComparisons: () =>
    wsService.request('REQ_LIST_COMPARISONS', {}),

  getComparison: (id, { refresh = false } = {}) =>
    wsService.request('REQ_GET_COMPARISON', { id, refresh }),

  deleteComparison: (id) =>
    wsService.request('REQ_DELETE_COMPARISON', { id }),

  listTemplates: () =>
    wsService.request('REQ_LIST_COMPARISON_TEMPLATES', {}),

  createTemplate: ({ name, config }) =>
    wsService.request('REQ_CREATE_COMPARISON_TEMPLATE', { name, config }),

  updateTemplate: ({ id, name, config }) =>
    wsService.request('REQ_UPDATE_COMPARISON_TEMPLATE', { id, name, config }),

  deleteTemplate: (id) =>
    wsService.request('REQ_DELETE_COMPARISON_TEMPLATE', { id })
}
