import request from './request'

// 维修质量回访接口。
export const callbackApi = {
  list: (params) => request.get('/callbacks', { params }),
  detail: (id) => request.get(`/callbacks/${id}`),
  addContact: (id, data) => request.post(`/callbacks/${id}/contacts`, data),
  judge: (id, data) => request.post(`/callbacks/${id}/judge`, data),
  meta: () => request.get('/callbacks/meta'),
  statistics: () => request.get('/callbacks/statistics'),
}
