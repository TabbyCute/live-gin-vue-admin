-- Gin-Vue-Admin PostgreSQL one-time initialization script.
-- Run this file once against an EXISTING, EMPTY database from Navicat.
-- The script creates the schema and all default data; it does not create the database.
-- Default administrator: admin / 888888 (stored as a bcrypt hash).
-- Role 888 is granted every API and menu included in this initialization snapshot.
-- Do not run this script against a database that already contains application tables.

--
-- PostgreSQL database dump
--


-- Dumped from database version 16.16
-- Dumped by pg_dump version 16.16

BEGIN;
-- Refuse non-empty targets; any error rolls back the entire initialization.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_catalog.pg_class c
        JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
        WHERE n.nspname = 'public' AND c.relkind IN ('r', 'p', 'v', 'm', 'S', 'f')
    ) THEN
        RAISE EXCEPTION 'Initialization requires an empty public schema. Existing data was not changed.';
    END IF;
END;
$$;
SET LOCAL statement_timeout = 0;
SET LOCAL lock_timeout = '10s';
SET LOCAL client_encoding = 'UTF8';
SET LOCAL standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', true);
SET LOCAL check_function_bodies = false;
SET LOCAL xmloption = content;
SET LOCAL client_min_messages = warning;

SET LOCAL default_tablespace = '';

SET LOCAL default_table_access_method = heap;

--
-- Name: casbin_rule; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.casbin_rule (
    id bigint NOT NULL,
    ptype character varying(100),
    v0 character varying(100),
    v1 character varying(100),
    v2 character varying(100),
    v3 character varying(100),
    v4 character varying(100),
    v5 character varying(100)
);


--
-- Name: casbin_rule_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.casbin_rule_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: casbin_rule_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.casbin_rule_id_seq OWNED BY public.casbin_rule.id;


--
-- Name: exa_attachment_category; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.exa_attachment_category (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    name character varying(255) DEFAULT NULL::character varying,
    pid bigint DEFAULT 0
);


--
-- Name: COLUMN exa_attachment_category.name; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.exa_attachment_category.name IS '分类名称';


--
-- Name: COLUMN exa_attachment_category.pid; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.exa_attachment_category.pid IS '父节点ID';


--
-- Name: exa_attachment_category_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.exa_attachment_category_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: exa_attachment_category_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.exa_attachment_category_id_seq OWNED BY public.exa_attachment_category.id;


--
-- Name: exa_customers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.exa_customers (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    customer_name text,
    customer_phone_data text,
    sys_user_id bigint,
    sys_user_authority_id bigint
);


--
-- Name: COLUMN exa_customers.customer_name; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.exa_customers.customer_name IS '客户名';


--
-- Name: COLUMN exa_customers.customer_phone_data; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.exa_customers.customer_phone_data IS '客户手机号';


--
-- Name: COLUMN exa_customers.sys_user_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.exa_customers.sys_user_id IS '管理ID';


--
-- Name: COLUMN exa_customers.sys_user_authority_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.exa_customers.sys_user_authority_id IS '管理角色ID';


--
-- Name: exa_customers_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.exa_customers_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: exa_customers_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.exa_customers_id_seq OWNED BY public.exa_customers.id;


--
-- Name: exa_file_chunks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.exa_file_chunks (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    exa_file_id bigint,
    file_chunk_number bigint,
    file_chunk_path text
);


--
-- Name: exa_file_chunks_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.exa_file_chunks_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: exa_file_chunks_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.exa_file_chunks_id_seq OWNED BY public.exa_file_chunks.id;


--
-- Name: exa_file_upload_and_downloads; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.exa_file_upload_and_downloads (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    name text,
    class_id bigint DEFAULT 0,
    url text,
    tag text,
    key text
);


--
-- Name: COLUMN exa_file_upload_and_downloads.name; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.exa_file_upload_and_downloads.name IS '文件名';


--
-- Name: COLUMN exa_file_upload_and_downloads.class_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.exa_file_upload_and_downloads.class_id IS '分类id';


--
-- Name: COLUMN exa_file_upload_and_downloads.url; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.exa_file_upload_and_downloads.url IS '文件地址';


--
-- Name: COLUMN exa_file_upload_and_downloads.tag; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.exa_file_upload_and_downloads.tag IS '文件标签';


--
-- Name: COLUMN exa_file_upload_and_downloads.key; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.exa_file_upload_and_downloads.key IS '编号';


--
-- Name: exa_file_upload_and_downloads_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.exa_file_upload_and_downloads_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: exa_file_upload_and_downloads_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.exa_file_upload_and_downloads_id_seq OWNED BY public.exa_file_upload_and_downloads.id;


--
-- Name: exa_files; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.exa_files (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    file_name text,
    file_md5 text,
    file_path text,
    chunk_total bigint,
    is_finish boolean
);


--
-- Name: exa_files_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.exa_files_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: exa_files_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.exa_files_id_seq OWNED BY public.exa_files.id;


--
-- Name: gva_announcements_info; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gva_announcements_info (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    title text,
    content text,
    user_id bigint,
    attachments jsonb
);


--
-- Name: COLUMN gva_announcements_info.title; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.gva_announcements_info.title IS '公告标题';


--
-- Name: COLUMN gva_announcements_info.content; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.gva_announcements_info.content IS '公告内容';


--
-- Name: COLUMN gva_announcements_info.user_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.gva_announcements_info.user_id IS '发布者';


--
-- Name: COLUMN gva_announcements_info.attachments; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.gva_announcements_info.attachments IS '相关附件';


--
-- Name: gva_announcements_info_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.gva_announcements_info_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: gva_announcements_info_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.gva_announcements_info_id_seq OWNED BY public.gva_announcements_info.id;


--
-- Name: jwt_blacklists; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.jwt_blacklists (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    jwt text
);


--
-- Name: COLUMN jwt_blacklists.jwt; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.jwt_blacklists.jwt IS 'jwt';


--
-- Name: jwt_blacklists_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.jwt_blacklists_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: jwt_blacklists_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.jwt_blacklists_id_seq OWNED BY public.jwt_blacklists.id;


--
-- Name: sys_ai_workflow_sessions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sys_ai_workflow_sessions (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    user_id bigint,
    tab character varying(32),
    title character varying(255),
    summary text,
    conversation_id character varying(255),
    message_id character varying(255),
    current_node_id character varying(64),
    settings jsonb,
    form_data jsonb,
    result_data jsonb,
    messages jsonb
);


--
-- Name: COLUMN sys_ai_workflow_sessions.user_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_ai_workflow_sessions.user_id IS '用户ID';


--
-- Name: COLUMN sys_ai_workflow_sessions.tab; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_ai_workflow_sessions.tab IS '会话类型';


--
-- Name: COLUMN sys_ai_workflow_sessions.title; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_ai_workflow_sessions.title IS '会话标题';


--
-- Name: COLUMN sys_ai_workflow_sessions.summary; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_ai_workflow_sessions.summary IS '摘要';


--
-- Name: COLUMN sys_ai_workflow_sessions.conversation_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_ai_workflow_sessions.conversation_id IS 'Dify会话ID';


--
-- Name: COLUMN sys_ai_workflow_sessions.message_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_ai_workflow_sessions.message_id IS 'Dify消息ID';


--
-- Name: COLUMN sys_ai_workflow_sessions.current_node_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_ai_workflow_sessions.current_node_id IS '当前选中节点ID';


--
-- Name: COLUMN sys_ai_workflow_sessions.settings; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_ai_workflow_sessions.settings IS '页面设置';


--
-- Name: COLUMN sys_ai_workflow_sessions.form_data; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_ai_workflow_sessions.form_data IS '表单数据';


--
-- Name: COLUMN sys_ai_workflow_sessions.result_data; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_ai_workflow_sessions.result_data IS '当前展示结果';


--
-- Name: COLUMN sys_ai_workflow_sessions.messages; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_ai_workflow_sessions.messages IS '会话消息';


--
-- Name: sys_ai_workflow_sessions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.sys_ai_workflow_sessions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sys_ai_workflow_sessions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.sys_ai_workflow_sessions_id_seq OWNED BY public.sys_ai_workflow_sessions.id;


--
-- Name: sys_api_tokens; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sys_api_tokens (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    user_id bigint,
    authority_id bigint,
    token text,
    status boolean DEFAULT true,
    expires_at timestamp with time zone,
    remark text
);


--
-- Name: COLUMN sys_api_tokens.user_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_api_tokens.user_id IS '用户ID';


--
-- Name: COLUMN sys_api_tokens.authority_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_api_tokens.authority_id IS '角色ID';


--
-- Name: COLUMN sys_api_tokens.token; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_api_tokens.token IS 'Token';


--
-- Name: COLUMN sys_api_tokens.status; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_api_tokens.status IS '状态';


--
-- Name: COLUMN sys_api_tokens.expires_at; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_api_tokens.expires_at IS '过期时间';


--
-- Name: COLUMN sys_api_tokens.remark; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_api_tokens.remark IS '备注';


--
-- Name: sys_api_tokens_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.sys_api_tokens_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sys_api_tokens_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.sys_api_tokens_id_seq OWNED BY public.sys_api_tokens.id;


--
-- Name: sys_apis; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sys_apis (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    path text,
    description text,
    api_group text,
    method text DEFAULT 'POST'::text
);


--
-- Name: COLUMN sys_apis.path; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_apis.path IS 'api路径';


--
-- Name: COLUMN sys_apis.description; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_apis.description IS 'api中文描述';


--
-- Name: COLUMN sys_apis.api_group; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_apis.api_group IS 'api组';


--
-- Name: COLUMN sys_apis.method; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_apis.method IS '方法';


--
-- Name: sys_apis_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.sys_apis_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sys_apis_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.sys_apis_id_seq OWNED BY public.sys_apis.id;


--
-- Name: sys_authorities; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sys_authorities (
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    authority_id bigint NOT NULL,
    authority_name text,
    parent_id bigint,
    default_router text DEFAULT 'dashboard'::text
);


--
-- Name: COLUMN sys_authorities.authority_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_authorities.authority_id IS '角色ID';


--
-- Name: COLUMN sys_authorities.authority_name; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_authorities.authority_name IS '角色名';


--
-- Name: COLUMN sys_authorities.parent_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_authorities.parent_id IS '父角色ID';


--
-- Name: COLUMN sys_authorities.default_router; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_authorities.default_router IS '默认菜单';


--
-- Name: sys_authorities_authority_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.sys_authorities_authority_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sys_authorities_authority_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.sys_authorities_authority_id_seq OWNED BY public.sys_authorities.authority_id;


--
-- Name: sys_authority_btns; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sys_authority_btns (
    authority_id bigint,
    sys_menu_id bigint,
    sys_base_menu_btn_id bigint
);


--
-- Name: COLUMN sys_authority_btns.authority_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_authority_btns.authority_id IS '角色ID';


--
-- Name: COLUMN sys_authority_btns.sys_menu_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_authority_btns.sys_menu_id IS '菜单ID';


--
-- Name: COLUMN sys_authority_btns.sys_base_menu_btn_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_authority_btns.sys_base_menu_btn_id IS '菜单按钮ID';


--
-- Name: sys_authority_menus; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sys_authority_menus (
    sys_base_menu_id bigint NOT NULL,
    sys_authority_authority_id bigint NOT NULL
);


--
-- Name: COLUMN sys_authority_menus.sys_authority_authority_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_authority_menus.sys_authority_authority_id IS '角色ID';


--
-- Name: sys_auto_code_histories; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sys_auto_code_histories (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    table_name text,
    package text,
    request text,
    struct_name text,
    abbreviation text,
    business_db text,
    description text,
    templates text,
    injections text,
    flag bigint,
    api_ids text,
    menu_id bigint,
    export_template_id bigint,
    package_id bigint
);


--
-- Name: COLUMN sys_auto_code_histories.table_name; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_auto_code_histories.table_name IS '表名';


--
-- Name: COLUMN sys_auto_code_histories.package; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_auto_code_histories.package IS '模块名或插件名';


--
-- Name: COLUMN sys_auto_code_histories.request; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_auto_code_histories.request IS '前端传入的结构化信息';


--
-- Name: COLUMN sys_auto_code_histories.struct_name; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_auto_code_histories.struct_name IS '结构体名称';


--
-- Name: COLUMN sys_auto_code_histories.abbreviation; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_auto_code_histories.abbreviation IS '结构体简称';


--
-- Name: COLUMN sys_auto_code_histories.business_db; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_auto_code_histories.business_db IS '业务库';


--
-- Name: COLUMN sys_auto_code_histories.description; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_auto_code_histories.description IS '结构体中文名';


--
-- Name: COLUMN sys_auto_code_histories.templates; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_auto_code_histories.templates IS '模板信息';


--
-- Name: COLUMN sys_auto_code_histories.injections; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_auto_code_histories.injections IS '注入信息';


--
-- Name: COLUMN sys_auto_code_histories.flag; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_auto_code_histories.flag IS '[0:创建,1:回滚]';


--
-- Name: COLUMN sys_auto_code_histories.api_ids; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_auto_code_histories.api_ids IS '关联API ID';


--
-- Name: COLUMN sys_auto_code_histories.menu_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_auto_code_histories.menu_id IS '菜单ID';


--
-- Name: COLUMN sys_auto_code_histories.export_template_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_auto_code_histories.export_template_id IS '导出模板ID';


--
-- Name: COLUMN sys_auto_code_histories.package_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_auto_code_histories.package_id IS '包ID';


--
-- Name: sys_auto_code_histories_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.sys_auto_code_histories_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sys_auto_code_histories_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.sys_auto_code_histories_id_seq OWNED BY public.sys_auto_code_histories.id;


--
-- Name: sys_auto_code_packages; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sys_auto_code_packages (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    "desc" text,
    label text,
    template text,
    package_name text,
    module text
);


--
-- Name: COLUMN sys_auto_code_packages."desc"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_auto_code_packages."desc" IS '描述';


--
-- Name: COLUMN sys_auto_code_packages.label; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_auto_code_packages.label IS '显示名称';


--
-- Name: COLUMN sys_auto_code_packages.template; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_auto_code_packages.template IS '模板';


--
-- Name: COLUMN sys_auto_code_packages.package_name; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_auto_code_packages.package_name IS '包名';


--
-- Name: sys_auto_code_packages_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.sys_auto_code_packages_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sys_auto_code_packages_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.sys_auto_code_packages_id_seq OWNED BY public.sys_auto_code_packages.id;


--
-- Name: sys_base_menu_btns; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sys_base_menu_btns (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    name text,
    "desc" text,
    sys_base_menu_id bigint
);


--
-- Name: COLUMN sys_base_menu_btns.name; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_base_menu_btns.name IS '按钮关键key';


--
-- Name: COLUMN sys_base_menu_btns.sys_base_menu_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_base_menu_btns.sys_base_menu_id IS '菜单ID';


--
-- Name: sys_base_menu_btns_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.sys_base_menu_btns_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sys_base_menu_btns_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.sys_base_menu_btns_id_seq OWNED BY public.sys_base_menu_btns.id;


--
-- Name: sys_base_menu_parameters; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sys_base_menu_parameters (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    sys_base_menu_id bigint,
    type text,
    key text,
    value text
);


--
-- Name: COLUMN sys_base_menu_parameters.type; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_base_menu_parameters.type IS '地址栏携带参数为params还是query';


--
-- Name: COLUMN sys_base_menu_parameters.key; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_base_menu_parameters.key IS '地址栏携带参数的key';


--
-- Name: COLUMN sys_base_menu_parameters.value; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_base_menu_parameters.value IS '地址栏携带参数的值';


--
-- Name: sys_base_menu_parameters_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.sys_base_menu_parameters_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sys_base_menu_parameters_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.sys_base_menu_parameters_id_seq OWNED BY public.sys_base_menu_parameters.id;


--
-- Name: sys_base_menus; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sys_base_menus (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    menu_level bigint,
    parent_id bigint,
    path text,
    name text,
    hidden boolean,
    component text,
    sort bigint,
    active_name text,
    keep_alive boolean,
    default_menu boolean,
    title text,
    icon text,
    close_tab boolean,
    transition_type text
);


--
-- Name: COLUMN sys_base_menus.parent_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_base_menus.parent_id IS '父菜单ID';


--
-- Name: COLUMN sys_base_menus.path; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_base_menus.path IS '路由path';


--
-- Name: COLUMN sys_base_menus.name; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_base_menus.name IS '路由name';


--
-- Name: COLUMN sys_base_menus.hidden; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_base_menus.hidden IS '是否在列表隐藏';


--
-- Name: COLUMN sys_base_menus.component; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_base_menus.component IS '对应前端文件路径';


--
-- Name: COLUMN sys_base_menus.sort; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_base_menus.sort IS '排序标记';


--
-- Name: COLUMN sys_base_menus.active_name; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_base_menus.active_name IS '高亮菜单';


--
-- Name: COLUMN sys_base_menus.keep_alive; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_base_menus.keep_alive IS '是否缓存';


--
-- Name: COLUMN sys_base_menus.default_menu; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_base_menus.default_menu IS '是否是基础路由（开发中）';


--
-- Name: COLUMN sys_base_menus.title; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_base_menus.title IS '菜单名';


--
-- Name: COLUMN sys_base_menus.icon; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_base_menus.icon IS '菜单图标';


--
-- Name: COLUMN sys_base_menus.close_tab; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_base_menus.close_tab IS '自动关闭tab';


--
-- Name: COLUMN sys_base_menus.transition_type; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_base_menus.transition_type IS '路由切换动画';


--
-- Name: sys_base_menus_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.sys_base_menus_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sys_base_menus_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.sys_base_menus_id_seq OWNED BY public.sys_base_menus.id;


--
-- Name: sys_data_authority_id; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sys_data_authority_id (
    sys_authority_authority_id bigint NOT NULL,
    data_authority_id_authority_id bigint NOT NULL
);


--
-- Name: COLUMN sys_data_authority_id.sys_authority_authority_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_data_authority_id.sys_authority_authority_id IS '角色ID';


--
-- Name: COLUMN sys_data_authority_id.data_authority_id_authority_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_data_authority_id.data_authority_id_authority_id IS '角色ID';


--
-- Name: sys_dictionaries; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sys_dictionaries (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    name text,
    type text,
    status boolean,
    "desc" text,
    parent_id bigint
);


--
-- Name: COLUMN sys_dictionaries.name; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_dictionaries.name IS '字典名（中）';


--
-- Name: COLUMN sys_dictionaries.type; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_dictionaries.type IS '字典名（英）';


--
-- Name: COLUMN sys_dictionaries.status; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_dictionaries.status IS '状态';


--
-- Name: COLUMN sys_dictionaries."desc"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_dictionaries."desc" IS '描述';


--
-- Name: COLUMN sys_dictionaries.parent_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_dictionaries.parent_id IS '父级字典ID';


--
-- Name: sys_dictionaries_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.sys_dictionaries_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sys_dictionaries_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.sys_dictionaries_id_seq OWNED BY public.sys_dictionaries.id;


--
-- Name: sys_dictionary_details; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sys_dictionary_details (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    label text,
    value text,
    extend text,
    status boolean,
    sort bigint,
    sys_dictionary_id bigint,
    parent_id bigint,
    level bigint,
    path text
);


--
-- Name: COLUMN sys_dictionary_details.label; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_dictionary_details.label IS '展示值';


--
-- Name: COLUMN sys_dictionary_details.value; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_dictionary_details.value IS '字典值';


--
-- Name: COLUMN sys_dictionary_details.extend; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_dictionary_details.extend IS '扩展值';


--
-- Name: COLUMN sys_dictionary_details.status; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_dictionary_details.status IS '启用状态';


--
-- Name: COLUMN sys_dictionary_details.sort; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_dictionary_details.sort IS '排序标记';


--
-- Name: COLUMN sys_dictionary_details.sys_dictionary_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_dictionary_details.sys_dictionary_id IS '关联标记';


--
-- Name: COLUMN sys_dictionary_details.parent_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_dictionary_details.parent_id IS '父级字典详情ID';


--
-- Name: COLUMN sys_dictionary_details.level; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_dictionary_details.level IS '层级深度';


--
-- Name: COLUMN sys_dictionary_details.path; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_dictionary_details.path IS '层级路径';


--
-- Name: sys_dictionary_details_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.sys_dictionary_details_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sys_dictionary_details_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.sys_dictionary_details_id_seq OWNED BY public.sys_dictionary_details.id;


--
-- Name: sys_error; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sys_error (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    form text,
    info text,
    level text,
    solution text,
    status character varying(20) DEFAULT '未处理'::character varying
);


--
-- Name: COLUMN sys_error.form; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_error.form IS '错误来源';


--
-- Name: COLUMN sys_error.info; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_error.info IS '错误内容';


--
-- Name: COLUMN sys_error.level; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_error.level IS '日志等级';


--
-- Name: COLUMN sys_error.solution; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_error.solution IS '解决方案';


--
-- Name: COLUMN sys_error.status; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_error.status IS '处理状态';


--
-- Name: sys_error_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.sys_error_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sys_error_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.sys_error_id_seq OWNED BY public.sys_error.id;


--
-- Name: sys_export_template_condition; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sys_export_template_condition (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    template_id text,
    "from" text,
    "column" text,
    operator text
);


--
-- Name: COLUMN sys_export_template_condition.template_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_export_template_condition.template_id IS '模板标识';


--
-- Name: COLUMN sys_export_template_condition."from"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_export_template_condition."from" IS '条件取的key';


--
-- Name: COLUMN sys_export_template_condition."column"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_export_template_condition."column" IS '作为查询条件的字段';


--
-- Name: COLUMN sys_export_template_condition.operator; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_export_template_condition.operator IS '操作符';


--
-- Name: sys_export_template_condition_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.sys_export_template_condition_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sys_export_template_condition_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.sys_export_template_condition_id_seq OWNED BY public.sys_export_template_condition.id;


--
-- Name: sys_export_template_join; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sys_export_template_join (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    template_id text,
    joins text,
    "table" text,
    "on" text
);


--
-- Name: COLUMN sys_export_template_join.template_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_export_template_join.template_id IS '模板标识';


--
-- Name: COLUMN sys_export_template_join.joins; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_export_template_join.joins IS '关联';


--
-- Name: COLUMN sys_export_template_join."table"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_export_template_join."table" IS '关联表';


--
-- Name: COLUMN sys_export_template_join."on"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_export_template_join."on" IS '关联条件';


--
-- Name: sys_export_template_join_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.sys_export_template_join_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sys_export_template_join_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.sys_export_template_join_id_seq OWNED BY public.sys_export_template_join.id;


--
-- Name: sys_export_templates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sys_export_templates (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    db_name text,
    name text,
    table_name text,
    template_id text,
    template_info text,
    sql text,
    import_sql text,
    "limit" bigint,
    "order" text
);


--
-- Name: COLUMN sys_export_templates.db_name; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_export_templates.db_name IS '数据库名称';


--
-- Name: COLUMN sys_export_templates.name; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_export_templates.name IS '模板名称';


--
-- Name: COLUMN sys_export_templates.table_name; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_export_templates.table_name IS '表名称';


--
-- Name: COLUMN sys_export_templates.template_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_export_templates.template_id IS '模板标识';


--
-- Name: COLUMN sys_export_templates.sql; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_export_templates.sql IS '自定义导出SQL';


--
-- Name: COLUMN sys_export_templates.import_sql; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_export_templates.import_sql IS '自定义导入SQL';


--
-- Name: COLUMN sys_export_templates."limit"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_export_templates."limit" IS '导出限制';


--
-- Name: COLUMN sys_export_templates."order"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_export_templates."order" IS '排序';


--
-- Name: sys_export_templates_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.sys_export_templates_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sys_export_templates_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.sys_export_templates_id_seq OWNED BY public.sys_export_templates.id;


--
-- Name: sys_ignore_apis; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sys_ignore_apis (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    path text,
    method text DEFAULT 'POST'::text
);


--
-- Name: COLUMN sys_ignore_apis.path; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_ignore_apis.path IS 'api路径';


--
-- Name: COLUMN sys_ignore_apis.method; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_ignore_apis.method IS '方法';


--
-- Name: sys_ignore_apis_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.sys_ignore_apis_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sys_ignore_apis_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.sys_ignore_apis_id_seq OWNED BY public.sys_ignore_apis.id;


--
-- Name: sys_login_logs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sys_login_logs (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    username text,
    ip text,
    status boolean,
    error_message text,
    agent text,
    user_id bigint
);


--
-- Name: COLUMN sys_login_logs.username; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_login_logs.username IS '用户名';


--
-- Name: COLUMN sys_login_logs.ip; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_login_logs.ip IS '请求ip';


--
-- Name: COLUMN sys_login_logs.status; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_login_logs.status IS '登录状态';


--
-- Name: COLUMN sys_login_logs.error_message; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_login_logs.error_message IS '错误信息';


--
-- Name: COLUMN sys_login_logs.agent; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_login_logs.agent IS '代理';


--
-- Name: COLUMN sys_login_logs.user_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_login_logs.user_id IS '用户id';


--
-- Name: sys_login_logs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.sys_login_logs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sys_login_logs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.sys_login_logs_id_seq OWNED BY public.sys_login_logs.id;


--
-- Name: sys_operation_records; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sys_operation_records (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    ip text,
    method text,
    path text,
    status bigint,
    latency bigint,
    agent text,
    error_message text,
    body text,
    resp text,
    user_id bigint
);


--
-- Name: COLUMN sys_operation_records.ip; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_operation_records.ip IS '请求ip';


--
-- Name: COLUMN sys_operation_records.method; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_operation_records.method IS '请求方法';


--
-- Name: COLUMN sys_operation_records.path; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_operation_records.path IS '请求路径';


--
-- Name: COLUMN sys_operation_records.status; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_operation_records.status IS '请求状态';


--
-- Name: COLUMN sys_operation_records.latency; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_operation_records.latency IS '延迟';


--
-- Name: COLUMN sys_operation_records.agent; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_operation_records.agent IS '代理';


--
-- Name: COLUMN sys_operation_records.error_message; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_operation_records.error_message IS '错误信息';


--
-- Name: COLUMN sys_operation_records.body; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_operation_records.body IS '请求Body';


--
-- Name: COLUMN sys_operation_records.resp; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_operation_records.resp IS '响应Body';


--
-- Name: COLUMN sys_operation_records.user_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_operation_records.user_id IS '用户id';


--
-- Name: sys_operation_records_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.sys_operation_records_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sys_operation_records_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.sys_operation_records_id_seq OWNED BY public.sys_operation_records.id;


--
-- Name: sys_params; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sys_params (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    name text,
    key text,
    value text,
    "desc" text
);


--
-- Name: COLUMN sys_params.name; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_params.name IS '参数名称';


--
-- Name: COLUMN sys_params.key; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_params.key IS '参数键';


--
-- Name: COLUMN sys_params.value; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_params.value IS '参数值';


--
-- Name: COLUMN sys_params."desc"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_params."desc" IS '参数说明';


--
-- Name: sys_params_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.sys_params_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sys_params_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.sys_params_id_seq OWNED BY public.sys_params.id;


--
-- Name: sys_user_authority; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sys_user_authority (
    sys_user_id bigint NOT NULL,
    sys_authority_authority_id bigint NOT NULL
);


--
-- Name: COLUMN sys_user_authority.sys_authority_authority_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_user_authority.sys_authority_authority_id IS '角色ID';


--
-- Name: sys_users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sys_users (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    uuid text,
    username text,
    password text,
    nick_name text DEFAULT '系统用户'::text,
    header_img text DEFAULT 'https://qmplusimg.henrongyi.top/gva_header.jpg'::text,
    authority_id bigint DEFAULT 888,
    phone text,
    email text,
    enable bigint DEFAULT 1,
    origin_setting jsonb
);


--
-- Name: COLUMN sys_users.uuid; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_users.uuid IS '用户UUID';


--
-- Name: COLUMN sys_users.username; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_users.username IS '用户登录名';


--
-- Name: COLUMN sys_users.password; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_users.password IS '用户登录密码';


--
-- Name: COLUMN sys_users.nick_name; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_users.nick_name IS '用户昵称';


--
-- Name: COLUMN sys_users.header_img; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_users.header_img IS '用户头像';


--
-- Name: COLUMN sys_users.authority_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_users.authority_id IS '用户角色ID';


--
-- Name: COLUMN sys_users.phone; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_users.phone IS '用户手机号';


--
-- Name: COLUMN sys_users.email; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_users.email IS '用户邮箱';


--
-- Name: COLUMN sys_users.enable; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_users.enable IS '用户是否被冻结 1正常 2冻结';


--
-- Name: COLUMN sys_users.origin_setting; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_users.origin_setting IS '配置';


--
-- Name: sys_users_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.sys_users_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sys_users_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.sys_users_id_seq OWNED BY public.sys_users.id;


--
-- Name: sys_versions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sys_versions (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    version_name character varying(255),
    version_code character varying(100),
    description character varying(500),
    version_data text
);


--
-- Name: COLUMN sys_versions.version_name; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_versions.version_name IS '版本名称';


--
-- Name: COLUMN sys_versions.version_code; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_versions.version_code IS '版本号';


--
-- Name: COLUMN sys_versions.description; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_versions.description IS '版本描述';


--
-- Name: COLUMN sys_versions.version_data; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sys_versions.version_data IS '版本数据JSON';


--
-- Name: sys_versions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.sys_versions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sys_versions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.sys_versions_id_seq OWNED BY public.sys_versions.id;


--
-- Name: casbin_rule id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.casbin_rule ALTER COLUMN id SET DEFAULT nextval('public.casbin_rule_id_seq'::regclass);


--
-- Name: exa_attachment_category id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.exa_attachment_category ALTER COLUMN id SET DEFAULT nextval('public.exa_attachment_category_id_seq'::regclass);


--
-- Name: exa_customers id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.exa_customers ALTER COLUMN id SET DEFAULT nextval('public.exa_customers_id_seq'::regclass);


--
-- Name: exa_file_chunks id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.exa_file_chunks ALTER COLUMN id SET DEFAULT nextval('public.exa_file_chunks_id_seq'::regclass);


--
-- Name: exa_file_upload_and_downloads id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.exa_file_upload_and_downloads ALTER COLUMN id SET DEFAULT nextval('public.exa_file_upload_and_downloads_id_seq'::regclass);


--
-- Name: exa_files id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.exa_files ALTER COLUMN id SET DEFAULT nextval('public.exa_files_id_seq'::regclass);


--
-- Name: gva_announcements_info id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gva_announcements_info ALTER COLUMN id SET DEFAULT nextval('public.gva_announcements_info_id_seq'::regclass);


--
-- Name: jwt_blacklists id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.jwt_blacklists ALTER COLUMN id SET DEFAULT nextval('public.jwt_blacklists_id_seq'::regclass);


--
-- Name: sys_ai_workflow_sessions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sys_ai_workflow_sessions ALTER COLUMN id SET DEFAULT nextval('public.sys_ai_workflow_sessions_id_seq'::regclass);


--
-- Name: sys_api_tokens id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sys_api_tokens ALTER COLUMN id SET DEFAULT nextval('public.sys_api_tokens_id_seq'::regclass);


--
-- Name: sys_apis id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sys_apis ALTER COLUMN id SET DEFAULT nextval('public.sys_apis_id_seq'::regclass);


--
-- Name: sys_authorities authority_id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sys_authorities ALTER COLUMN authority_id SET DEFAULT nextval('public.sys_authorities_authority_id_seq'::regclass);


--
-- Name: sys_auto_code_histories id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sys_auto_code_histories ALTER COLUMN id SET DEFAULT nextval('public.sys_auto_code_histories_id_seq'::regclass);


--
-- Name: sys_auto_code_packages id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sys_auto_code_packages ALTER COLUMN id SET DEFAULT nextval('public.sys_auto_code_packages_id_seq'::regclass);


--
-- Name: sys_base_menu_btns id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sys_base_menu_btns ALTER COLUMN id SET DEFAULT nextval('public.sys_base_menu_btns_id_seq'::regclass);


--
-- Name: sys_base_menu_parameters id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sys_base_menu_parameters ALTER COLUMN id SET DEFAULT nextval('public.sys_base_menu_parameters_id_seq'::regclass);


--
-- Name: sys_base_menus id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sys_base_menus ALTER COLUMN id SET DEFAULT nextval('public.sys_base_menus_id_seq'::regclass);


--
-- Name: sys_dictionaries id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sys_dictionaries ALTER COLUMN id SET DEFAULT nextval('public.sys_dictionaries_id_seq'::regclass);


--
-- Name: sys_dictionary_details id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sys_dictionary_details ALTER COLUMN id SET DEFAULT nextval('public.sys_dictionary_details_id_seq'::regclass);


--
-- Name: sys_error id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sys_error ALTER COLUMN id SET DEFAULT nextval('public.sys_error_id_seq'::regclass);


--
-- Name: sys_export_template_condition id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sys_export_template_condition ALTER COLUMN id SET DEFAULT nextval('public.sys_export_template_condition_id_seq'::regclass);


--
-- Name: sys_export_template_join id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sys_export_template_join ALTER COLUMN id SET DEFAULT nextval('public.sys_export_template_join_id_seq'::regclass);


--
-- Name: sys_export_templates id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sys_export_templates ALTER COLUMN id SET DEFAULT nextval('public.sys_export_templates_id_seq'::regclass);


--
-- Name: sys_ignore_apis id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sys_ignore_apis ALTER COLUMN id SET DEFAULT nextval('public.sys_ignore_apis_id_seq'::regclass);


--
-- Name: sys_login_logs id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sys_login_logs ALTER COLUMN id SET DEFAULT nextval('public.sys_login_logs_id_seq'::regclass);


--
-- Name: sys_operation_records id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sys_operation_records ALTER COLUMN id SET DEFAULT nextval('public.sys_operation_records_id_seq'::regclass);


--
-- Name: sys_params id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sys_params ALTER COLUMN id SET DEFAULT nextval('public.sys_params_id_seq'::regclass);


--
-- Name: sys_users id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sys_users ALTER COLUMN id SET DEFAULT nextval('public.sys_users_id_seq'::regclass);


--
-- Name: sys_versions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sys_versions ALTER COLUMN id SET DEFAULT nextval('public.sys_versions_id_seq'::regclass);


--
-- Data for Name: casbin_rule; Type: TABLE DATA; Schema: public; Owner: -
--

INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (1, 'p', '888', '/user/admin_register', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (2, 'p', '888', '/sysLoginLog/deleteLoginLog', 'DELETE', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (3, 'p', '888', '/sysLoginLog/deleteLoginLogByIds', 'DELETE', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (4, 'p', '888', '/sysLoginLog/findLoginLog', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (5, 'p', '888', '/sysLoginLog/getLoginLogList', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (6, 'p', '888', '/sysApiToken/createApiToken', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (7, 'p', '888', '/sysApiToken/getApiTokenList', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (8, 'p', '888', '/sysApiToken/deleteApiToken', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (9, 'p', '888', '/api/createApi', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (10, 'p', '888', '/api/getApiList', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (11, 'p', '888', '/api/getApiById', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (12, 'p', '888', '/api/deleteApi', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (13, 'p', '888', '/api/updateApi', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (14, 'p', '888', '/api/getAllApis', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (15, 'p', '888', '/api/deleteApisByIds', 'DELETE', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (16, 'p', '888', '/api/syncApi', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (17, 'p', '888', '/api/getApiGroups', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (18, 'p', '888', '/api/enterSyncApi', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (19, 'p', '888', '/api/ignoreApi', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (20, 'p', '888', '/api/getApiRoles', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (21, 'p', '888', '/api/setApiRoles', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (22, 'p', '888', '/authority/copyAuthority', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (23, 'p', '888', '/authority/updateAuthority', 'PUT', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (24, 'p', '888', '/authority/createAuthority', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (25, 'p', '888', '/authority/deleteAuthority', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (26, 'p', '888', '/authority/getAuthorityList', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (27, 'p', '888', '/authority/setDataAuthority', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (28, 'p', '888', '/authority/getUsersByAuthority', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (29, 'p', '888', '/authority/setRoleUsers', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (30, 'p', '888', '/menu/getMenu', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (31, 'p', '888', '/menu/getMenuList', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (32, 'p', '888', '/menu/addBaseMenu', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (33, 'p', '888', '/menu/getBaseMenuTree', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (34, 'p', '888', '/menu/addMenuAuthority', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (35, 'p', '888', '/menu/getMenuAuthority', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (36, 'p', '888', '/menu/getMenuRoles', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (37, 'p', '888', '/menu/setMenuRoles', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (38, 'p', '888', '/menu/deleteBaseMenu', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (39, 'p', '888', '/menu/updateBaseMenu', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (40, 'p', '888', '/menu/getBaseMenuById', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (41, 'p', '888', '/user/getUserInfo', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (42, 'p', '888', '/user/setUserInfo', 'PUT', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (43, 'p', '888', '/user/setSelfInfo', 'PUT', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (44, 'p', '888', '/user/getUserList', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (45, 'p', '888', '/user/deleteUser', 'DELETE', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (46, 'p', '888', '/user/changePassword', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (47, 'p', '888', '/user/setUserAuthority', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (48, 'p', '888', '/user/setUserAuthorities', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (49, 'p', '888', '/user/resetPassword', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (50, 'p', '888', '/user/setSelfSetting', 'PUT', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (51, 'p', '888', '/fileUploadAndDownload/findFile', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (52, 'p', '888', '/fileUploadAndDownload/breakpointContinueFinish', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (53, 'p', '888', '/fileUploadAndDownload/breakpointContinue', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (54, 'p', '888', '/fileUploadAndDownload/removeChunk', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (55, 'p', '888', '/fileUploadAndDownload/upload', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (56, 'p', '888', '/fileUploadAndDownload/deleteFile', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (57, 'p', '888', '/fileUploadAndDownload/editFileName', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (58, 'p', '888', '/fileUploadAndDownload/getFileList', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (59, 'p', '888', '/fileUploadAndDownload/importURL', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (60, 'p', '888', '/casbin/updateCasbin', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (61, 'p', '888', '/casbin/getPolicyPathByAuthorityId', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (62, 'p', '888', '/jwt/jsonInBlacklist', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (63, 'p', '888', '/system/getSystemConfig', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (64, 'p', '888', '/system/setSystemConfig', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (65, 'p', '888', '/system/getServerInfo', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (66, 'p', '888', '/skills/getTools', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (67, 'p', '888', '/skills/getSkillList', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (68, 'p', '888', '/skills/getSkillDetail', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (69, 'p', '888', '/skills/saveSkill', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (70, 'p', '888', '/skills/deleteSkill', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (71, 'p', '888', '/skills/createScript', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (72, 'p', '888', '/skills/getScript', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (73, 'p', '888', '/skills/saveScript', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (74, 'p', '888', '/skills/createResource', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (75, 'p', '888', '/skills/getResource', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (76, 'p', '888', '/skills/saveResource', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (77, 'p', '888', '/skills/createReference', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (78, 'p', '888', '/skills/getReference', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (79, 'p', '888', '/skills/saveReference', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (80, 'p', '888', '/skills/createTemplate', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (81, 'p', '888', '/skills/getTemplate', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (82, 'p', '888', '/skills/saveTemplate', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (83, 'p', '888', '/skills/getGlobalConstraint', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (84, 'p', '888', '/skills/saveGlobalConstraint', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (85, 'p', '888', '/skills/packageSkill', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (86, 'p', '888', '/customer/customer', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (87, 'p', '888', '/customer/customer', 'PUT', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (88, 'p', '888', '/customer/customer', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (89, 'p', '888', '/customer/customer', 'DELETE', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (90, 'p', '888', '/customer/customerList', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (91, 'p', '888', '/autoCode/getDB', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (92, 'p', '888', '/autoCode/getMeta', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (93, 'p', '888', '/autoCode/preview', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (94, 'p', '888', '/autoCode/getTables', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (95, 'p', '888', '/autoCode/getColumn', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (96, 'p', '888', '/autoCode/rollback', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (97, 'p', '888', '/autoCode/createTemp', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (98, 'p', '888', '/autoCode/delSysHistory', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (99, 'p', '888', '/autoCode/getSysHistory', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (100, 'p', '888', '/autoCode/createPackage', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (101, 'p', '888', '/autoCode/getTemplates', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (102, 'p', '888', '/autoCode/getPackage', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (103, 'p', '888', '/autoCode/delPackage', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (105, 'p', '888', '/autoCode/installPlugin', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (106, 'p', '888', '/autoCode/pubPlug', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (107, 'p', '888', '/autoCode/removePlugin', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (108, 'p', '888', '/autoCode/getPluginList', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (109, 'p', '888', '/autoCode/addFunc', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (110, 'p', '888', '/autoCode/mcp', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (111, 'p', '888', '/autoCode/mcpStatus', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (112, 'p', '888', '/autoCode/mcpStart', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (113, 'p', '888', '/autoCode/mcpStop', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (114, 'p', '888', '/autoCode/mcpRoutes', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (115, 'p', '888', '/autoCode/mcpTest', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (116, 'p', '888', '/autoCode/mcpList', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (117, 'p', '888', '/autoCode/saveAIWorkflowSession', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (118, 'p', '888', '/autoCode/getAIWorkflowSessionList', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (119, 'p', '888', '/autoCode/getAIWorkflowSessionDetail', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (120, 'p', '888', '/autoCode/deleteAIWorkflowSession', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (121, 'p', '888', '/autoCode/dumpAIWorkflowMarkdown', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (122, 'p', '888', '/sysDictionaryDetail/findSysDictionaryDetail', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (123, 'p', '888', '/sysDictionaryDetail/updateSysDictionaryDetail', 'PUT', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (124, 'p', '888', '/sysDictionaryDetail/createSysDictionaryDetail', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (125, 'p', '888', '/sysDictionaryDetail/getSysDictionaryDetailList', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (126, 'p', '888', '/sysDictionaryDetail/deleteSysDictionaryDetail', 'DELETE', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (127, 'p', '888', '/sysDictionaryDetail/getDictionaryTreeList', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (128, 'p', '888', '/sysDictionaryDetail/getDictionaryTreeListByType', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (129, 'p', '888', '/sysDictionaryDetail/getDictionaryDetailsByParent', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (130, 'p', '888', '/sysDictionaryDetail/getDictionaryPath', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (131, 'p', '888', '/sysDictionary/findSysDictionary', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (132, 'p', '888', '/sysDictionary/updateSysDictionary', 'PUT', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (133, 'p', '888', '/sysDictionary/getSysDictionaryList', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (134, 'p', '888', '/sysDictionary/createSysDictionary', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (135, 'p', '888', '/sysDictionary/deleteSysDictionary', 'DELETE', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (136, 'p', '888', '/sysDictionary/importSysDictionary', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (137, 'p', '888', '/sysDictionary/exportSysDictionary', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (138, 'p', '888', '/sysOperationRecord/findSysOperationRecord', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (140, 'p', '888', '/sysOperationRecord/createSysOperationRecord', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (141, 'p', '888', '/sysOperationRecord/getSysOperationRecordList', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (142, 'p', '888', '/sysOperationRecord/deleteSysOperationRecord', 'DELETE', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (143, 'p', '888', '/sysOperationRecord/deleteSysOperationRecordByIds', 'DELETE', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (144, 'p', '888', '/email/emailTest', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (145, 'p', '888', '/email/sendEmail', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (146, 'p', '888', '/simpleUploader/upload', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (147, 'p', '888', '/simpleUploader/checkFileMd5', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (148, 'p', '888', '/simpleUploader/mergeFileMd5', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (149, 'p', '888', '/authorityBtn/setAuthorityBtn', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (150, 'p', '888', '/authorityBtn/getAuthorityBtn', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (151, 'p', '888', '/authorityBtn/canRemoveAuthorityBtn', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (152, 'p', '888', '/sysExportTemplate/createSysExportTemplate', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (153, 'p', '888', '/sysExportTemplate/deleteSysExportTemplate', 'DELETE', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (154, 'p', '888', '/sysExportTemplate/deleteSysExportTemplateByIds', 'DELETE', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (155, 'p', '888', '/sysExportTemplate/updateSysExportTemplate', 'PUT', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (156, 'p', '888', '/sysExportTemplate/findSysExportTemplate', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (157, 'p', '888', '/sysExportTemplate/getSysExportTemplateList', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (158, 'p', '888', '/sysExportTemplate/exportExcel', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (159, 'p', '888', '/sysExportTemplate/exportTemplate', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (160, 'p', '888', '/sysExportTemplate/previewSQL', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (161, 'p', '888', '/sysExportTemplate/importExcel', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (162, 'p', '888', '/sysError/createSysError', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (163, 'p', '888', '/sysError/deleteSysError', 'DELETE', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (164, 'p', '888', '/sysError/deleteSysErrorByIds', 'DELETE', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (165, 'p', '888', '/sysError/updateSysError', 'PUT', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (166, 'p', '888', '/sysError/findSysError', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (167, 'p', '888', '/sysError/getSysErrorList', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (168, 'p', '888', '/sysError/getSysErrorSolution', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (169, 'p', '888', '/info/createInfo', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (170, 'p', '888', '/info/deleteInfo', 'DELETE', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (171, 'p', '888', '/info/deleteInfoByIds', 'DELETE', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (172, 'p', '888', '/info/updateInfo', 'PUT', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (173, 'p', '888', '/info/findInfo', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (174, 'p', '888', '/info/getInfoList', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (175, 'p', '888', '/sysParams/createSysParams', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (176, 'p', '888', '/sysParams/deleteSysParams', 'DELETE', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (177, 'p', '888', '/sysParams/deleteSysParamsByIds', 'DELETE', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (178, 'p', '888', '/sysParams/updateSysParams', 'PUT', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (179, 'p', '888', '/sysParams/findSysParams', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (180, 'p', '888', '/sysParams/getSysParamsList', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (181, 'p', '888', '/sysParams/getSysParam', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (182, 'p', '888', '/attachmentCategory/getCategoryList', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (183, 'p', '888', '/attachmentCategory/addCategory', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (184, 'p', '888', '/attachmentCategory/deleteCategory', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (185, 'p', '888', '/sysVersion/findSysVersion', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (186, 'p', '888', '/sysVersion/getSysVersionList', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (187, 'p', '888', '/sysVersion/downloadVersionJson', 'GET', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (188, 'p', '888', '/sysVersion/exportVersion', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (189, 'p', '888', '/sysVersion/importVersion', 'POST', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (190, 'p', '888', '/sysVersion/deleteSysVersion', 'DELETE', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (191, 'p', '888', '/sysVersion/deleteSysVersionByIds', 'DELETE', '', '', '');
INSERT INTO public.casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES (292, 'p', '888', '/skills/downloadOnlineSkill', 'POST', NULL, NULL, NULL);


--
-- Data for Name: exa_attachment_category; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: exa_customers; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: exa_file_chunks; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: exa_file_upload_and_downloads; Type: TABLE DATA; Schema: public; Owner: -
--

INSERT INTO public.exa_file_upload_and_downloads (id, created_at, updated_at, deleted_at, name, class_id, url, tag, key) VALUES (1, '2026-11-15 07:05:35.514523+00', '2026-11-15 07:05:35.514523+00', NULL, '10.png', 0, 'https://qmplusimg.henrongyi.top/gvalogo.png', 'png', '158787308910.png');
INSERT INTO public.exa_file_upload_and_downloads (id, created_at, updated_at, deleted_at, name, class_id, url, tag, key) VALUES (2, '2026-11-15 07:05:35.514523+00', '2026-11-15 07:05:35.514523+00', NULL, 'logo.png', 0, 'https://qmplusimg.henrongyi.top/1576554439myAvatar.png', 'png', '1587973709logo.png');


--
-- Data for Name: exa_files; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: gva_announcements_info; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: jwt_blacklists; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sys_ai_workflow_sessions; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sys_api_tokens; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sys_apis; Type: TABLE DATA; Schema: public; Owner: -
--

INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (1, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/jwt/jsonInBlacklist', 'jwt加入黑名单(退出，必选)', 'jwt', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (2, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysLoginLog/deleteLoginLog', '删除登录日志', '登录日志', 'DELETE');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (3, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysLoginLog/deleteLoginLogByIds', '批量删除登录日志', '登录日志', 'DELETE');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (4, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysLoginLog/findLoginLog', '根据ID获取登录日志', '登录日志', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (5, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysLoginLog/getLoginLogList', '获取登录日志列表', '登录日志', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (6, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysApiToken/createApiToken', '签发API Token', 'API Token', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (7, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysApiToken/getApiTokenList', '获取API Token列表', 'API Token', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (8, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysApiToken/deleteApiToken', '作废API Token', 'API Token', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (9, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/user/deleteUser', '删除用户', '系统用户', 'DELETE');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (10, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/user/admin_register', '用户注册', '系统用户', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (11, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/user/getUserList', '获取用户列表', '系统用户', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (12, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/user/setUserInfo', '设置用户信息', '系统用户', 'PUT');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (13, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/user/setSelfInfo', '设置自身信息(必选)', '系统用户', 'PUT');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (14, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/user/getUserInfo', '获取自身信息(必选)', '系统用户', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (15, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/user/setUserAuthorities', '设置权限组', '系统用户', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (16, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/user/changePassword', '修改密码（建议选择)', '系统用户', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (17, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/user/setUserAuthority', '修改用户角色(必选)', '系统用户', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (18, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/user/resetPassword', '重置用户密码', '系统用户', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (19, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/user/setSelfSetting', '用户界面配置', '系统用户', 'PUT');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (20, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/api/createApi', '创建api', 'api', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (21, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/api/deleteApi', '删除Api', 'api', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (22, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/api/updateApi', '更新Api', 'api', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (23, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/api/getApiList', '获取api列表', 'api', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (24, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/api/getAllApis', '获取所有api', 'api', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (25, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/api/getApiById', '获取api详细信息', 'api', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (26, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/api/deleteApisByIds', '批量删除api', 'api', 'DELETE');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (27, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/api/syncApi', '获取待同步API', 'api', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (28, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/api/getApiGroups', '获取路由组', 'api', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (29, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/api/enterSyncApi', '确认同步API', 'api', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (30, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/api/ignoreApi', '忽略API', 'api', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (31, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/api/getApiRoles', '获取指定API关联角色列表', 'api', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (32, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/api/setApiRoles', '全量覆盖API关联角色列表', 'api', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (33, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/authority/copyAuthority', '拷贝角色', '角色', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (34, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/authority/createAuthority', '创建角色', '角色', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (35, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/authority/deleteAuthority', '删除角色', '角色', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (36, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/authority/updateAuthority', '更新角色信息', '角色', 'PUT');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (37, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/authority/getAuthorityList', '获取角色列表', '角色', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (38, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/authority/setDataAuthority', '设置角色资源权限', '角色', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (39, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/authority/getUsersByAuthority', '获取角色关联用户ID列表', '角色', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (40, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/authority/setRoleUsers', '全量覆盖角色关联用户', '角色', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (41, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/casbin/updateCasbin', '更改角色api权限', 'casbin', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (42, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/casbin/getPolicyPathByAuthorityId', '获取权限列表', 'casbin', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (43, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/menu/addBaseMenu', '新增菜单', '菜单', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (44, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/menu/getMenu', '获取菜单树(必选)', '菜单', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (45, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/menu/deleteBaseMenu', '删除菜单', '菜单', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (46, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/menu/updateBaseMenu', '更新菜单', '菜单', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (47, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/menu/getBaseMenuById', '根据id获取菜单', '菜单', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (48, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/menu/getMenuList', '分页获取基础menu列表', '菜单', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (49, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/menu/getBaseMenuTree', '获取用户动态路由', '菜单', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (50, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/menu/getMenuAuthority', '获取指定角色menu', '菜单', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (51, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/menu/addMenuAuthority', '增加menu和角色关联关系', '菜单', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (52, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/menu/getMenuRoles', '获取菜单关联角色列表', '菜单', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (53, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/menu/setMenuRoles', '全量覆盖菜单关联角色列表', '菜单', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (54, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/fileUploadAndDownload/findFile', '寻找目标文件（秒传）', '分片上传', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (55, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/fileUploadAndDownload/breakpointContinue', '断点续传', '分片上传', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (56, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/fileUploadAndDownload/breakpointContinueFinish', '断点续传完成', '分片上传', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (57, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/fileUploadAndDownload/removeChunk', '上传完成移除文件', '分片上传', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (58, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/fileUploadAndDownload/upload', '文件上传（建议选择）', '文件上传与下载', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (59, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/fileUploadAndDownload/deleteFile', '删除文件', '文件上传与下载', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (60, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/fileUploadAndDownload/editFileName', '文件名或者备注编辑', '文件上传与下载', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (61, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/fileUploadAndDownload/getFileList', '获取上传文件列表', '文件上传与下载', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (62, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/fileUploadAndDownload/importURL', '导入URL', '文件上传与下载', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (63, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/system/getServerInfo', '获取服务器信息', '系统服务', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (64, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/system/getSystemConfig', '获取配置文件内容', '系统服务', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (65, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/system/setSystemConfig', '设置配置文件内容', '系统服务', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (66, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/skills/getTools', '获取技能工具列表', 'skills', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (67, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/skills/getSkillList', '获取技能列表', 'skills', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (68, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/skills/getSkillDetail', '获取技能详情', 'skills', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (69, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/skills/saveSkill', '保存技能定义', 'skills', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (70, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/skills/deleteSkill', '删除技能', 'skills', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (71, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/skills/createScript', '创建技能脚本', 'skills', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (72, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/skills/getScript', '读取技能脚本', 'skills', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (73, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/skills/saveScript', '保存技能脚本', 'skills', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (74, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/skills/createResource', '创建技能资源', 'skills', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (75, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/skills/getResource', '读取技能资源', 'skills', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (76, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/skills/saveResource', '保存技能资源', 'skills', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (77, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/skills/createReference', '创建技能参考', 'skills', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (78, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/skills/getReference', '读取技能参考', 'skills', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (79, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/skills/saveReference', '保存技能参考', 'skills', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (80, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/skills/createTemplate', '创建技能模板', 'skills', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (81, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/skills/getTemplate', '读取技能模板', 'skills', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (82, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/skills/saveTemplate', '保存技能模板', 'skills', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (83, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/skills/getGlobalConstraint', '读取全局约束', 'skills', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (84, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/skills/saveGlobalConstraint', '保存全局约束', 'skills', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (85, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/skills/packageSkill', '打包技能', 'skills', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (86, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/customer/customer', '更新客户', '客户', 'PUT');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (87, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/customer/customer', '创建客户', '客户', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (88, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/customer/customer', '删除客户', '客户', 'DELETE');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (89, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/customer/customer', '获取单一客户', '客户', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (90, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/customer/customerList', '获取客户列表', '客户', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (91, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/autoCode/getDB', '获取所有数据库', '代码生成器', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (92, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/autoCode/getTables', '获取数据库表', '代码生成器', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (93, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/autoCode/createTemp', '自动化代码', '代码生成器', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (94, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/autoCode/preview', '预览自动化代码', '代码生成器', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (95, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/autoCode/getColumn', '获取所选table的所有字段', '代码生成器', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (96, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/autoCode/installPlugin', '安装插件', '代码生成器', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (97, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/autoCode/pubPlug', '打包插件', '代码生成器', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (98, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/autoCode/removePlugin', '卸载插件', '代码生成器', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (99, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/autoCode/getPluginList', '获取已安装插件', '代码生成器', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (100, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/autoCode/mcp', '自动生成 MCP Tool 模板', '代码生成器', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (101, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/autoCode/mcpStatus', '获取 MCP 独立服务状态', '代码生成器', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (102, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/autoCode/mcpStart', '启动 MCP 独立服务', '代码生成器', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (103, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/autoCode/mcpStop', '停用 MCP 独立服务', '代码生成器', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (104, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/autoCode/mcpRoutes', '获取 MCP 路由列表', '代码生成器', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (105, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/autoCode/mcpTest', 'MCP Tool 管理', '代码生成器', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (106, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/autoCode/mcpList', '获取 MCP ToolList', '代码生成器', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (107, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/autoCode/saveAIWorkflowSession', '保存AI需求工作流会话', '代码生成器', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (108, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/autoCode/getAIWorkflowSessionList', '获取AI需求工作流会话列表', '代码生成器', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (109, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/autoCode/getAIWorkflowSessionDetail', '获取AI需求工作流会话详情', '代码生成器', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (110, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/autoCode/deleteAIWorkflowSession', '删除AI需求工作流会话', '代码生成器', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (111, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/autoCode/dumpAIWorkflowMarkdown', 'AI需求工作流Markdown落盘', '代码生成器', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (112, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/autoCode/createPackage', '配置模板', '模板配置', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (113, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/autoCode/getTemplates', '获取模板文件', '模板配置', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (114, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/autoCode/getPackage', '获取所有模板', '模板配置', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (115, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/autoCode/delPackage', '删除模板', '模板配置', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (116, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/autoCode/getMeta', '获取meta信息', '代码生成器历史', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (117, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/autoCode/rollback', '回滚自动生成代码', '代码生成器历史', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (118, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/autoCode/getSysHistory', '查询回滚记录', '代码生成器历史', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (119, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/autoCode/delSysHistory', '删除回滚记录', '代码生成器历史', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (120, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/autoCode/addFunc', '增加模板方法', '代码生成器历史', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (121, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysDictionaryDetail/updateSysDictionaryDetail', '更新字典内容', '系统字典详情', 'PUT');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (122, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysDictionaryDetail/createSysDictionaryDetail', '新增字典内容', '系统字典详情', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (123, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysDictionaryDetail/deleteSysDictionaryDetail', '删除字典内容', '系统字典详情', 'DELETE');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (124, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysDictionaryDetail/findSysDictionaryDetail', '根据ID获取字典内容', '系统字典详情', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (125, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysDictionaryDetail/getSysDictionaryDetailList', '获取字典内容列表', '系统字典详情', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (126, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysDictionaryDetail/getDictionaryTreeList', '获取字典数列表', '系统字典详情', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (127, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysDictionaryDetail/getDictionaryTreeListByType', '根据分类获取字典数列表', '系统字典详情', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (128, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysDictionaryDetail/getDictionaryDetailsByParent', '根据父级ID获取字典详情', '系统字典详情', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (129, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysDictionaryDetail/getDictionaryPath', '获取字典详情的完整路径', '系统字典详情', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (130, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysDictionary/createSysDictionary', '新增字典', '系统字典', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (131, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysDictionary/deleteSysDictionary', '删除字典', '系统字典', 'DELETE');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (132, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysDictionary/updateSysDictionary', '更新字典', '系统字典', 'PUT');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (133, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysDictionary/findSysDictionary', '根据ID获取字典（建议选择）', '系统字典', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (134, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysDictionary/getSysDictionaryList', '获取字典列表', '系统字典', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (135, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysDictionary/importSysDictionary', '导入字典JSON', '系统字典', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (136, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysDictionary/exportSysDictionary', '导出字典JSON', '系统字典', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (137, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysOperationRecord/createSysOperationRecord', '新增操作记录', '操作记录', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (138, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysOperationRecord/findSysOperationRecord', '根据ID获取操作记录', '操作记录', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (139, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysOperationRecord/getSysOperationRecordList', '获取操作记录列表', '操作记录', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (140, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysOperationRecord/deleteSysOperationRecord', '删除操作记录', '操作记录', 'DELETE');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (141, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysOperationRecord/deleteSysOperationRecordByIds', '批量删除操作历史', '操作记录', 'DELETE');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (142, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/simpleUploader/upload', '插件版分片上传', '断点续传(插件版)', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (143, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/simpleUploader/checkFileMd5', '文件完整度验证', '断点续传(插件版)', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (144, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/simpleUploader/mergeFileMd5', '上传完成合并文件', '断点续传(插件版)', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (145, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/email/emailTest', '发送测试邮件', 'email', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (146, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/email/sendEmail', '发送邮件', 'email', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (147, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/authorityBtn/setAuthorityBtn', '设置按钮权限', '按钮权限', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (148, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/authorityBtn/getAuthorityBtn', '获取已有按钮权限', '按钮权限', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (149, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/authorityBtn/canRemoveAuthorityBtn', '删除按钮', '按钮权限', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (150, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysExportTemplate/createSysExportTemplate', '新增导出模板', '导出模板', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (151, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysExportTemplate/deleteSysExportTemplate', '删除导出模板', '导出模板', 'DELETE');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (152, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysExportTemplate/deleteSysExportTemplateByIds', '批量删除导出模板', '导出模板', 'DELETE');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (153, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysExportTemplate/updateSysExportTemplate', '更新导出模板', '导出模板', 'PUT');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (154, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysExportTemplate/findSysExportTemplate', '根据ID获取导出模板', '导出模板', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (155, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysExportTemplate/getSysExportTemplateList', '获取导出模板列表', '导出模板', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (156, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysExportTemplate/exportExcel', '导出Excel', '导出模板', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (157, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysExportTemplate/exportTemplate', '下载模板', '导出模板', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (158, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysExportTemplate/previewSQL', '预览SQL', '导出模板', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (159, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysExportTemplate/importExcel', '导入Excel', '导出模板', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (160, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysError/createSysError', '新建错误日志', '错误日志', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (161, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysError/deleteSysError', '删除错误日志', '错误日志', 'DELETE');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (162, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysError/deleteSysErrorByIds', '批量删除错误日志', '错误日志', 'DELETE');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (163, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysError/updateSysError', '更新错误日志', '错误日志', 'PUT');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (164, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysError/findSysError', '根据ID获取错误日志', '错误日志', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (165, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysError/getSysErrorList', '获取错误日志列表', '错误日志', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (166, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysError/getSysErrorSolution', '触发错误处理(异步)', '错误日志', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (167, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/info/createInfo', '新建公告', '公告', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (168, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/info/deleteInfo', '删除公告', '公告', 'DELETE');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (169, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/info/deleteInfoByIds', '批量删除公告', '公告', 'DELETE');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (170, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/info/updateInfo', '更新公告', '公告', 'PUT');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (171, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/info/findInfo', '根据ID获取公告', '公告', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (172, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/info/getInfoList', '获取公告列表', '公告', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (173, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysParams/createSysParams', '新建参数', '参数管理', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (174, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysParams/deleteSysParams', '删除参数', '参数管理', 'DELETE');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (175, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysParams/deleteSysParamsByIds', '批量删除参数', '参数管理', 'DELETE');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (176, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysParams/updateSysParams', '更新参数', '参数管理', 'PUT');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (177, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysParams/findSysParams', '根据ID获取参数', '参数管理', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (178, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysParams/getSysParamsList', '获取参数列表', '参数管理', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (179, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysParams/getSysParam', '获取参数列表', '参数管理', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (180, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/attachmentCategory/getCategoryList', '分类列表', '媒体库分类', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (181, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/attachmentCategory/addCategory', '添加/编辑分类', '媒体库分类', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (182, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/attachmentCategory/deleteCategory', '删除分类', '媒体库分类', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (183, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysVersion/findSysVersion', '获取单一版本', '版本控制', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (184, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysVersion/getSysVersionList', '获取版本列表', '版本控制', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (185, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysVersion/downloadVersionJson', '下载版本json', '版本控制', 'GET');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (186, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysVersion/exportVersion', '创建版本', '版本控制', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (187, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysVersion/importVersion', '同步版本', '版本控制', 'POST');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (188, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysVersion/deleteSysVersion', '删除版本', '版本控制', 'DELETE');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (189, '2026-11-15 07:05:35.295708+00', '2026-11-15 07:05:35.295708+00', NULL, '/sysVersion/deleteSysVersionByIds', '批量删除版本', '版本控制', 'DELETE');
INSERT INTO public.sys_apis (id, created_at, updated_at, deleted_at, path, description, api_group, method) VALUES (190, '2026-11-15 07:05:35.560208+00', '2026-11-15 07:05:35.560208+00', NULL, '/skills/downloadOnlineSkill', '下载在线技能', 'skills', 'POST');


--
-- Data for Name: sys_authorities; Type: TABLE DATA; Schema: public; Owner: -
--

INSERT INTO public.sys_authorities (created_at, updated_at, deleted_at, authority_id, authority_name, parent_id, default_router) VALUES ('2026-11-15 07:05:35.315168+00', '2026-11-15 07:05:35.500978+00', NULL, 888, '超级管理员', 0, 'dashboard');


--
-- Data for Name: sys_authority_btns; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sys_authority_menus; Type: TABLE DATA; Schema: public; Owner: -
--

INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (1, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (3, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (4, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (5, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (6, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (8, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (9, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (10, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (11, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (12, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (13, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (14, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (15, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (16, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (17, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (18, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (19, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (20, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (21, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (22, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (23, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (24, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (25, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (26, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (27, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (28, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (29, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (30, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (31, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (32, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (33, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (34, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (35, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (36, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (37, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (38, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (39, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (40, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (41, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (42, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (43, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (44, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (45, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (46, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (47, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (48, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (49, 888);
INSERT INTO public.sys_authority_menus (sys_base_menu_id, sys_authority_authority_id) VALUES (50, 888);


--
-- Data for Name: sys_auto_code_histories; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sys_auto_code_packages; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sys_base_menu_btns; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sys_base_menu_parameters; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sys_base_menus; Type: TABLE DATA; Schema: public; Owner: -
--

INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (1, '2026-11-15 07:05:35.344174+00', '2026-11-15 07:05:35.344174+00', NULL, 0, 0, 'dashboard', 'dashboard', false, 'view/dashboard/index.vue', 1, '', false, false, '仪表盘', 'odometer', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (3, '2026-11-15 07:05:35.344174+00', '2026-11-15 07:05:35.344174+00', NULL, 0, 0, 'admin', 'superAdmin', false, 'view/superAdmin/index.vue', 3, '', false, false, '超级管理员', 'user', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (4, '2026-11-15 07:05:35.344174+00', '2026-11-15 07:05:35.344174+00', NULL, 0, 0, 'person', 'person', true, 'view/person/person.vue', 4, '', false, false, '个人信息', 'message', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (5, '2026-11-15 07:05:35.344174+00', '2026-11-15 07:05:35.344174+00', NULL, 0, 0, 'example', 'example', false, 'view/example/index.vue', 7, '', false, false, '示例文件', 'management', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (6, '2026-11-15 07:05:35.344174+00', '2026-11-15 07:05:35.344174+00', NULL, 0, 0, 'systemTools', 'systemTools', false, 'view/systemTools/index.vue', 5, '', false, false, '编程辅助', 'tools', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (8, '2026-11-15 07:05:35.344174+00', '2026-11-15 07:05:35.344174+00', NULL, 0, 0, 'state', 'state', false, 'view/system/state.vue', 8, '', false, false, '服务器状态', 'cloudy', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (9, '2026-11-15 07:05:35.344174+00', '2026-11-15 07:05:35.344174+00', NULL, 0, 0, 'plugin', 'plugin', false, 'view/routerHolder.vue', 6, '', false, false, '插件系统', 'cherry', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (10, '2026-11-15 07:05:35.345855+00', '2026-11-15 07:05:35.345855+00', NULL, 1, 3, 'authority', 'authority', false, 'view/superAdmin/authority/authority.vue', 1, '', false, false, '角色管理', 'avatar', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (11, '2026-11-15 07:05:35.345855+00', '2026-11-15 07:05:35.345855+00', NULL, 1, 3, 'menu', 'menu', false, 'view/superAdmin/menu/menu.vue', 2, '', true, false, '菜单管理', 'tickets', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (12, '2026-11-15 07:05:35.345855+00', '2026-11-15 07:05:35.345855+00', NULL, 1, 3, 'api', 'api', false, 'view/superAdmin/api/api.vue', 3, '', true, false, 'api管理', 'platform', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (13, '2026-11-15 07:05:35.345855+00', '2026-11-15 07:05:35.345855+00', NULL, 1, 3, 'user', 'user', false, 'view/superAdmin/user/user.vue', 4, '', false, false, '用户管理', 'coordinate', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (14, '2026-11-15 07:05:35.345855+00', '2026-11-15 07:05:35.345855+00', NULL, 1, 3, 'dictionary', 'dictionary', false, 'view/superAdmin/dictionary/sysDictionary.vue', 5, '', false, false, '字典管理', 'notebook', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (15, '2026-11-15 07:05:35.345855+00', '2026-11-15 07:05:35.345855+00', NULL, 1, 3, 'operation', 'operation', false, 'view/superAdmin/operation/sysOperationRecord.vue', 6, '', false, false, '操作历史', 'pie-chart', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (16, '2026-11-15 07:05:35.345855+00', '2026-11-15 07:05:35.345855+00', NULL, 1, 3, 'sysParams', 'sysParams', false, 'view/superAdmin/params/sysParams.vue', 7, '', false, false, '参数管理', 'compass', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (17, '2026-11-15 07:05:35.345855+00', '2026-11-15 07:05:35.345855+00', NULL, 1, 3, 'system', 'system', false, 'view/systemTools/system/system.vue', 8, '', false, false, '系统配置', 'operation', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (18, '2026-11-15 07:05:35.345855+00', '2026-11-15 07:05:35.345855+00', NULL, 1, 3, 'apiToken', 'apiToken', false, 'view/systemTools/apiToken/index.vue', 9, '', false, false, 'API Token', 'key', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (19, '2026-11-15 07:05:35.345855+00', '2026-11-15 07:05:35.345855+00', NULL, 1, 3, 'loginLog', 'loginLog', false, 'view/systemTools/loginLog/index.vue', 10, '', false, false, '登录日志', 'monitor', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (20, '2026-11-15 07:05:35.345855+00', '2026-11-15 07:05:35.345855+00', NULL, 1, 3, 'sysVersion', 'sysVersion', false, 'view/systemTools/version/version.vue', 11, '', false, false, '版本管理', 'server', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (21, '2026-11-15 07:05:35.345855+00', '2026-11-15 07:05:35.345855+00', NULL, 1, 3, 'sysError', 'sysError', false, 'view/systemTools/sysError/sysError.vue', 12, '', false, false, '错误日志', 'warn', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (22, '2026-11-15 07:05:35.345855+00', '2026-11-15 07:05:35.345855+00', NULL, 1, 5, 'upload', 'upload', false, 'view/example/upload/upload.vue', 5, '', false, false, '媒体库（上传下载）', 'upload', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (23, '2026-11-15 07:05:35.345855+00', '2026-11-15 07:05:35.345855+00', NULL, 1, 5, 'breakpoint', 'breakpoint', false, 'view/example/breakpoint/breakpoint.vue', 6, '', false, false, '断点续传', 'upload-filled', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (24, '2026-11-15 07:05:35.345855+00', '2026-11-15 07:05:35.345855+00', NULL, 1, 5, 'customer', 'customer', false, 'view/example/customer/customer.vue', 7, '', false, false, '客户列表（资源示例）', 'avatar', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (25, '2026-11-15 07:05:35.345855+00', '2026-11-15 07:05:35.345855+00', NULL, 1, 6, 'autoPkg', 'autoPkg', false, 'view/systemTools/autoPkg/autoPkg.vue', 0, '', false, false, '模板配置', 'folder', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (26, '2026-11-15 07:05:35.345855+00', '2026-11-15 07:05:35.345855+00', NULL, 1, 6, 'autoCode', 'autoCode', false, 'view/systemTools/autoCode/index.vue', 1, '', true, false, '代码生成器', 'cpu', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (27, '2026-11-15 07:05:35.345855+00', '2026-11-15 07:05:35.345855+00', NULL, 1, 6, 'autoCodeAdmin', 'autoCodeAdmin', false, 'view/systemTools/autoCodeAdmin/index.vue', 2, '', false, false, '自动化代码管理', 'magic-stick', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (28, '2026-11-15 07:05:35.345855+00', '2026-11-15 07:05:35.345855+00', NULL, 1, 6, 'formCreate', 'formCreate', false, 'view/systemTools/formCreate/index.vue', 3, '', true, false, '表单生成器', 'magic-stick', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (29, '2026-11-15 07:05:35.345855+00', '2026-11-15 07:05:35.345855+00', NULL, 1, 6, 'aiWorkflow', 'aiWorkflow', false, 'view/systemTools/aiWrokflow/index.vue', 4, '', true, false, 'AI需求工作流', 'magic-stick', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (30, '2026-11-15 07:05:35.345855+00', '2026-11-15 07:05:35.345855+00', NULL, 1, 6, 'autoCodeEdit/:id', 'autoCodeEdit', true, 'view/systemTools/autoCode/index.vue', 0, '', false, false, '自动化代码-${id}', 'magic-stick', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (31, '2026-11-15 07:05:35.345855+00', '2026-11-15 07:05:35.345855+00', NULL, 1, 6, 'exportTemplate', 'exportTemplate', false, 'view/systemTools/exportTemplate/exportTemplate.vue', 5, '', false, false, '导出模板', 'reading', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (32, '2026-11-15 07:05:35.345855+00', '2026-11-15 07:05:35.345855+00', NULL, 1, 6, 'mcpTest', 'mcpTest', false, 'view/systemTools/autoCode/mcpTest.vue', 6, '', false, false, 'Mcp Tools管理', 'partly-cloudy', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (33, '2026-11-15 07:05:35.345855+00', '2026-11-15 07:05:35.345855+00', NULL, 1, 6, 'mcpTool', 'mcpTool', false, 'view/systemTools/autoCode/mcp.vue', 7, '', false, false, 'Mcp Tools模板', 'magnet', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (34, '2026-11-15 07:05:35.345855+00', '2026-11-15 07:05:35.345855+00', NULL, 1, 6, 'skills', 'skills', false, 'view/systemTools/skills/index.vue', 8, '', false, false, 'Skills管理', 'document', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (35, '2026-11-15 07:05:35.345855+00', '2026-11-15 07:05:35.345855+00', NULL, 1, 6, 'picture', 'picture', false, 'view/systemTools/autoCode/picture.vue', 9, '', false, false, 'AI页面绘制', 'picture-filled', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (36, '2026-11-15 07:05:35.345855+00', '2026-11-15 07:05:35.345855+00', NULL, 1, 9, 'https://plugin.gin-vue-admin.com/', 'https://plugin.gin-vue-admin.com/', false, 'https://plugin.gin-vue-admin.com/', 0, '', false, false, '插件市场', 'shop', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (37, '2026-11-15 07:05:35.345855+00', '2026-11-15 07:05:35.345855+00', NULL, 1, 9, 'installPlugin', 'installPlugin', false, 'view/systemTools/installPlugin/index.vue', 1, '', false, false, '插件安装', 'box', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (38, '2026-11-15 07:05:35.345855+00', '2026-11-15 07:05:35.345855+00', NULL, 1, 9, 'pubPlug', 'pubPlug', false, 'view/systemTools/pubPlug/pubPlug.vue', 3, '', false, false, '打包插件', 'files', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (39, '2026-11-15 07:05:35.345855+00', '2026-11-15 07:05:35.345855+00', NULL, 1, 9, 'plugin-email', 'plugin-email', false, 'plugin/email/view/index.vue', 4, '', false, false, '邮件插件', 'message', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (40, '2026-11-15 07:05:35.345855+00', '2026-11-15 07:05:35.345855+00', NULL, 1, 9, 'anInfo', 'anInfo', false, 'plugin/announcement/view/info.vue', 5, '', false, false, '公告管理[示例]', 'scaleToOriginal', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (41, '2026-11-15 07:05:35.562288+00', '2026-11-15 07:05:35.562288+00', NULL, 0, 0, 'programmingAssistant', 'AutoRoot', false, 'view/routerHolder.vue', 91, '', false, false, '编程辅助', 'cpu', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (42, '2026-11-15 07:05:35.563451+00', '2026-11-15 07:05:35.563451+00', NULL, 0, 41, 'autoCode', 'AutoCode', false, 'plugin/auto/view/autoCode/index.vue', 1, '', false, false, '自动代码', 'magic-stick', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (43, '2026-11-15 07:05:35.564272+00', '2026-11-15 07:05:35.564272+00', NULL, 0, 41, 'autoPkg', 'AutoPkg', false, 'plugin/auto/view/autoPkg/autoPkg.vue', 2, '', false, false, '自动化包', 'files', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (44, '2026-11-15 07:05:35.565235+00', '2026-11-15 07:05:35.565235+00', NULL, 0, 41, 'aiWorkflow', 'AIWorkflow', false, 'plugin/auto/view/aiWorkflow/index.vue', 4, '', false, false, 'AI 工作流', 'memo', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (45, '2026-11-15 07:05:35.566461+00', '2026-11-15 07:05:35.566461+00', NULL, 0, 41, 'mcp', 'MCP', false, 'plugin/auto/view/autoCode/mcp.vue', 6, '', false, false, 'MCP 工具', 'monitor', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (46, '2026-11-15 07:05:35.567063+00', '2026-11-15 07:05:35.567063+00', NULL, 0, 41, 'mcpTest', 'MCPTest', false, 'plugin/auto/view/autoCode/mcpTest.vue', 5, '', false, false, 'MCP 测试', 'connection', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (47, '2026-11-15 07:05:35.567649+00', '2026-11-15 07:05:35.567649+00', NULL, 0, 41, 'installPlugin', 'AutoInstallPlugin', false, 'plugin/auto/view/installPlugin/index.vue', 6, '', false, false, '插件安装', 'upload-filled', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (48, '2026-11-15 07:05:35.568388+00', '2026-11-15 07:05:35.568388+00', NULL, 0, 41, 'pubPlug', 'PubPlug', false, 'plugin/auto/view/pubPlug/pubPlug.vue', 7, '', false, false, '插件打包', 'box', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (49, '2026-11-15 07:05:35.569021+00', '2026-11-15 07:05:35.569021+00', NULL, 0, 41, 'skills', 'Skills', false, 'plugin/auto/view/skills/index.vue', 8, '', false, false, '技能管理', 'edit-pen', false, '');
INSERT INTO public.sys_base_menus (id, created_at, updated_at, deleted_at, menu_level, parent_id, path, name, hidden, component, sort, active_name, keep_alive, default_menu, title, icon, close_tab, transition_type) VALUES (50, '2026-11-15 07:05:35.569907+00', '2026-11-15 07:05:35.569907+00', NULL, 0, 41, 'autoCodeAdmin', 'AutoCodeAdmin', true, 'plugin/auto/view/autoCodeAdmin/index.vue', 9, '', false, false, '自动代码管理', 'setting', false, '');


--
-- Data for Name: sys_data_authority_id; Type: TABLE DATA; Schema: public; Owner: -
--

INSERT INTO public.sys_data_authority_id (sys_authority_authority_id, data_authority_id_authority_id) VALUES (888, 888);


--
-- Data for Name: sys_dictionaries; Type: TABLE DATA; Schema: public; Owner: -
--

INSERT INTO public.sys_dictionaries (id, created_at, updated_at, deleted_at, name, type, status, "desc", parent_id) VALUES (1, '2026-11-15 07:05:35.324996+00', '2026-11-15 07:05:35.328752+00', NULL, '性别', 'gender', true, '性别字典', NULL);
INSERT INTO public.sys_dictionaries (id, created_at, updated_at, deleted_at, name, type, status, "desc", parent_id) VALUES (2, '2026-11-15 07:05:35.324996+00', '2026-11-15 07:05:35.331749+00', NULL, '数据库int类型', 'int', true, 'int类型对应的数据库类型', NULL);
INSERT INTO public.sys_dictionaries (id, created_at, updated_at, deleted_at, name, type, status, "desc", parent_id) VALUES (3, '2026-11-15 07:05:35.324996+00', '2026-11-15 07:05:35.334368+00', NULL, '数据库时间日期类型', 'time.Time', true, '数据库时间日期类型', NULL);
INSERT INTO public.sys_dictionaries (id, created_at, updated_at, deleted_at, name, type, status, "desc", parent_id) VALUES (4, '2026-11-15 07:05:35.324996+00', '2026-11-15 07:05:35.336717+00', NULL, '数据库浮点型', 'float64', true, '数据库浮点型', NULL);
INSERT INTO public.sys_dictionaries (id, created_at, updated_at, deleted_at, name, type, status, "desc", parent_id) VALUES (5, '2026-11-15 07:05:35.324996+00', '2026-11-15 07:05:35.339188+00', NULL, '数据库字符串', 'string', true, '数据库字符串', NULL);
INSERT INTO public.sys_dictionaries (id, created_at, updated_at, deleted_at, name, type, status, "desc", parent_id) VALUES (6, '2026-11-15 07:05:35.324996+00', '2026-11-15 07:05:35.341621+00', NULL, '数据库bool类型', 'bool', true, '数据库bool类型', NULL);


--
-- Data for Name: sys_dictionary_details; Type: TABLE DATA; Schema: public; Owner: -
--

INSERT INTO public.sys_dictionary_details (id, created_at, updated_at, deleted_at, label, value, extend, status, sort, sys_dictionary_id, parent_id, level, path) VALUES (1, '2026-11-15 07:05:35.329369+00', '2026-11-15 07:05:35.329369+00', NULL, '男', '1', '', true, 1, 1, NULL, 0, '');
INSERT INTO public.sys_dictionary_details (id, created_at, updated_at, deleted_at, label, value, extend, status, sort, sys_dictionary_id, parent_id, level, path) VALUES (2, '2026-11-15 07:05:35.329369+00', '2026-11-15 07:05:35.329369+00', NULL, '女', '2', '', true, 2, 1, NULL, 0, '');
INSERT INTO public.sys_dictionary_details (id, created_at, updated_at, deleted_at, label, value, extend, status, sort, sys_dictionary_id, parent_id, level, path) VALUES (3, '2026-11-15 07:05:35.332042+00', '2026-11-15 07:05:35.332042+00', NULL, 'smallint', '1', 'mysql', true, 1, 2, NULL, 0, '');
INSERT INTO public.sys_dictionary_details (id, created_at, updated_at, deleted_at, label, value, extend, status, sort, sys_dictionary_id, parent_id, level, path) VALUES (4, '2026-11-15 07:05:35.332042+00', '2026-11-15 07:05:35.332042+00', NULL, 'mediumint', '2', 'mysql', true, 2, 2, NULL, 0, '');
INSERT INTO public.sys_dictionary_details (id, created_at, updated_at, deleted_at, label, value, extend, status, sort, sys_dictionary_id, parent_id, level, path) VALUES (5, '2026-11-15 07:05:35.332042+00', '2026-11-15 07:05:35.332042+00', NULL, 'int', '3', 'mysql', true, 3, 2, NULL, 0, '');
INSERT INTO public.sys_dictionary_details (id, created_at, updated_at, deleted_at, label, value, extend, status, sort, sys_dictionary_id, parent_id, level, path) VALUES (6, '2026-11-15 07:05:35.332042+00', '2026-11-15 07:05:35.332042+00', NULL, 'bigint', '4', 'mysql', true, 4, 2, NULL, 0, '');
INSERT INTO public.sys_dictionary_details (id, created_at, updated_at, deleted_at, label, value, extend, status, sort, sys_dictionary_id, parent_id, level, path) VALUES (7, '2026-11-15 07:05:35.332042+00', '2026-11-15 07:05:35.332042+00', NULL, 'int2', '5', 'pgsql', true, 5, 2, NULL, 0, '');
INSERT INTO public.sys_dictionary_details (id, created_at, updated_at, deleted_at, label, value, extend, status, sort, sys_dictionary_id, parent_id, level, path) VALUES (8, '2026-11-15 07:05:35.332042+00', '2026-11-15 07:05:35.332042+00', NULL, 'int4', '6', 'pgsql', true, 6, 2, NULL, 0, '');
INSERT INTO public.sys_dictionary_details (id, created_at, updated_at, deleted_at, label, value, extend, status, sort, sys_dictionary_id, parent_id, level, path) VALUES (9, '2026-11-15 07:05:35.332042+00', '2026-11-15 07:05:35.332042+00', NULL, 'int6', '7', 'pgsql', true, 7, 2, NULL, 0, '');
INSERT INTO public.sys_dictionary_details (id, created_at, updated_at, deleted_at, label, value, extend, status, sort, sys_dictionary_id, parent_id, level, path) VALUES (10, '2026-11-15 07:05:35.332042+00', '2026-11-15 07:05:35.332042+00', NULL, 'int8', '8', 'pgsql', true, 8, 2, NULL, 0, '');
INSERT INTO public.sys_dictionary_details (id, created_at, updated_at, deleted_at, label, value, extend, status, sort, sys_dictionary_id, parent_id, level, path) VALUES (11, '2026-11-15 07:05:35.334627+00', '2026-11-15 07:05:35.334627+00', NULL, 'date', '0', 'mysql', true, 0, 3, NULL, 0, '');
INSERT INTO public.sys_dictionary_details (id, created_at, updated_at, deleted_at, label, value, extend, status, sort, sys_dictionary_id, parent_id, level, path) VALUES (12, '2026-11-15 07:05:35.334627+00', '2026-11-15 07:05:35.334627+00', NULL, 'time', '1', 'mysql', true, 1, 3, NULL, 0, '');
INSERT INTO public.sys_dictionary_details (id, created_at, updated_at, deleted_at, label, value, extend, status, sort, sys_dictionary_id, parent_id, level, path) VALUES (13, '2026-11-15 07:05:35.334627+00', '2026-11-15 07:05:35.334627+00', NULL, 'year', '2', 'mysql', true, 2, 3, NULL, 0, '');
INSERT INTO public.sys_dictionary_details (id, created_at, updated_at, deleted_at, label, value, extend, status, sort, sys_dictionary_id, parent_id, level, path) VALUES (14, '2026-11-15 07:05:35.334627+00', '2026-11-15 07:05:35.334627+00', NULL, 'datetime', '3', 'mysql', true, 3, 3, NULL, 0, '');
INSERT INTO public.sys_dictionary_details (id, created_at, updated_at, deleted_at, label, value, extend, status, sort, sys_dictionary_id, parent_id, level, path) VALUES (15, '2026-11-15 07:05:35.334627+00', '2026-11-15 07:05:35.334627+00', NULL, 'timestamp', '5', 'mysql', true, 5, 3, NULL, 0, '');
INSERT INTO public.sys_dictionary_details (id, created_at, updated_at, deleted_at, label, value, extend, status, sort, sys_dictionary_id, parent_id, level, path) VALUES (16, '2026-11-15 07:05:35.334627+00', '2026-11-15 07:05:35.334627+00', NULL, 'timestamptz', '6', 'pgsql', true, 5, 3, NULL, 0, '');
INSERT INTO public.sys_dictionary_details (id, created_at, updated_at, deleted_at, label, value, extend, status, sort, sys_dictionary_id, parent_id, level, path) VALUES (17, '2026-11-15 07:05:35.336981+00', '2026-11-15 07:05:35.336981+00', NULL, 'float', '0', 'mysql', true, 0, 4, NULL, 0, '');
INSERT INTO public.sys_dictionary_details (id, created_at, updated_at, deleted_at, label, value, extend, status, sort, sys_dictionary_id, parent_id, level, path) VALUES (18, '2026-11-15 07:05:35.336981+00', '2026-11-15 07:05:35.336981+00', NULL, 'double', '1', 'mysql', true, 1, 4, NULL, 0, '');
INSERT INTO public.sys_dictionary_details (id, created_at, updated_at, deleted_at, label, value, extend, status, sort, sys_dictionary_id, parent_id, level, path) VALUES (19, '2026-11-15 07:05:35.336981+00', '2026-11-15 07:05:35.336981+00', NULL, 'decimal', '2', 'mysql', true, 2, 4, NULL, 0, '');
INSERT INTO public.sys_dictionary_details (id, created_at, updated_at, deleted_at, label, value, extend, status, sort, sys_dictionary_id, parent_id, level, path) VALUES (20, '2026-11-15 07:05:35.336981+00', '2026-11-15 07:05:35.336981+00', NULL, 'numeric', '3', 'pgsql', true, 3, 4, NULL, 0, '');
INSERT INTO public.sys_dictionary_details (id, created_at, updated_at, deleted_at, label, value, extend, status, sort, sys_dictionary_id, parent_id, level, path) VALUES (21, '2026-11-15 07:05:35.336981+00', '2026-11-15 07:05:35.336981+00', NULL, 'smallserial', '4', 'pgsql', true, 4, 4, NULL, 0, '');
INSERT INTO public.sys_dictionary_details (id, created_at, updated_at, deleted_at, label, value, extend, status, sort, sys_dictionary_id, parent_id, level, path) VALUES (22, '2026-11-15 07:05:35.339457+00', '2026-11-15 07:05:35.339457+00', NULL, 'char', '0', 'mysql', true, 0, 5, NULL, 0, '');
INSERT INTO public.sys_dictionary_details (id, created_at, updated_at, deleted_at, label, value, extend, status, sort, sys_dictionary_id, parent_id, level, path) VALUES (23, '2026-11-15 07:05:35.339457+00', '2026-11-15 07:05:35.339457+00', NULL, 'varchar', '1', 'mysql', true, 1, 5, NULL, 0, '');
INSERT INTO public.sys_dictionary_details (id, created_at, updated_at, deleted_at, label, value, extend, status, sort, sys_dictionary_id, parent_id, level, path) VALUES (24, '2026-11-15 07:05:35.339457+00', '2026-11-15 07:05:35.339457+00', NULL, 'tinyblob', '2', 'mysql', true, 2, 5, NULL, 0, '');
INSERT INTO public.sys_dictionary_details (id, created_at, updated_at, deleted_at, label, value, extend, status, sort, sys_dictionary_id, parent_id, level, path) VALUES (25, '2026-11-15 07:05:35.339457+00', '2026-11-15 07:05:35.339457+00', NULL, 'tinytext', '3', 'mysql', true, 3, 5, NULL, 0, '');
INSERT INTO public.sys_dictionary_details (id, created_at, updated_at, deleted_at, label, value, extend, status, sort, sys_dictionary_id, parent_id, level, path) VALUES (26, '2026-11-15 07:05:35.339457+00', '2026-11-15 07:05:35.339457+00', NULL, 'text', '4', 'mysql', true, 4, 5, NULL, 0, '');
INSERT INTO public.sys_dictionary_details (id, created_at, updated_at, deleted_at, label, value, extend, status, sort, sys_dictionary_id, parent_id, level, path) VALUES (27, '2026-11-15 07:05:35.339457+00', '2026-11-15 07:05:35.339457+00', NULL, 'blob', '5', 'mysql', true, 5, 5, NULL, 0, '');
INSERT INTO public.sys_dictionary_details (id, created_at, updated_at, deleted_at, label, value, extend, status, sort, sys_dictionary_id, parent_id, level, path) VALUES (28, '2026-11-15 07:05:35.339457+00', '2026-11-15 07:05:35.339457+00', NULL, 'mediumblob', '6', 'mysql', true, 6, 5, NULL, 0, '');
INSERT INTO public.sys_dictionary_details (id, created_at, updated_at, deleted_at, label, value, extend, status, sort, sys_dictionary_id, parent_id, level, path) VALUES (29, '2026-11-15 07:05:35.339457+00', '2026-11-15 07:05:35.339457+00', NULL, 'mediumtext', '7', 'mysql', true, 7, 5, NULL, 0, '');
INSERT INTO public.sys_dictionary_details (id, created_at, updated_at, deleted_at, label, value, extend, status, sort, sys_dictionary_id, parent_id, level, path) VALUES (30, '2026-11-15 07:05:35.339457+00', '2026-11-15 07:05:35.339457+00', NULL, 'longblob', '8', 'mysql', true, 8, 5, NULL, 0, '');
INSERT INTO public.sys_dictionary_details (id, created_at, updated_at, deleted_at, label, value, extend, status, sort, sys_dictionary_id, parent_id, level, path) VALUES (31, '2026-11-15 07:05:35.339457+00', '2026-11-15 07:05:35.339457+00', NULL, 'longtext', '9', 'mysql', true, 9, 5, NULL, 0, '');
INSERT INTO public.sys_dictionary_details (id, created_at, updated_at, deleted_at, label, value, extend, status, sort, sys_dictionary_id, parent_id, level, path) VALUES (32, '2026-11-15 07:05:35.34187+00', '2026-11-15 07:05:35.34187+00', NULL, 'tinyint', '1', 'mysql', true, 0, 6, NULL, 0, '');
INSERT INTO public.sys_dictionary_details (id, created_at, updated_at, deleted_at, label, value, extend, status, sort, sys_dictionary_id, parent_id, level, path) VALUES (33, '2026-11-15 07:05:35.34187+00', '2026-11-15 07:05:35.34187+00', NULL, 'bool', '2', 'pgsql', true, 0, 6, NULL, 0, '');


--
-- Data for Name: sys_error; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sys_export_template_condition; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sys_export_template_join; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sys_export_templates; Type: TABLE DATA; Schema: public; Owner: -
--

INSERT INTO public.sys_export_templates (id, created_at, updated_at, deleted_at, db_name, name, table_name, template_id, template_info, sql, import_sql, "limit", "order") VALUES (1, '2026-11-15 07:05:35.498306+00', '2026-11-15 07:05:35.498306+00', NULL, '', 'api', 'sys_apis', 'api', '{
"path":"路径",
"method":"方法（大写）",
"description":"方法介绍",
"api_group":"方法分组"
}', '', '', NULL, '');


--
-- Data for Name: sys_ignore_apis; Type: TABLE DATA; Schema: public; Owner: -
--

INSERT INTO public.sys_ignore_apis (id, created_at, updated_at, deleted_at, path, method) VALUES (1, '2026-11-15 07:05:35.303217+00', '2026-11-15 07:05:35.303217+00', NULL, '/swagger/*any', 'GET');
INSERT INTO public.sys_ignore_apis (id, created_at, updated_at, deleted_at, path, method) VALUES (3, '2026-11-15 07:05:35.303217+00', '2026-11-15 07:05:35.303217+00', NULL, '/uploads/file/*filepath', 'GET');
INSERT INTO public.sys_ignore_apis (id, created_at, updated_at, deleted_at, path, method) VALUES (4, '2026-11-15 07:05:35.303217+00', '2026-11-15 07:05:35.303217+00', NULL, '/health', 'GET');
INSERT INTO public.sys_ignore_apis (id, created_at, updated_at, deleted_at, path, method) VALUES (5, '2026-11-15 07:05:35.303217+00', '2026-11-15 07:05:35.303217+00', NULL, '/uploads/file/*filepath', 'HEAD');
INSERT INTO public.sys_ignore_apis (id, created_at, updated_at, deleted_at, path, method) VALUES (9, '2026-11-15 07:05:35.303217+00', '2026-11-15 07:05:35.303217+00', NULL, '/base/login', 'POST');
INSERT INTO public.sys_ignore_apis (id, created_at, updated_at, deleted_at, path, method) VALUES (10, '2026-11-15 07:05:35.303217+00', '2026-11-15 07:05:35.303217+00', NULL, '/base/captcha', 'POST');
INSERT INTO public.sys_ignore_apis (id, created_at, updated_at, deleted_at, path, method) VALUES (11, '2026-11-15 07:05:35.303217+00', '2026-11-15 07:05:35.303217+00', NULL, '/info/getInfoDataSource', 'GET');
INSERT INTO public.sys_ignore_apis (id, created_at, updated_at, deleted_at, path, method) VALUES (12, '2026-11-15 07:05:35.303217+00', '2026-11-15 07:05:35.303217+00', NULL, '/info/getInfoPublic', 'GET');


--
-- Data for Name: sys_login_logs; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sys_operation_records; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sys_params; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sys_user_authority; Type: TABLE DATA; Schema: public; Owner: -
--

INSERT INTO public.sys_user_authority (sys_user_id, sys_authority_authority_id) VALUES (1, 888);


--
-- Data for Name: sys_users; Type: TABLE DATA; Schema: public; Owner: -
--

INSERT INTO public.sys_users (id, created_at, updated_at, deleted_at, uuid, username, password, nick_name, header_img, authority_id, phone, email, enable, origin_setting) VALUES (1, '2026-11-15 07:05:35.489545+00', '2026-11-15 07:05:35.491436+00', NULL, '6274aa51-f638-450c-b7b0-58fd55fe4831', 'admin', '$2a$10$L1mzr308MPMOQnxBF0rNleOT6c38hnq/RbLkbMRDeHD580tY.//GC', 'Mr.奇淼', 'https://qmplusimg.henrongyi.top/gva_header.jpg', 888, '17611111111', '333333333@qq.com', 1, NULL);


--
-- Data for Name: sys_versions; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Name: casbin_rule_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.casbin_rule_id_seq', 292, true);


--
-- Name: exa_attachment_category_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.exa_attachment_category_id_seq', 1, false);


--
-- Name: exa_customers_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.exa_customers_id_seq', 1, false);


--
-- Name: exa_file_chunks_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.exa_file_chunks_id_seq', 1, false);


--
-- Name: exa_file_upload_and_downloads_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.exa_file_upload_and_downloads_id_seq', 2, true);


--
-- Name: exa_files_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.exa_files_id_seq', 1, false);


--
-- Name: gva_announcements_info_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.gva_announcements_info_id_seq', 1, false);


--
-- Name: jwt_blacklists_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.jwt_blacklists_id_seq', 1, false);


--
-- Name: sys_ai_workflow_sessions_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.sys_ai_workflow_sessions_id_seq', 1, false);


--
-- Name: sys_api_tokens_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.sys_api_tokens_id_seq', 1, false);


--
-- Name: sys_apis_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.sys_apis_id_seq', 190, true);


--
-- Name: sys_authorities_authority_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.sys_authorities_authority_id_seq', 888, true);


--
-- Name: sys_auto_code_histories_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.sys_auto_code_histories_id_seq', 1, false);


--
-- Name: sys_auto_code_packages_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.sys_auto_code_packages_id_seq', 1, false);


--
-- Name: sys_base_menu_btns_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.sys_base_menu_btns_id_seq', 1, false);


--
-- Name: sys_base_menu_parameters_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.sys_base_menu_parameters_id_seq', 1, false);


--
-- Name: sys_base_menus_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.sys_base_menus_id_seq', 50, true);


--
-- Name: sys_dictionaries_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.sys_dictionaries_id_seq', 6, true);


--
-- Name: sys_dictionary_details_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.sys_dictionary_details_id_seq', 33, true);


--
-- Name: sys_error_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.sys_error_id_seq', 1, false);


--
-- Name: sys_export_template_condition_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.sys_export_template_condition_id_seq', 1, false);


--
-- Name: sys_export_template_join_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.sys_export_template_join_id_seq', 1, false);


--
-- Name: sys_export_templates_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.sys_export_templates_id_seq', 1, true);


--
-- Name: sys_ignore_apis_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.sys_ignore_apis_id_seq', 12, true);


--
-- Name: sys_login_logs_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.sys_login_logs_id_seq', 1, false);


--
-- Name: sys_operation_records_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.sys_operation_records_id_seq', 1, false);


--
-- Name: sys_params_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.sys_params_id_seq', 1, false);


--
-- Name: sys_users_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.sys_users_id_seq', 1, true);


--
-- Name: sys_versions_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.sys_versions_id_seq', 1, false);


--
-- Name: casbin_rule casbin_rule_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.casbin_rule
    ADD CONSTRAINT casbin_rule_pkey PRIMARY KEY (id);


--
-- Name: exa_attachment_category exa_attachment_category_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.exa_attachment_category
    ADD CONSTRAINT exa_attachment_category_pkey PRIMARY KEY (id);


--
-- Name: exa_customers exa_customers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.exa_customers
    ADD CONSTRAINT exa_customers_pkey PRIMARY KEY (id);


--
-- Name: exa_file_chunks exa_file_chunks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.exa_file_chunks
    ADD CONSTRAINT exa_file_chunks_pkey PRIMARY KEY (id);


--
-- Name: exa_file_upload_and_downloads exa_file_upload_and_downloads_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.exa_file_upload_and_downloads
    ADD CONSTRAINT exa_file_upload_and_downloads_pkey PRIMARY KEY (id);


--
-- Name: exa_files exa_files_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.exa_files
    ADD CONSTRAINT exa_files_pkey PRIMARY KEY (id);


--
-- Name: gva_announcements_info gva_announcements_info_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gva_announcements_info
    ADD CONSTRAINT gva_announcements_info_pkey PRIMARY KEY (id);


--
-- Name: jwt_blacklists jwt_blacklists_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.jwt_blacklists
    ADD CONSTRAINT jwt_blacklists_pkey PRIMARY KEY (id);


--
-- Name: sys_ai_workflow_sessions sys_ai_workflow_sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sys_ai_workflow_sessions
    ADD CONSTRAINT sys_ai_workflow_sessions_pkey PRIMARY KEY (id);


--
-- Name: sys_api_tokens sys_api_tokens_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sys_api_tokens
    ADD CONSTRAINT sys_api_tokens_pkey PRIMARY KEY (id);


--
-- Name: sys_apis sys_apis_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sys_apis
    ADD CONSTRAINT sys_apis_pkey PRIMARY KEY (id);


--
-- Name: sys_authority_menus sys_authority_menus_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sys_authority_menus
    ADD CONSTRAINT sys_authority_menus_pkey PRIMARY KEY (sys_base_menu_id, sys_authority_authority_id);


--
-- Name: sys_auto_code_histories sys_auto_code_histories_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sys_auto_code_histories
    ADD CONSTRAINT sys_auto_code_histories_pkey PRIMARY KEY (id);


--
-- Name: sys_auto_code_packages sys_auto_code_packages_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sys_auto_code_packages
    ADD CONSTRAINT sys_auto_code_packages_pkey PRIMARY KEY (id);


--
-- Name: sys_base_menu_btns sys_base_menu_btns_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sys_base_menu_btns
    ADD CONSTRAINT sys_base_menu_btns_pkey PRIMARY KEY (id);


--
-- Name: sys_base_menu_parameters sys_base_menu_parameters_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sys_base_menu_parameters
    ADD CONSTRAINT sys_base_menu_parameters_pkey PRIMARY KEY (id);


--
-- Name: sys_base_menus sys_base_menus_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sys_base_menus
    ADD CONSTRAINT sys_base_menus_pkey PRIMARY KEY (id);


--
-- Name: sys_data_authority_id sys_data_authority_id_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sys_data_authority_id
    ADD CONSTRAINT sys_data_authority_id_pkey PRIMARY KEY (sys_authority_authority_id, data_authority_id_authority_id);


--
-- Name: sys_dictionaries sys_dictionaries_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sys_dictionaries
    ADD CONSTRAINT sys_dictionaries_pkey PRIMARY KEY (id);


--
-- Name: sys_dictionary_details sys_dictionary_details_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sys_dictionary_details
    ADD CONSTRAINT sys_dictionary_details_pkey PRIMARY KEY (id);


--
-- Name: sys_error sys_error_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sys_error
    ADD CONSTRAINT sys_error_pkey PRIMARY KEY (id);


--
-- Name: sys_export_template_condition sys_export_template_condition_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sys_export_template_condition
    ADD CONSTRAINT sys_export_template_condition_pkey PRIMARY KEY (id);


--
-- Name: sys_export_template_join sys_export_template_join_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sys_export_template_join
    ADD CONSTRAINT sys_export_template_join_pkey PRIMARY KEY (id);


--
-- Name: sys_export_templates sys_export_templates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sys_export_templates
    ADD CONSTRAINT sys_export_templates_pkey PRIMARY KEY (id);


--
-- Name: sys_ignore_apis sys_ignore_apis_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sys_ignore_apis
    ADD CONSTRAINT sys_ignore_apis_pkey PRIMARY KEY (id);


--
-- Name: sys_login_logs sys_login_logs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sys_login_logs
    ADD CONSTRAINT sys_login_logs_pkey PRIMARY KEY (id);


--
-- Name: sys_operation_records sys_operation_records_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sys_operation_records
    ADD CONSTRAINT sys_operation_records_pkey PRIMARY KEY (id);


--
-- Name: sys_params sys_params_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sys_params
    ADD CONSTRAINT sys_params_pkey PRIMARY KEY (id);


--
-- Name: sys_user_authority sys_user_authority_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sys_user_authority
    ADD CONSTRAINT sys_user_authority_pkey PRIMARY KEY (sys_user_id, sys_authority_authority_id);


--
-- Name: sys_users sys_users_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sys_users
    ADD CONSTRAINT sys_users_pkey PRIMARY KEY (id);


--
-- Name: sys_versions sys_versions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sys_versions
    ADD CONSTRAINT sys_versions_pkey PRIMARY KEY (id);


--
-- Name: sys_authorities uni_sys_authorities_authority_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sys_authorities
    ADD CONSTRAINT uni_sys_authorities_authority_id PRIMARY KEY (authority_id);


--
-- Name: idx_exa_attachment_category_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_exa_attachment_category_deleted_at ON public.exa_attachment_category USING btree (deleted_at);


--
-- Name: idx_exa_customers_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_exa_customers_deleted_at ON public.exa_customers USING btree (deleted_at);


--
-- Name: idx_exa_file_chunks_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_exa_file_chunks_deleted_at ON public.exa_file_chunks USING btree (deleted_at);


--
-- Name: idx_exa_file_upload_and_downloads_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_exa_file_upload_and_downloads_deleted_at ON public.exa_file_upload_and_downloads USING btree (deleted_at);


--
-- Name: idx_exa_files_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_exa_files_deleted_at ON public.exa_files USING btree (deleted_at);


--
-- Name: idx_gva_announcements_info_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_gva_announcements_info_deleted_at ON public.gva_announcements_info USING btree (deleted_at);


--
-- Name: idx_jwt_blacklists_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_jwt_blacklists_deleted_at ON public.jwt_blacklists USING btree (deleted_at);


--
-- Name: idx_sys_ai_workflow_sessions_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sys_ai_workflow_sessions_deleted_at ON public.sys_ai_workflow_sessions USING btree (deleted_at);


--
-- Name: idx_sys_ai_workflow_sessions_tab; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sys_ai_workflow_sessions_tab ON public.sys_ai_workflow_sessions USING btree (tab);


--
-- Name: idx_sys_ai_workflow_sessions_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sys_ai_workflow_sessions_user_id ON public.sys_ai_workflow_sessions USING btree (user_id);


--
-- Name: idx_sys_api_tokens_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sys_api_tokens_deleted_at ON public.sys_api_tokens USING btree (deleted_at);


--
-- Name: idx_sys_apis_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sys_apis_deleted_at ON public.sys_apis USING btree (deleted_at);


--
-- Name: idx_sys_auto_code_histories_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sys_auto_code_histories_deleted_at ON public.sys_auto_code_histories USING btree (deleted_at);


--
-- Name: idx_sys_auto_code_packages_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sys_auto_code_packages_deleted_at ON public.sys_auto_code_packages USING btree (deleted_at);


--
-- Name: idx_sys_base_menu_btns_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sys_base_menu_btns_deleted_at ON public.sys_base_menu_btns USING btree (deleted_at);


--
-- Name: idx_sys_base_menu_parameters_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sys_base_menu_parameters_deleted_at ON public.sys_base_menu_parameters USING btree (deleted_at);


--
-- Name: idx_sys_base_menus_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sys_base_menus_deleted_at ON public.sys_base_menus USING btree (deleted_at);


--
-- Name: idx_sys_dictionaries_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sys_dictionaries_deleted_at ON public.sys_dictionaries USING btree (deleted_at);


--
-- Name: idx_sys_dictionary_details_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sys_dictionary_details_deleted_at ON public.sys_dictionary_details USING btree (deleted_at);


--
-- Name: idx_sys_error_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sys_error_deleted_at ON public.sys_error USING btree (deleted_at);


--
-- Name: idx_sys_export_template_condition_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sys_export_template_condition_deleted_at ON public.sys_export_template_condition USING btree (deleted_at);


--
-- Name: idx_sys_export_template_join_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sys_export_template_join_deleted_at ON public.sys_export_template_join USING btree (deleted_at);


--
-- Name: idx_sys_export_templates_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sys_export_templates_deleted_at ON public.sys_export_templates USING btree (deleted_at);


--
-- Name: idx_sys_ignore_apis_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sys_ignore_apis_deleted_at ON public.sys_ignore_apis USING btree (deleted_at);


--
-- Name: idx_sys_login_logs_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sys_login_logs_deleted_at ON public.sys_login_logs USING btree (deleted_at);


--
-- Name: idx_sys_operation_records_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sys_operation_records_deleted_at ON public.sys_operation_records USING btree (deleted_at);


--
-- Name: idx_sys_params_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sys_params_deleted_at ON public.sys_params USING btree (deleted_at);


--
-- Name: idx_sys_users_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sys_users_deleted_at ON public.sys_users USING btree (deleted_at);


--
-- Name: idx_sys_users_username; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sys_users_username ON public.sys_users USING btree (username);


--
-- Name: idx_sys_users_uuid; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sys_users_uuid ON public.sys_users USING btree (uuid);


--
-- Name: idx_sys_versions_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sys_versions_deleted_at ON public.sys_versions USING btree (deleted_at);


--
-- PostgreSQL database dump complete
--

-- These plugin operations require authentication and explicit API permissions.
INSERT INTO public.sys_apis (path, method, description, api_group, created_at, updated_at)
SELECT seed.path, seed.method, seed.description, seed.api_group, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
FROM (VALUES
    ('/autoCode/llmAuto', 'POST', 'AI 代码生成', '代码生成器'),
    ('/autoCode/llmAutoSSE', 'POST', 'AI 代码生成流式响应', '代码生成器'),
    ('/autoCode/initMenu', 'POST', '生成插件菜单文件', '代码生成器'),
    ('/autoCode/initAPI', 'POST', '生成插件 API 文件', '代码生成器'),
    ('/autoCode/initDictionary', 'POST', '生成插件字典文件', '代码生成器'),
    ('/api/freshCasbin', 'GET', '刷新权限缓存', 'api管理'),
    ('/system/reloadSystem', 'POST', '重载系统配置', '系统服务')
) AS seed(path, method, description, api_group)
WHERE NOT EXISTS (
    SELECT 1 FROM public.sys_apis a WHERE a.path = seed.path AND a.method = seed.method
);

-- Grant only the administrator every API present in this snapshot.
INSERT INTO public.casbin_rule (ptype, v0, v1, v2, v3, v4, v5)
SELECT 'p', '888', a.path, a.method, '', '', ''
FROM public.sys_apis a
WHERE a.deleted_at IS NULL AND NOT EXISTS (
    SELECT 1 FROM public.casbin_rule p
    WHERE p.ptype = 'p' AND p.v0 = '888' AND p.v1 = a.path AND p.v2 = a.method
);

COMMIT;


