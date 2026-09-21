import service from '@/utils/request'

// @Summary 分页查询主播列表
// @Router /live/anchor/list [get]
export const getAnchorList = (params) => {
  return service({
    url: '/live/anchor/list',
    method: 'get',
    params
  })
}

// @Summary 获取主播后台完整详情
// @Router /live/anchor/detail [get]
export const getAnchorDetail = (params) => {
  return service({
    url: '/live/anchor/detail',
    method: 'get',
    params
  })
}

// @Summary 审核主播申请
// @Router /live/anchor/audit [post]
export const auditAnchor = (data) => {
  return service({
    url: '/live/anchor/audit',
    method: 'post',
    data
  })
}

// @Summary 修改主播账号状态
// @Router /live/anchor/status/update [post]
export const updateAnchorStatus = (data) => {
  return service({
    url: '/live/anchor/status/update',
    method: 'post',
    data
  })
}

// @Summary 修改主播功能权限
// @Router /live/anchor/permission/update [post]
export const updateAnchorPermission = (data) => {
  return service({
    url: '/live/anchor/permission/update',
    method: 'post',
    data
  })
}

// @Summary 后台修改主播资料
// @Router /live/anchor/profile/update [post]
export const updateAnchorProfile = (data) => {
  return service({
    url: '/live/anchor/profile/update',
    method: 'post',
    data
  })
}

// @Summary 修改主播运营推荐属性
// @Router /live/anchor/recommend/update [post]
export const updateAnchorRecommend = (data) => {
  return service({
    url: '/live/anchor/recommend/update',
    method: 'post',
    data
  })
}

// @Summary 修改主播签约状态
// @Router /live/anchor/signed/update [post]
export const updateAnchorSigned = (data) => {
  return service({
    url: '/live/anchor/signed/update',
    method: 'post',
    data
  })
}

// @Summary 修改主播公会归属
// @Router /live/anchor/agency/update [post]
export const updateAnchorAgency = (data) => {
  return service({
    url: '/live/anchor/agency/update',
    method: 'post',
    data
  })
}

// @Summary 修改主播风险等级
// @Router /live/anchor/risk/update [post]
export const updateAnchorRisk = (data) => {
  return service({
    url: '/live/anchor/risk/update',
    method: 'post',
    data
  })
}

// @Summary 修改主播后台备注
// @Router /live/anchor/remark/update [post]
export const updateAnchorRemark = (data) => {
  return service({
    url: '/live/anchor/remark/update',
    method: 'post',
    data
  })
}

// @Summary 修改主播认证状态
// @Router /live/anchor/cert/update [post]
export const updateAnchorCert = (data) => {
  return service({
    url: '/live/anchor/cert/update',
    method: 'post',
    data
  })
}
