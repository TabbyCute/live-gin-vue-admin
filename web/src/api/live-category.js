import service from '@/utils/request'

// @Summary 分页查询直播分类
// @Router /live/category/list [get]
export const getCategoryList = (params) => {
  return service({
    url: '/live/category/list',
    method: 'get',
    params
  })
}

// @Summary 获取后台直播分类树
// @Router /live/category/tree [get]
export const getCategoryTree = () => {
  return service({
    url: '/live/category/tree',
    method: 'get'
  })
}

// @Summary 创建直播分类
// @Router /live/category/create [post]
export const createCategory = (data) => {
  return service({
    url: '/live/category/create',
    method: 'post',
    data
  })
}

// @Summary 修改直播分类资料
// @Router /live/category/update [post]
export const updateCategory = (data) => {
  return service({
    url: '/live/category/update',
    method: 'post',
    data
  })
}

// @Summary 启用或停用直播分类
// @Router /live/category/status/update [post]
export const updateCategoryStatus = (data) => {
  return service({
    url: '/live/category/status/update',
    method: 'post',
    data
  })
}

// @Summary 删除直播分类
// @Router /live/category/delete [post]
export const deleteCategory = (data) => {
  return service({
    url: '/live/category/delete',
    method: 'post',
    data
  })
}
