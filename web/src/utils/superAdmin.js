// Backend sys_users ID only; live APP account IDs use a separate identity domain.
export const isSuperAdmin = (userInfo) => String(userInfo?.ID ?? '') === '1'
