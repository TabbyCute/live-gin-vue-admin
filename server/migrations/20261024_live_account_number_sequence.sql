-- MySQL 8.0 migration for short numeric client account numbers.
-- Safe to run after the main live_account migration. Existing account_no values are left unchanged.

CREATE TABLE IF NOT EXISTS `live_account_number_sequence` (
  `id` tinyint unsigned NOT NULL,
  `next_value` bigint unsigned NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='客户端数字账号序列';

INSERT INTO `live_account_number_sequence` (`id`, `next_value`)
VALUES (1, 1145781)
ON DUPLICATE KEY UPDATE `next_value` = GREATEST(`next_value`, VALUES(`next_value`));
