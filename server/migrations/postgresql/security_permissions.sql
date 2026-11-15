-- For an ALREADY INITIALIZED database only.
-- Adds explicit administrator permissions for the newly protected routes.
-- Does not remove accounts, change passwords, delete logs, or modify table structure.
BEGIN;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM public.sys_users
        WHERE username = 'admin' AND authority_id = 888 AND deleted_at IS NULL
    ) THEN
        RAISE EXCEPTION 'Expected admin with role 888; verify the role before applying.';
    END IF;
END;
$$;

INSERT INTO public.sys_apis (path, method, description, api_group, created_at, updated_at)
SELECT seed.path, seed.method, seed.description, seed.api_group, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
FROM (VALUES
    ('/autoCode/llmAuto', 'POST', 'AI 代码生成', '代码生成器'),
    ('/autoCode/llmAutoSSE', 'POST', 'AI 代码生成流式响应', '代码生成器'),
    ('/autoCode/initMenu', 'POST', '生成插件菜单文件', '代码生成器'),
    ('/autoCode/initAPI', 'POST', '生成插件 API 文件', '代码生成器'),
    ('/autoCode/initDictionary', 'POST', '生成插件字典文件', '代码生成器'),
    ('/skills/downloadOnlineSkill', 'POST', '下载在线技能', 'skills'),
    ('/api/freshCasbin', 'GET', '刷新权限缓存', 'api管理'),
    ('/system/reloadSystem', 'POST', '重载系统配置', '系统服务')
) AS seed(path, method, description, api_group)
WHERE NOT EXISTS (
    SELECT 1 FROM public.sys_apis a WHERE a.path = seed.path AND a.method = seed.method
);

INSERT INTO public.casbin_rule (ptype, v0, v1, v2, v3, v4, v5)
SELECT 'p', '888', a.path, a.method, '', '', ''
FROM public.sys_apis a
WHERE a.deleted_at IS NULL AND a.path IN (
    '/autoCode/llmAuto', '/autoCode/llmAutoSSE', '/autoCode/initMenu',
    '/autoCode/initAPI', '/autoCode/initDictionary', '/skills/downloadOnlineSkill',
    '/api/freshCasbin', '/system/reloadSystem'
) AND NOT EXISTS (
    SELECT 1 FROM public.casbin_rule p
    WHERE p.ptype = 'p' AND p.v0 = '888' AND p.v1 = a.path AND p.v2 = a.method
);

DELETE FROM public.sys_ignore_apis
WHERE (method = 'POST' AND path IN ('/autoCode/llmAuto', '/autoCode/llmAutoSSE', '/system/reloadSystem'))
   OR (method = 'GET' AND path = '/api/freshCasbin');

COMMIT;
