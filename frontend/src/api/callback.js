import request from './request'

// 质量回访接口。
export const callbackApi = {
  list: (params) => request.get('/callbacks', { params }),
  detail: (id) => request.get(`/callbacks/${id}`),
  listByFault: (faultId) => request.get(`/callbacks/fault/${faultId}`),
  record: (id, data) => request.post(`/callbacks/${id}/record`, data),
  meta: () => request.get('/callbacks/meta'),
}
