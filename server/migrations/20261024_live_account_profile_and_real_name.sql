-- MySQL 8.0 migration. Back up the database before applying.
-- Production keeps system.disable-auto-migrate=true, so deploy this script before the new binary.

ALTER TABLE `live_account`
  ADD COLUMN `account_no` varchar(32) NULL COMMENT '客户端对外账号编号' AFTER `id`,
  ADD COLUMN `phone` varchar(20) NULL COMMENT '验证通过的E.164手机号' AFTER `password`,
  ADD COLUMN `phone_verified_at` bigint NOT NULL DEFAULT 0 COMMENT '手机号验证完成时间，毫秒时间戳' AFTER `phone`,
  ADD COLUMN `signature` varchar(255) NOT NULL DEFAULT '' COMMENT '个性签名' AFTER `avatar`,
  ADD COLUMN `gender` tinyint unsigned NOT NULL DEFAULT 0 COMMENT '性别：0未知 1男 2女 3其他' AFTER `signature`,
  ADD COLUMN `birthday` date NULL COMMENT '出生日期' AFTER `gender`,
  ADD COLUMN `country_code` char(2) NOT NULL DEFAULT '' COMMENT '国家或地区代码' AFTER `birthday`,
  ADD COLUMN `region_code` varchar(32) NOT NULL DEFAULT '' COMMENT '省州地区编码' AFTER `country_code`,
  ADD COLUMN `city_code` varchar(32) NOT NULL DEFAULT '' COMMENT '城市编码' AFTER `region_code`,
  ADD COLUMN `language` varchar(16) NOT NULL DEFAULT '' COMMENT '首选语言' AFTER `city_code`,
  ADD COLUMN `real_name_status` tinyint unsigned NOT NULL DEFAULT 0 COMMENT '实名认证状态' AFTER `language`,
  ADD COLUMN `real_name_verified_at` bigint NOT NULL DEFAULT 0 COMMENT '实名认证通过时间' AFTER `real_name_status`,
  ADD COLUMN `status_reason` varchar(255) NOT NULL DEFAULT '' COMMENT '当前账号状态原因' AFTER `status`,
  ADD COLUMN `ban_until` bigint NOT NULL DEFAULT 0 COMMENT '封禁截止毫秒时间戳' AFTER `status_reason`,
  ADD COLUMN `token_version` bigint unsigned NOT NULL DEFAULT 1 COMMENT '客户端JWT撤销版本' AFTER `ban_until`,
  ADD COLUMN `password_changed_at` bigint NOT NULL DEFAULT 0 COMMENT '最近修改密码时间' AFTER `token_version`,
  ADD COLUMN `last_login_at` bigint NOT NULL DEFAULT 0 COMMENT '最近成功登录时间' AFTER `password_changed_at`,
  ADD COLUMN `register_source` varchar(32) NOT NULL DEFAULT 'app' COMMENT '注册来源' AFTER `last_login_at`,
  ADD COLUMN `register_channel_id` bigint unsigned NOT NULL DEFAULT 0 COMMENT '注册渠道ID' AFTER `register_source`;

UPDATE `live_account`
SET `account_no` = CONCAT('U', UPPER(SUBSTRING(REPLACE(UUID(), '-', ''), 1, 20)))
WHERE `account_no` IS NULL OR `account_no` = '';

ALTER TABLE `live_account`
  MODIFY COLUMN `account_no` varchar(32) NOT NULL COMMENT '客户端对外账号编号',
  MODIFY COLUMN `status` tinyint unsigned NOT NULL DEFAULT 1 COMMENT '账号状态：1正常 2禁用 3封禁 4注销',
  ADD UNIQUE KEY `uk_live_account_account_no` (`account_no`),
  ADD UNIQUE KEY `uk_live_account_phone` (`phone`),
  ADD KEY `idx_live_account_created_at` (`created_at`),
  ADD KEY `idx_live_account_real_name_status` (`real_name_status`),
  ADD KEY `idx_live_account_status_ban` (`status`, `ban_until`),
  ADD KEY `idx_live_account_register_channel` (`register_channel_id`);

CREATE TABLE IF NOT EXISTS `live_account_real_name` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `account_id` bigint unsigned NOT NULL,
  `country_code` char(2) NOT NULL,
  `document_type` tinyint unsigned NOT NULL,
  `legal_name_ciphertext` text NOT NULL,
  `legal_name_masked` varchar(64) NOT NULL DEFAULT '',
  `document_no_ciphertext` text NOT NULL,
  `document_no_hash` binary(32) NOT NULL,
  `document_no_masked` varchar(64) NOT NULL DEFAULT '',
  `encryption_key_id` varchar(32) NOT NULL DEFAULT '',
  `status` tinyint unsigned NOT NULL DEFAULT 1,
  `provider` varchar(32) NOT NULL DEFAULT 'manual',
  `provider_reference` varchar(128) DEFAULT NULL,
  `submitted_at` bigint NOT NULL DEFAULT 0,
  `reviewed_at` bigint NOT NULL DEFAULT 0,
  `expires_at` bigint NOT NULL DEFAULT 0,
  `review_user_id` bigint unsigned NOT NULL DEFAULT 0,
  `reject_reason` varchar(255) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_live_account_real_name_account` (`account_id`),
  UNIQUE KEY `uk_live_account_real_name_document` (`document_no_hash`),
  UNIQUE KEY `uk_live_account_real_name_provider_ref` (`provider`, `provider_reference`),
  KEY `idx_live_account_real_name_status_expire` (`status`, `expires_at`),
  KEY `idx_live_account_real_name_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='客户端实名认证保留表';
