// 仅最新查询可以更新页面；切换用户、重置或离开页面会使旧响应失效。
export const createRequestScope = () => {
  let generation = 0
  const capture = () => {
    const current = generation
    return () => current === generation
  }
  return {
    capture,
    begin: () => { generation += 1; return capture() },
    invalidate: () => { generation += 1 }
  }
}
