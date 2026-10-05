-- LiveRoom / LiveSession V1（MySQL 8+）
-- 生产环境 disable-auto-migrate=true 时执行；执行前请先备份并在预发布验证。

CREATE TABLE IF NOT EXISTS `live_room` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `created_at` DATETIME(3) DEFAULT NULL,
  `updated_at` DATETIME(3) DEFAULT NULL,
  `deleted_at` DATETIME(3) DEFAULT NULL,
  `room_no` VARCHAR(32) NOT NULL COMMENT '直播间对外编号，默认主播编号，后台可修改',
  `anchor_id` BIGINT UNSIGNED NOT NULL COMMENT '主播内部ID，一名主播一个直播间',
  `category_id` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '当前直播分类ID',
  `title` VARCHAR(128) NOT NULL DEFAULT '' COMMENT '直播间标题',
  `cover_url` VARCHAR(500) NOT NULL DEFAULT '' COMMENT '直播间封面地址',
  `notice` VARCHAR(500) NOT NULL DEFAULT '' COMMENT '直播间公告',
  `stream_name` VARCHAR(64) NOT NULL COMMENT 'SRS稳定流名称，不是场次ID',
  `publish_secret_hash` CHAR(64) NOT NULL DEFAULT '' COMMENT '当前pt字符串SHA-256哈希',
  `stream_key_version` INT UNSIGNED NOT NULL DEFAULT 1 COMMENT '推流凭证代数，对应pt的credential_version',
  `status` TINYINT UNSIGNED NOT NULL DEFAULT 1 COMMENT '0禁用 1正常 2关闭',
  `status_reason` VARCHAR(255) NOT NULL DEFAULT '' COMMENT '禁用或关闭原因',
  `status_changed_at` BIGINT NOT NULL DEFAULT 0 COMMENT '状态变更毫秒时间戳',
  `live_status` TINYINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '0未开播 1准备中 2直播中 3结束中',
  `current_session_id` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '当前场次内部ID，0表示无',
  `last_session_id` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '最近一次已完成直播场次内部ID，0表示从未完成直播',
  `live_started_at` BIGINT NOT NULL DEFAULT 0 COMMENT '当前场次开始毫秒时间戳，结束后清零',
  `visibility` TINYINT UNSIGNED NOT NULL DEFAULT 1 COMMENT '0私密 1公开 2仅关注者',
  `recommend_weight` INT NOT NULL DEFAULT 0 COMMENT '直播间推荐权重',
  `extra` JSON DEFAULT NULL COMMENT '低频扩展配置',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_live_room_no` (`room_no`),
  UNIQUE KEY `uk_live_room_anchor` (`anchor_id`),
  UNIQUE KEY `uk_live_room_stream` (`stream_name`),
  KEY `idx_live_room_discovery` (`live_status`, `status`, `recommend_weight` DESC, `live_started_at` DESC),
  KEY `idx_live_room_category` (`category_id`, `live_status`, `status`, `recommend_weight` DESC),
  KEY `idx_live_room_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- CREATE TABLE IF NOT EXISTS 不会修改存量列，因此为存量 live_room 幂等增加最后一次已完成直播指针。
SET @has_last_session_id = (
  SELECT COUNT(*) FROM `information_schema`.`COLUMNS`
  WHERE `TABLE_SCHEMA` = DATABASE() AND `TABLE_NAME` = 'live_room' AND `COLUMN_NAME` = 'last_session_id'
);
SET @add_last_session_id_sql = IF(
  @has_last_session_id = 0,
  'ALTER TABLE `live_room` ADD COLUMN `last_session_id` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT ''最近一次已完成直播场次内部ID，0表示从未完成直播'' AFTER `current_session_id`',
  'SELECT 1'
);
PREPARE add_last_session_id_stmt FROM @add_last_session_id_sql;
EXECUTE add_last_session_id_stmt;
DEALLOCATE PREPARE add_last_session_id_stmt;

-- 为升级前已审核通过但尚无房间的主播补建直播间。
-- 只补缺失记录，不改写已有房间号；房间号和稳定流名称初始均使用主播编号。
INSERT IGNORE INTO `live_room` (
  `created_at`, `updated_at`, `room_no`, `anchor_id`, `category_id`, `title`, `cover_url`, `notice`,
  `stream_name`, `publish_secret_hash`, `stream_key_version`, `status`, `status_reason`, `status_changed_at`,
  `live_status`, `current_session_id`, `last_session_id`, `live_started_at`, `visibility`, `recommend_weight`
)
SELECT
  NOW(3), NOW(3), a.`anchor_no`, a.`id`, a.`category_id`, CONCAT(a.`nickname`, '的直播间'), a.`cover`, '',
  a.`anchor_no`, '', 1, 1, '', CAST(UNIX_TIMESTAMP(CURRENT_TIMESTAMP(3)) * 1000 AS SIGNED),
  0, 0, 0, 0, 1, 0
FROM `live_anchor` a
LEFT JOIN `live_room` r ON r.`anchor_id` = a.`id` AND r.`deleted_at` IS NULL
WHERE a.`apply_status` = 2 AND a.`deleted_at` IS NULL AND r.`id` IS NULL;

CREATE TABLE IF NOT EXISTS `live_session` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `created_at` DATETIME(3) DEFAULT NULL,
  `updated_at` DATETIME(3) DEFAULT NULL,
  `deleted_at` DATETIME(3) DEFAULT NULL,
  `session_no` VARCHAR(96) NOT NULL COMMENT '直播场次对外编号，格式为yyyyMMddHHmmssSSS-anchorNo-roomNo',
  `room_id` BIGINT UNSIGNED NOT NULL COMMENT '直播间内部ID',
  `anchor_id` BIGINT UNSIGNED NOT NULL COMMENT '主播内部ID快照',
  `category_id` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '本场分类ID快照',
  `title` VARCHAR(128) NOT NULL DEFAULT '' COMMENT '本场标题快照',
  `cover_url` VARCHAR(500) NOT NULL DEFAULT '' COMMENT '本场封面快照',
  `stream_info` JSON DEFAULT NULL COMMENT '取得有效视频宽高后保存的公开流媒体信息白名单JSON快照',
  `status` TINYINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '0准备中 1直播中 2结束中 3已结束 4已取消 5失败',
  `active_room_id` BIGINT UNSIGNED GENERATED ALWAYS AS (
    CASE WHEN `status` IN (0, 1, 2) THEN `room_id` ELSE NULL END
  ) STORED COMMENT '活动场次房间ID生成列',
  `prepare_deadline_at` BIGINT NOT NULL DEFAULT 0 COMMENT '等待首次publish回调的截止毫秒时间戳，0表示已正式开播或兼容待清理旧数据',
  `publish_credential_issue_count` INT UNSIGNED NOT NULL DEFAULT 1 COMMENT '本场推流凭证累计签发次数，prepare首次签发计为1',
  `started_at` BIGINT NOT NULL DEFAULT 0 COMMENT 'SRS首次取得有效视频宽高并确认正式开播的毫秒时间戳',
  `publish_ip` VARCHAR(45) NOT NULL DEFAULT '' COMMENT '本场首次接受publish回调的推流IP，仅管理后台可见',
  `ended_at` BIGINT NOT NULL DEFAULT 0 COMMENT '实际最终断流或主动结束毫秒时间戳',
  `duration_ms` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '逻辑直播时长，含成功重连前短暂断流',
  `last_unpublish_at` BIGINT NOT NULL DEFAULT 0 COMMENT '最近一次通过on_unpublish或SRS对账确认断流的毫秒时间戳',
  `reconnect_deadline_at` BIGINT NOT NULL DEFAULT 0 COMMENT '等待重连截止毫秒时间戳',
  `disconnect_count` INT UNSIGNED NOT NULL DEFAULT 0 COMMENT '本场累计有效断流次数',
  `srs_server_id` VARCHAR(128) NOT NULL DEFAULT '' COMMENT '最近一次发布回调携带的SRS服务实例标识',
  `srs_stream_id` VARCHAR(128) NOT NULL DEFAULT '' COMMENT '最近一次发布回调携带的SRS流标识',
  `srs_client_id` VARCHAR(128) NOT NULL DEFAULT '' COMMENT '最近一次发布回调携带的SRS客户端标识',
  `srs_vhost` VARCHAR(255) NOT NULL DEFAULT '' COMMENT '最近一次有效发布使用的SRS vhost',
  `srs_app` VARCHAR(128) NOT NULL DEFAULT '' COMMENT '最近一次有效发布使用的SRS app',
  `srs_generation` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '接受最近一次发布连接时的单SRS实例代际',
  `publisher_epoch` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '每次发布连接接管时递增的防迟到事件版本',
  `media_state` TINYINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '0未知 1发布中 2已断流 3停止中 4已停止',
  `media_state_changed_at` BIGINT NOT NULL DEFAULT 0 COMMENT '媒体状态最近变化毫秒时间戳',
  `media_last_confirmed_at` BIGINT NOT NULL DEFAULT 0 COMMENT '最近一次从SRS可靠确认媒体状态的毫秒时间戳',
  `stream_ready_deadline_at` BIGINT NOT NULL DEFAULT 0 COMMENT '等待SRS返回有效视频宽高的截止毫秒时间戳',
  `stream_probe_attempts` INT UNSIGNED NOT NULL DEFAULT 0 COMMENT 'SRS流信息退避探测次数',
  `stream_probe_next_at` BIGINT NOT NULL DEFAULT 0 COMMENT '下一次SRS流信息探测时间或处理租约截止时间',
  `stream_probe_last_error` VARCHAR(500) NOT NULL DEFAULT '' COMMENT '最近一次SRS流信息探测错误',
  `stream_reconcile_checked_at` BIGINT NOT NULL DEFAULT 0 COMMENT '最近一次直播中SRS对账完成毫秒时间戳',
  `stream_reconcile_missing_count` INT UNSIGNED NOT NULL DEFAULT 0 COMMENT '连续成功查询SRS但未发现当前发布流的次数',
  `stream_reconcile_last_seen_at` BIGINT NOT NULL DEFAULT 0 COMMENT '最近一次从SRS确认当前发布流仍存在的毫秒时间戳',
  `stream_reconcile_last_error` VARCHAR(500) NOT NULL DEFAULT '' COMMENT '最近一次直播中SRS对账请求或身份校验错误',
  `stop_requested_at` BIGINT NOT NULL DEFAULT 0 COMMENT '进入结束中的请求毫秒时间戳',
  `stop_attempts` INT UNSIGNED NOT NULL DEFAULT 0 COMMENT 'SRS停止推流累计尝试次数',
  `stop_next_retry_at` BIGINT NOT NULL DEFAULT 0 COMMENT '下次停止推流重试时间或当前处理租约截止时间',
  `stop_last_error` VARCHAR(500) NOT NULL DEFAULT '' COMMENT '最近一次停止推流失败原因',
  `stop_recovery_state` TINYINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '0自动恢复 1需要人工关注',
  `ending_alert_level` TINYINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '0无 1警告 2严重',
  `ending_alerted_at` BIGINT NOT NULL DEFAULT 0 COMMENT '最近一次提升Ending告警级别的毫秒时间戳',
  `media_stop_source` TINYINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '0未知 1SRS 2管理员',
  `media_stop_confirmed_at` BIGINT NOT NULL DEFAULT 0 COMMENT '媒体停止确认毫秒时间戳',
  `media_stop_confirmed_by` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '人工确认媒体停止的管理员ID',
  `end_reason` TINYINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '0未知 1主播结束 2管理员结束 3断流超时 4主播封禁 5系统异常 6准备开播超时',
  `failure_reason` VARCHAR(500) NOT NULL DEFAULT '' COMMENT '取消、失败或强制结束原因',
  `view_count` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '累计进入直播间次数',
  `viewer_count` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '累计去重观看人数',
  `peak_online_count` INT UNSIGNED NOT NULL DEFAULT 0 COMMENT '最高同时在线人数',
  `like_count` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '本场最终点赞次数',
  `gift_count` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '本场有效礼物总件数',
  `gift_coin_amount` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '本场有效礼物金币总额',
  `gift_user_count` INT UNSIGNED NOT NULL DEFAULT 0 COMMENT '本场去重送礼人数',
  `stats_finalized_at` BIGINT NOT NULL DEFAULT 0 COMMENT '最终统计汇总完成毫秒时间戳',
  `extra` JSON DEFAULT NULL COMMENT '场次低频扩展信息',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_live_session_no` (`session_no`),
  UNIQUE KEY `uk_live_session_active_room` (`active_room_id`),
  KEY `idx_live_session_room_created` (`room_id`, `created_at`),
  KEY `idx_live_session_anchor_started` (`anchor_id`, `started_at` DESC),
  KEY `idx_live_session_category_started` (`category_id`, `started_at` DESC),
  KEY `idx_live_session_prepare_timeout` (`status`, `prepare_deadline_at`),
  KEY `idx_live_session_stream_ready` (`status`, `stream_ready_deadline_at`),
  KEY `idx_live_session_stream_probe` (`status`, `stream_probe_next_at`),
  KEY `idx_live_session_stream_reconcile` (`status`, `stream_reconcile_checked_at`),
  KEY `idx_live_session_reconnect` (`status`, `reconnect_deadline_at`),
  KEY `idx_live_session_end_retry` (`status`, `stop_next_retry_at`),
  KEY `idx_live_session_end_stall` (`status`, `stop_requested_at`),
  KEY `idx_live_session_end_attention` (`stop_recovery_state`, `ending_alert_level`),
  KEY `idx_live_session_media_state` (`media_state`),
  KEY `idx_live_session_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `live_srs_runtime` (
  `id` BIGINT UNSIGNED NOT NULL,
  `server_id` VARCHAR(128) NOT NULL DEFAULT '',
  `service_id` VARCHAR(128) NOT NULL DEFAULT '',
  `generation` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `health_state` TINYINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '0未知 1健康 2降级',
  `last_success_at` BIGINT NOT NULL DEFAULT 0,
  `last_failure_at` BIGINT NOT NULL DEFAULT 0,
  `degraded_at` BIGINT NOT NULL DEFAULT 0,
  `recovered_at` BIGINT NOT NULL DEFAULT 0,
  `last_error` VARCHAR(500) NOT NULL DEFAULT '',
  `version` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO `live_srs_runtime` (`id`) VALUES (1)
ON DUPLICATE KEY UPDATE `id` = VALUES(`id`);

CREATE TABLE IF NOT EXISTS `live_session_media_audit` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `created_at` BIGINT NOT NULL,
  `session_id` BIGINT UNSIGNED NOT NULL,
  `room_id` BIGINT UNSIGNED NOT NULL,
  `operator_id` BIGINT UNSIGNED NOT NULL,
  `operator_name` VARCHAR(128) NOT NULL DEFAULT '',
  `reason` VARCHAR(500) NOT NULL,
  `evidence` VARCHAR(1000) NOT NULL,
  `client_ip` VARCHAR(64) NOT NULL DEFAULT '',
  `user_agent` VARCHAR(500) NOT NULL DEFAULT '',
  `request_id` VARCHAR(128) NOT NULL DEFAULT '',
  `publisher_epoch` BIGINT UNSIGNED NOT NULL,
  `srs_generation` BIGINT UNSIGNED NOT NULL,
  `srs_health_state` TINYINT UNSIGNED NOT NULL,
  `srs_last_success_at` BIGINT NOT NULL,
  `previous_media_state` TINYINT UNSIGNED NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_live_media_audit_session` (`session_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- CREATE TABLE IF NOT EXISTS 不会修改存量列，因此这里同时兼容新建和已存在的 live_session。
SET @has_stream_info = (
  SELECT COUNT(*) FROM `information_schema`.`COLUMNS`
  WHERE `TABLE_SCHEMA` = DATABASE() AND `TABLE_NAME` = 'live_session' AND `COLUMN_NAME` = 'stream_info'
);
SET @add_stream_info_sql = IF(
  @has_stream_info = 0,
  'ALTER TABLE `live_session` ADD COLUMN `stream_info` JSON DEFAULT NULL COMMENT ''取得有效视频宽高后保存的公开流媒体信息白名单JSON快照'' AFTER `cover_url`',
  'SELECT 1'
);
PREPARE add_stream_info_stmt FROM @add_stream_info_sql;
EXECUTE add_stream_info_stmt;
DEALLOCATE PREPARE add_stream_info_stmt;

SET @has_prepare_deadline_at = (
  SELECT COUNT(*) FROM `information_schema`.`COLUMNS`
  WHERE `TABLE_SCHEMA` = DATABASE() AND `TABLE_NAME` = 'live_session' AND `COLUMN_NAME` = 'prepare_deadline_at'
);
SET @add_prepare_deadline_at_sql = IF(
  @has_prepare_deadline_at = 0,
  'ALTER TABLE `live_session` ADD COLUMN `prepare_deadline_at` BIGINT NOT NULL DEFAULT 0 COMMENT ''等待首次publish回调的截止毫秒时间戳，0表示已正式开播或兼容待清理旧数据'' AFTER `active_room_id`',
  'SELECT 1'
);
PREPARE add_prepare_deadline_at_stmt FROM @add_prepare_deadline_at_sql;
EXECUTE add_prepare_deadline_at_stmt;
DEALLOCATE PREPARE add_prepare_deadline_at_stmt;

SET @has_publish_credential_issue_count = (
  SELECT COUNT(*) FROM `information_schema`.`COLUMNS`
  WHERE `TABLE_SCHEMA` = DATABASE() AND `TABLE_NAME` = 'live_session' AND `COLUMN_NAME` = 'publish_credential_issue_count'
);
SET @add_publish_credential_issue_count_sql = IF(
  @has_publish_credential_issue_count = 0,
  'ALTER TABLE `live_session` ADD COLUMN `publish_credential_issue_count` INT UNSIGNED NOT NULL DEFAULT 1 COMMENT ''本场推流凭证累计签发次数，prepare首次签发计为1'' AFTER `prepare_deadline_at`',
  'SELECT 1'
);
PREPARE add_publish_credential_issue_count_stmt FROM @add_publish_credential_issue_count_sql;
EXECUTE add_publish_credential_issue_count_stmt;
DEALLOCATE PREPARE add_publish_credential_issue_count_stmt;

SET @has_prepare_timeout_index = (
  SELECT COUNT(*) FROM `information_schema`.`STATISTICS`
  WHERE `TABLE_SCHEMA` = DATABASE() AND `TABLE_NAME` = 'live_session' AND `INDEX_NAME` = 'idx_live_session_prepare_timeout'
);
SET @add_prepare_timeout_index_sql = IF(
  @has_prepare_timeout_index = 0,
  'ALTER TABLE `live_session` ADD INDEX `idx_live_session_prepare_timeout` (`status`, `prepare_deadline_at`)',
  'SELECT 1'
);
PREPARE add_prepare_timeout_index_stmt FROM @add_prepare_timeout_index_sql;
EXECUTE add_prepare_timeout_index_stmt;
DEALLOCATE PREPARE add_prepare_timeout_index_stmt;

SET @has_publish_ip = (
  SELECT COUNT(*) FROM `information_schema`.`COLUMNS`
  WHERE `TABLE_SCHEMA` = DATABASE() AND `TABLE_NAME` = 'live_session' AND `COLUMN_NAME` = 'publish_ip'
);
SET @add_publish_ip_sql = IF(
  @has_publish_ip = 0,
  'ALTER TABLE `live_session` ADD COLUMN `publish_ip` VARCHAR(45) NOT NULL DEFAULT '''' COMMENT ''本场首次接受publish回调的推流IP，仅管理后台可见'' AFTER `started_at`',
  'SELECT 1'
);
PREPARE add_publish_ip_stmt FROM @add_publish_ip_sql;
EXECUTE add_publish_ip_stmt;
DEALLOCATE PREPARE add_publish_ip_stmt;

SET @has_srs_server_id = (
  SELECT COUNT(*) FROM `information_schema`.`COLUMNS`
  WHERE `TABLE_SCHEMA` = DATABASE() AND `TABLE_NAME` = 'live_session' AND `COLUMN_NAME` = 'srs_server_id'
);
SET @add_srs_server_id_sql = IF(
  @has_srs_server_id = 0,
  'ALTER TABLE `live_session` ADD COLUMN `srs_server_id` VARCHAR(128) NOT NULL DEFAULT '''' COMMENT ''最近一次发布回调携带的SRS服务实例标识'' AFTER `disconnect_count`',
  'SELECT 1'
);
PREPARE add_srs_server_id_stmt FROM @add_srs_server_id_sql;
EXECUTE add_srs_server_id_stmt;
DEALLOCATE PREPARE add_srs_server_id_stmt;

SET @has_srs_stream_id = (
  SELECT COUNT(*) FROM `information_schema`.`COLUMNS`
  WHERE `TABLE_SCHEMA` = DATABASE() AND `TABLE_NAME` = 'live_session' AND `COLUMN_NAME` = 'srs_stream_id'
);
SET @add_srs_stream_id_sql = IF(
  @has_srs_stream_id = 0,
  'ALTER TABLE `live_session` ADD COLUMN `srs_stream_id` VARCHAR(128) NOT NULL DEFAULT '''' COMMENT ''最近一次发布回调携带的SRS流标识'' AFTER `srs_server_id`',
  'SELECT 1'
);
PREPARE add_srs_stream_id_stmt FROM @add_srs_stream_id_sql;
EXECUTE add_srs_stream_id_stmt;
DEALLOCATE PREPARE add_srs_stream_id_stmt;

SET @has_srs_client_id = (
  SELECT COUNT(*) FROM `information_schema`.`COLUMNS`
  WHERE `TABLE_SCHEMA` = DATABASE() AND `TABLE_NAME` = 'live_session' AND `COLUMN_NAME` = 'srs_client_id'
);
SET @add_srs_client_id_sql = IF(
  @has_srs_client_id = 0,
  'ALTER TABLE `live_session` ADD COLUMN `srs_client_id` VARCHAR(128) NOT NULL DEFAULT '''' COMMENT ''最近一次发布回调携带的SRS客户端标识'' AFTER `srs_stream_id`',
  'SELECT 1'
);
PREPARE add_srs_client_id_stmt FROM @add_srs_client_id_sql;
EXECUTE add_srs_client_id_stmt;
DEALLOCATE PREPARE add_srs_client_id_stmt;

SET @has_srs_vhost = (
  SELECT COUNT(*) FROM `information_schema`.`COLUMNS`
  WHERE `TABLE_SCHEMA` = DATABASE() AND `TABLE_NAME` = 'live_session' AND `COLUMN_NAME` = 'srs_vhost'
);
SET @add_srs_vhost_sql = IF(
  @has_srs_vhost = 0,
  'ALTER TABLE `live_session` ADD COLUMN `srs_vhost` VARCHAR(255) NOT NULL DEFAULT '''' COMMENT ''最近一次有效发布使用的SRS vhost'' AFTER `srs_client_id`',
  'SELECT 1'
);
PREPARE add_srs_vhost_stmt FROM @add_srs_vhost_sql;
EXECUTE add_srs_vhost_stmt;
DEALLOCATE PREPARE add_srs_vhost_stmt;

SET @has_srs_app = (
  SELECT COUNT(*) FROM `information_schema`.`COLUMNS`
  WHERE `TABLE_SCHEMA` = DATABASE() AND `TABLE_NAME` = 'live_session' AND `COLUMN_NAME` = 'srs_app'
);
SET @add_srs_app_sql = IF(
  @has_srs_app = 0,
  'ALTER TABLE `live_session` ADD COLUMN `srs_app` VARCHAR(128) NOT NULL DEFAULT '''' COMMENT ''最近一次有效发布使用的SRS app'' AFTER `srs_vhost`',
  'SELECT 1'
);
PREPARE add_srs_app_stmt FROM @add_srs_app_sql;
EXECUTE add_srs_app_stmt;
DEALLOCATE PREPARE add_srs_app_stmt;

SET @has_stream_ready_deadline_at = (
  SELECT COUNT(*) FROM `information_schema`.`COLUMNS`
  WHERE `TABLE_SCHEMA` = DATABASE() AND `TABLE_NAME` = 'live_session' AND `COLUMN_NAME` = 'stream_ready_deadline_at'
);
SET @add_stream_ready_deadline_at_sql = IF(
  @has_stream_ready_deadline_at = 0,
  'ALTER TABLE `live_session` ADD COLUMN `stream_ready_deadline_at` BIGINT NOT NULL DEFAULT 0 COMMENT ''等待SRS返回有效视频宽高的截止毫秒时间戳'' AFTER `srs_app`',
  'SELECT 1'
);
PREPARE add_stream_ready_deadline_at_stmt FROM @add_stream_ready_deadline_at_sql;
EXECUTE add_stream_ready_deadline_at_stmt;
DEALLOCATE PREPARE add_stream_ready_deadline_at_stmt;

SET @has_stream_probe_attempts = (
  SELECT COUNT(*) FROM `information_schema`.`COLUMNS`
  WHERE `TABLE_SCHEMA` = DATABASE() AND `TABLE_NAME` = 'live_session' AND `COLUMN_NAME` = 'stream_probe_attempts'
);
SET @add_stream_probe_attempts_sql = IF(
  @has_stream_probe_attempts = 0,
  'ALTER TABLE `live_session` ADD COLUMN `stream_probe_attempts` INT UNSIGNED NOT NULL DEFAULT 0 COMMENT ''SRS流信息退避探测次数'' AFTER `stream_ready_deadline_at`',
  'SELECT 1'
);
PREPARE add_stream_probe_attempts_stmt FROM @add_stream_probe_attempts_sql;
EXECUTE add_stream_probe_attempts_stmt;
DEALLOCATE PREPARE add_stream_probe_attempts_stmt;

SET @has_stream_probe_next_at = (
  SELECT COUNT(*) FROM `information_schema`.`COLUMNS`
  WHERE `TABLE_SCHEMA` = DATABASE() AND `TABLE_NAME` = 'live_session' AND `COLUMN_NAME` = 'stream_probe_next_at'
);
SET @add_stream_probe_next_at_sql = IF(
  @has_stream_probe_next_at = 0,
  'ALTER TABLE `live_session` ADD COLUMN `stream_probe_next_at` BIGINT NOT NULL DEFAULT 0 COMMENT ''下一次SRS流信息探测时间或处理租约截止时间'' AFTER `stream_probe_attempts`',
  'SELECT 1'
);
PREPARE add_stream_probe_next_at_stmt FROM @add_stream_probe_next_at_sql;
EXECUTE add_stream_probe_next_at_stmt;
DEALLOCATE PREPARE add_stream_probe_next_at_stmt;

SET @has_stream_probe_last_error = (
  SELECT COUNT(*) FROM `information_schema`.`COLUMNS`
  WHERE `TABLE_SCHEMA` = DATABASE() AND `TABLE_NAME` = 'live_session' AND `COLUMN_NAME` = 'stream_probe_last_error'
);
SET @add_stream_probe_last_error_sql = IF(
  @has_stream_probe_last_error = 0,
  'ALTER TABLE `live_session` ADD COLUMN `stream_probe_last_error` VARCHAR(500) NOT NULL DEFAULT '''' COMMENT ''最近一次SRS流信息探测错误'' AFTER `stream_probe_next_at`',
  'SELECT 1'
);
PREPARE add_stream_probe_last_error_stmt FROM @add_stream_probe_last_error_sql;
EXECUTE add_stream_probe_last_error_stmt;
DEALLOCATE PREPARE add_stream_probe_last_error_stmt;

SET @has_stream_ready_index = (
  SELECT COUNT(*) FROM `information_schema`.`STATISTICS`
  WHERE `TABLE_SCHEMA` = DATABASE() AND `TABLE_NAME` = 'live_session' AND `INDEX_NAME` = 'idx_live_session_stream_ready'
);
SET @add_stream_ready_index_sql = IF(
  @has_stream_ready_index = 0,
  'ALTER TABLE `live_session` ADD INDEX `idx_live_session_stream_ready` (`status`, `stream_ready_deadline_at`)',
  'SELECT 1'
);
PREPARE add_stream_ready_index_stmt FROM @add_stream_ready_index_sql;
EXECUTE add_stream_ready_index_stmt;
DEALLOCATE PREPARE add_stream_ready_index_stmt;

SET @has_stream_probe_index = (
  SELECT COUNT(*) FROM `information_schema`.`STATISTICS`
  WHERE `TABLE_SCHEMA` = DATABASE() AND `TABLE_NAME` = 'live_session' AND `INDEX_NAME` = 'idx_live_session_stream_probe'
);
SET @add_stream_probe_index_sql = IF(
  @has_stream_probe_index = 0,
  'ALTER TABLE `live_session` ADD INDEX `idx_live_session_stream_probe` (`status`, `stream_probe_next_at`)',
  'SELECT 1'
);
PREPARE add_stream_probe_index_stmt FROM @add_stream_probe_index_sql;
EXECUTE add_stream_probe_index_stmt;
DEALLOCATE PREPARE add_stream_probe_index_stmt;

SET @has_stream_reconcile_checked_at = (
  SELECT COUNT(*) FROM `information_schema`.`COLUMNS`
  WHERE `TABLE_SCHEMA` = DATABASE() AND `TABLE_NAME` = 'live_session' AND `COLUMN_NAME` = 'stream_reconcile_checked_at'
);
SET @add_stream_reconcile_checked_at_sql = IF(
  @has_stream_reconcile_checked_at = 0,
  'ALTER TABLE `live_session` ADD COLUMN `stream_reconcile_checked_at` BIGINT NOT NULL DEFAULT 0 COMMENT ''最近一次直播中SRS对账完成毫秒时间戳'' AFTER `stream_probe_last_error`',
  'SELECT 1'
);
PREPARE add_stream_reconcile_checked_at_stmt FROM @add_stream_reconcile_checked_at_sql;
EXECUTE add_stream_reconcile_checked_at_stmt;
DEALLOCATE PREPARE add_stream_reconcile_checked_at_stmt;

SET @has_stream_reconcile_missing_count = (
  SELECT COUNT(*) FROM `information_schema`.`COLUMNS`
  WHERE `TABLE_SCHEMA` = DATABASE() AND `TABLE_NAME` = 'live_session' AND `COLUMN_NAME` = 'stream_reconcile_missing_count'
);
SET @add_stream_reconcile_missing_count_sql = IF(
  @has_stream_reconcile_missing_count = 0,
  'ALTER TABLE `live_session` ADD COLUMN `stream_reconcile_missing_count` INT UNSIGNED NOT NULL DEFAULT 0 COMMENT ''连续成功查询SRS但未发现当前发布流的次数'' AFTER `stream_reconcile_checked_at`',
  'SELECT 1'
);
PREPARE add_stream_reconcile_missing_count_stmt FROM @add_stream_reconcile_missing_count_sql;
EXECUTE add_stream_reconcile_missing_count_stmt;
DEALLOCATE PREPARE add_stream_reconcile_missing_count_stmt;

SET @has_stream_reconcile_last_seen_at = (
  SELECT COUNT(*) FROM `information_schema`.`COLUMNS`
  WHERE `TABLE_SCHEMA` = DATABASE() AND `TABLE_NAME` = 'live_session' AND `COLUMN_NAME` = 'stream_reconcile_last_seen_at'
);
SET @add_stream_reconcile_last_seen_at_sql = IF(
  @has_stream_reconcile_last_seen_at = 0,
  'ALTER TABLE `live_session` ADD COLUMN `stream_reconcile_last_seen_at` BIGINT NOT NULL DEFAULT 0 COMMENT ''最近一次从SRS确认当前发布流仍存在的毫秒时间戳'' AFTER `stream_reconcile_missing_count`',
  'SELECT 1'
);
PREPARE add_stream_reconcile_last_seen_at_stmt FROM @add_stream_reconcile_last_seen_at_sql;
EXECUTE add_stream_reconcile_last_seen_at_stmt;
DEALLOCATE PREPARE add_stream_reconcile_last_seen_at_stmt;

SET @has_stream_reconcile_last_error = (
  SELECT COUNT(*) FROM `information_schema`.`COLUMNS`
  WHERE `TABLE_SCHEMA` = DATABASE() AND `TABLE_NAME` = 'live_session' AND `COLUMN_NAME` = 'stream_reconcile_last_error'
);
SET @add_stream_reconcile_last_error_sql = IF(
  @has_stream_reconcile_last_error = 0,
  'ALTER TABLE `live_session` ADD COLUMN `stream_reconcile_last_error` VARCHAR(500) NOT NULL DEFAULT '''' COMMENT ''最近一次直播中SRS对账请求或身份校验错误'' AFTER `stream_reconcile_last_seen_at`',
  'SELECT 1'
);
PREPARE add_stream_reconcile_last_error_stmt FROM @add_stream_reconcile_last_error_sql;
EXECUTE add_stream_reconcile_last_error_stmt;
DEALLOCATE PREPARE add_stream_reconcile_last_error_stmt;

SET @has_stream_reconcile_index = (
  SELECT COUNT(*) FROM `information_schema`.`STATISTICS`
  WHERE `TABLE_SCHEMA` = DATABASE() AND `TABLE_NAME` = 'live_session' AND `INDEX_NAME` = 'idx_live_session_stream_reconcile'
);
SET @add_stream_reconcile_index_sql = IF(
  @has_stream_reconcile_index = 0,
  'ALTER TABLE `live_session` ADD INDEX `idx_live_session_stream_reconcile` (`status`, `stream_reconcile_checked_at`)',
  'SELECT 1'
);
PREPARE add_stream_reconcile_index_stmt FROM @add_stream_reconcile_index_sql;
EXECUTE add_stream_reconcile_index_stmt;
DEALLOCATE PREPARE add_stream_reconcile_index_stmt;

SET @has_stop_requested_at = (
  SELECT COUNT(*) FROM `information_schema`.`COLUMNS`
  WHERE `TABLE_SCHEMA` = DATABASE() AND `TABLE_NAME` = 'live_session' AND `COLUMN_NAME` = 'stop_requested_at'
);
SET @add_stop_requested_at_sql = IF(
  @has_stop_requested_at = 0,
  'ALTER TABLE `live_session` ADD COLUMN `stop_requested_at` BIGINT NOT NULL DEFAULT 0 COMMENT ''进入结束中的请求毫秒时间戳'' AFTER `srs_app`',
  'SELECT 1'
);
PREPARE add_stop_requested_at_stmt FROM @add_stop_requested_at_sql;
EXECUTE add_stop_requested_at_stmt;
DEALLOCATE PREPARE add_stop_requested_at_stmt;

SET @has_stop_attempts = (
  SELECT COUNT(*) FROM `information_schema`.`COLUMNS`
  WHERE `TABLE_SCHEMA` = DATABASE() AND `TABLE_NAME` = 'live_session' AND `COLUMN_NAME` = 'stop_attempts'
);
SET @add_stop_attempts_sql = IF(
  @has_stop_attempts = 0,
  'ALTER TABLE `live_session` ADD COLUMN `stop_attempts` INT UNSIGNED NOT NULL DEFAULT 0 COMMENT ''SRS停止推流累计尝试次数'' AFTER `stop_requested_at`',
  'SELECT 1'
);
PREPARE add_stop_attempts_stmt FROM @add_stop_attempts_sql;
EXECUTE add_stop_attempts_stmt;
DEALLOCATE PREPARE add_stop_attempts_stmt;

SET @has_stop_next_retry_at = (
  SELECT COUNT(*) FROM `information_schema`.`COLUMNS`
  WHERE `TABLE_SCHEMA` = DATABASE() AND `TABLE_NAME` = 'live_session' AND `COLUMN_NAME` = 'stop_next_retry_at'
);
SET @add_stop_next_retry_at_sql = IF(
  @has_stop_next_retry_at = 0,
  'ALTER TABLE `live_session` ADD COLUMN `stop_next_retry_at` BIGINT NOT NULL DEFAULT 0 COMMENT ''下次停止推流重试时间或当前处理租约截止时间'' AFTER `stop_attempts`',
  'SELECT 1'
);
PREPARE add_stop_next_retry_at_stmt FROM @add_stop_next_retry_at_sql;
EXECUTE add_stop_next_retry_at_stmt;
DEALLOCATE PREPARE add_stop_next_retry_at_stmt;

SET @has_stop_last_error = (
  SELECT COUNT(*) FROM `information_schema`.`COLUMNS`
  WHERE `TABLE_SCHEMA` = DATABASE() AND `TABLE_NAME` = 'live_session' AND `COLUMN_NAME` = 'stop_last_error'
);
SET @add_stop_last_error_sql = IF(
  @has_stop_last_error = 0,
  'ALTER TABLE `live_session` ADD COLUMN `stop_last_error` VARCHAR(500) NOT NULL DEFAULT '''' COMMENT ''最近一次停止推流失败原因'' AFTER `stop_next_retry_at`',
  'SELECT 1'
);
PREPARE add_stop_last_error_stmt FROM @add_stop_last_error_sql;
EXECUTE add_stop_last_error_stmt;
DEALLOCATE PREPARE add_stop_last_error_stmt;

-- 单SRS代际、媒体状态和人工恢复字段。使用一次动态ALTER兼容已有表；
-- 全部字段已存在时退化为SELECT 1，可安全重复执行。
SET @add_live_recovery_columns_sql = CONCAT(
  'ALTER TABLE `live_session` ',
  IF((SELECT COUNT(*) FROM `information_schema`.`COLUMNS` WHERE `TABLE_SCHEMA` = DATABASE() AND `TABLE_NAME` = 'live_session' AND `COLUMN_NAME` = 'srs_generation') = 0,
    'ADD COLUMN `srs_generation` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT ''接受最近一次发布连接时的单SRS实例代际'',', ''),
  IF((SELECT COUNT(*) FROM `information_schema`.`COLUMNS` WHERE `TABLE_SCHEMA` = DATABASE() AND `TABLE_NAME` = 'live_session' AND `COLUMN_NAME` = 'publisher_epoch') = 0,
    'ADD COLUMN `publisher_epoch` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT ''每次发布连接接管时递增的防迟到事件版本'',', ''),
  IF((SELECT COUNT(*) FROM `information_schema`.`COLUMNS` WHERE `TABLE_SCHEMA` = DATABASE() AND `TABLE_NAME` = 'live_session' AND `COLUMN_NAME` = 'media_state') = 0,
    'ADD COLUMN `media_state` TINYINT UNSIGNED NOT NULL DEFAULT 0 COMMENT ''0未知 1发布中 2已断流 3停止中 4已停止'',', ''),
  IF((SELECT COUNT(*) FROM `information_schema`.`COLUMNS` WHERE `TABLE_SCHEMA` = DATABASE() AND `TABLE_NAME` = 'live_session' AND `COLUMN_NAME` = 'media_state_changed_at') = 0,
    'ADD COLUMN `media_state_changed_at` BIGINT NOT NULL DEFAULT 0 COMMENT ''媒体状态最近变化毫秒时间戳'',', ''),
  IF((SELECT COUNT(*) FROM `information_schema`.`COLUMNS` WHERE `TABLE_SCHEMA` = DATABASE() AND `TABLE_NAME` = 'live_session' AND `COLUMN_NAME` = 'media_last_confirmed_at') = 0,
    'ADD COLUMN `media_last_confirmed_at` BIGINT NOT NULL DEFAULT 0 COMMENT ''最近一次从SRS可靠确认媒体状态的毫秒时间戳'',', ''),
  IF((SELECT COUNT(*) FROM `information_schema`.`COLUMNS` WHERE `TABLE_SCHEMA` = DATABASE() AND `TABLE_NAME` = 'live_session' AND `COLUMN_NAME` = 'stop_recovery_state') = 0,
    'ADD COLUMN `stop_recovery_state` TINYINT UNSIGNED NOT NULL DEFAULT 0 COMMENT ''0自动恢复 1需要人工关注'',', ''),
  IF((SELECT COUNT(*) FROM `information_schema`.`COLUMNS` WHERE `TABLE_SCHEMA` = DATABASE() AND `TABLE_NAME` = 'live_session' AND `COLUMN_NAME` = 'ending_alert_level') = 0,
    'ADD COLUMN `ending_alert_level` TINYINT UNSIGNED NOT NULL DEFAULT 0 COMMENT ''0无 1警告 2严重'',', ''),
  IF((SELECT COUNT(*) FROM `information_schema`.`COLUMNS` WHERE `TABLE_SCHEMA` = DATABASE() AND `TABLE_NAME` = 'live_session' AND `COLUMN_NAME` = 'ending_alerted_at') = 0,
    'ADD COLUMN `ending_alerted_at` BIGINT NOT NULL DEFAULT 0 COMMENT ''最近一次提升Ending告警级别的毫秒时间戳'',', ''),
  IF((SELECT COUNT(*) FROM `information_schema`.`COLUMNS` WHERE `TABLE_SCHEMA` = DATABASE() AND `TABLE_NAME` = 'live_session' AND `COLUMN_NAME` = 'media_stop_source') = 0,
    'ADD COLUMN `media_stop_source` TINYINT UNSIGNED NOT NULL DEFAULT 0 COMMENT ''0未知 1SRS 2管理员'',', ''),
  IF((SELECT COUNT(*) FROM `information_schema`.`COLUMNS` WHERE `TABLE_SCHEMA` = DATABASE() AND `TABLE_NAME` = 'live_session' AND `COLUMN_NAME` = 'media_stop_confirmed_at') = 0,
    'ADD COLUMN `media_stop_confirmed_at` BIGINT NOT NULL DEFAULT 0 COMMENT ''媒体停止确认毫秒时间戳'',', ''),
  IF((SELECT COUNT(*) FROM `information_schema`.`COLUMNS` WHERE `TABLE_SCHEMA` = DATABASE() AND `TABLE_NAME` = 'live_session' AND `COLUMN_NAME` = 'media_stop_confirmed_by') = 0,
    'ADD COLUMN `media_stop_confirmed_by` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT ''人工确认媒体停止的管理员ID'',', '')
);
SET @add_live_recovery_columns_sql = IF(
  @add_live_recovery_columns_sql = 'ALTER TABLE `live_session` ',
  'SELECT 1',
  LEFT(@add_live_recovery_columns_sql, CHAR_LENGTH(@add_live_recovery_columns_sql) - 1)
);
PREPARE add_live_recovery_columns_stmt FROM @add_live_recovery_columns_sql;
EXECUTE add_live_recovery_columns_stmt;
DEALLOCATE PREPARE add_live_recovery_columns_stmt;

SET @add_live_recovery_indexes_sql = CONCAT(
  'ALTER TABLE `live_session` ',
  IF((SELECT COUNT(*) FROM `information_schema`.`STATISTICS` WHERE `TABLE_SCHEMA` = DATABASE() AND `TABLE_NAME` = 'live_session' AND `INDEX_NAME` = 'idx_live_session_end_stall') = 0,
    'ADD INDEX `idx_live_session_end_stall` (`status`, `stop_requested_at`),', ''),
  IF((SELECT COUNT(*) FROM `information_schema`.`STATISTICS` WHERE `TABLE_SCHEMA` = DATABASE() AND `TABLE_NAME` = 'live_session' AND `INDEX_NAME` = 'idx_live_session_end_attention') = 0,
    'ADD INDEX `idx_live_session_end_attention` (`stop_recovery_state`, `ending_alert_level`),', ''),
  IF((SELECT COUNT(*) FROM `information_schema`.`STATISTICS` WHERE `TABLE_SCHEMA` = DATABASE() AND `TABLE_NAME` = 'live_session' AND `INDEX_NAME` = 'idx_live_session_media_state') = 0,
    'ADD INDEX `idx_live_session_media_state` (`media_state`),', '')
);
SET @add_live_recovery_indexes_sql = IF(
  @add_live_recovery_indexes_sql = 'ALTER TABLE `live_session` ',
  'SELECT 1',
  LEFT(@add_live_recovery_indexes_sql, CHAR_LENGTH(@add_live_recovery_indexes_sql) - 1)
);
PREPARE add_live_recovery_indexes_stmt FROM @add_live_recovery_indexes_sql;
EXECUTE add_live_recovery_indexes_stmt;
DEALLOCATE PREPARE add_live_recovery_indexes_stmt;

SET @has_end_retry_index = (
  SELECT COUNT(*) FROM `information_schema`.`STATISTICS`
  WHERE `TABLE_SCHEMA` = DATABASE() AND `TABLE_NAME` = 'live_session' AND `INDEX_NAME` = 'idx_live_session_end_retry'
);
SET @add_end_retry_index_sql = IF(
  @has_end_retry_index = 0,
  'ALTER TABLE `live_session` ADD INDEX `idx_live_session_end_retry` (`status`, `stop_next_retry_at`)',
  'SELECT 1'
);
PREPARE add_end_retry_index_stmt FROM @add_end_retry_index_sql;
EXECUTE add_end_retry_index_stmt;
DEALLOCATE PREPARE add_end_retry_index_stmt;

ALTER TABLE `live_session`
  MODIFY COLUMN `session_no` VARCHAR(96) NOT NULL
  COMMENT '直播场次对外编号，格式为yyyyMMddHHmmssSSS-anchorNo-roomNo',
  MODIFY COLUMN `end_reason` TINYINT UNSIGNED NOT NULL DEFAULT 0
  COMMENT '0未知 1主播结束 2管理员结束 3断流超时 4主播封禁 5系统异常 6准备开播超时';

-- 只回填真正完成结算的直播；准备后取消或失败的场次不属于“最后一次直播”。
UPDATE `live_room` r
JOIN (
  SELECT `room_id`, MAX(`id`) AS `last_session_id`
  FROM `live_session`
  WHERE `status` = 3 AND `deleted_at` IS NULL
  GROUP BY `room_id`
) latest ON latest.`room_id` = r.`id`
SET r.`last_session_id` = latest.`last_session_id`
WHERE r.`last_session_id` = 0;

-- 若旧 live_anchor 仍为秒字段，请只执行一次以下迁移：
-- ALTER TABLE live_anchor ADD COLUMN total_live_duration_ms BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '累计逻辑直播时长，单位毫秒';
-- UPDATE live_anchor SET total_live_duration_ms = total_live_duration * 1000 WHERE total_live_duration_ms = 0 AND total_live_duration > 0;
-- ALTER TABLE live_anchor DROP COLUMN total_live_duration;
