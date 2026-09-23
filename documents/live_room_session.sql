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
  `publish_secret_hash` CHAR(64) NOT NULL DEFAULT '' COMMENT '推流密钥SHA-256哈希',
  `stream_key_version` INT UNSIGNED NOT NULL DEFAULT 1 COMMENT '推流密钥版本',
  `status` TINYINT UNSIGNED NOT NULL DEFAULT 1 COMMENT '0禁用 1正常 2关闭',
  `status_reason` VARCHAR(255) NOT NULL DEFAULT '' COMMENT '禁用或关闭原因',
  `status_changed_at` BIGINT NOT NULL DEFAULT 0 COMMENT '状态变更毫秒时间戳',
  `live_status` TINYINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '0未开播 1准备中 2直播中 3结束中',
  `current_session_id` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '当前场次内部ID，0表示无',
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

-- 为升级前已审核通过但尚无房间的主播补建直播间。
-- 只补缺失记录，不改写已有房间号；房间号和稳定流名称初始均使用主播编号。
INSERT IGNORE INTO `live_room` (
  `created_at`, `updated_at`, `room_no`, `anchor_id`, `category_id`, `title`, `cover_url`, `notice`,
  `stream_name`, `publish_secret_hash`, `stream_key_version`, `status`, `status_reason`, `status_changed_at`,
  `live_status`, `current_session_id`, `live_started_at`, `visibility`, `recommend_weight`
)
SELECT
  NOW(3), NOW(3), a.`anchor_no`, a.`id`, a.`category_id`, CONCAT(a.`nickname`, '的直播间'), a.`cover`, '',
  a.`anchor_no`, '', 1, 1, '', CAST(UNIX_TIMESTAMP(CURRENT_TIMESTAMP(3)) * 1000 AS SIGNED),
  0, 0, 0, 1, 0
FROM `live_anchor` a
LEFT JOIN `live_room` r ON r.`anchor_id` = a.`id` AND r.`deleted_at` IS NULL
WHERE a.`apply_status` = 2 AND a.`deleted_at` IS NULL AND r.`id` IS NULL;

CREATE TABLE IF NOT EXISTS `live_session` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `created_at` DATETIME(3) DEFAULT NULL,
  `updated_at` DATETIME(3) DEFAULT NULL,
  `deleted_at` DATETIME(3) DEFAULT NULL,
  `session_no` VARCHAR(32) NOT NULL COMMENT '直播场次对外编号',
  `room_id` BIGINT UNSIGNED NOT NULL COMMENT '直播间内部ID',
  `anchor_id` BIGINT UNSIGNED NOT NULL COMMENT '主播内部ID快照',
  `category_id` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '本场分类ID快照',
  `title` VARCHAR(128) NOT NULL DEFAULT '' COMMENT '本场标题快照',
  `cover_url` VARCHAR(500) NOT NULL DEFAULT '' COMMENT '本场封面快照',
  `status` TINYINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '0准备中 1直播中 2结束中 3已结束 4已取消 5失败',
  `active_room_id` BIGINT UNSIGNED GENERATED ALWAYS AS (
    CASE WHEN `status` IN (0, 1, 2) THEN `room_id` ELSE NULL END
  ) STORED COMMENT '活动场次房间ID生成列',
  `started_at` BIGINT NOT NULL DEFAULT 0 COMMENT '首次成功发布流毫秒时间戳',
  `ended_at` BIGINT NOT NULL DEFAULT 0 COMMENT '实际最终断流或主动结束毫秒时间戳',
  `duration_ms` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '逻辑直播时长，含成功重连前短暂断流',
  `last_unpublish_at` BIGINT NOT NULL DEFAULT 0 COMMENT '最近一次SRS断流毫秒时间戳',
  `reconnect_deadline_at` BIGINT NOT NULL DEFAULT 0 COMMENT '等待重连截止毫秒时间戳',
  `disconnect_count` INT UNSIGNED NOT NULL DEFAULT 0 COMMENT '本场累计有效断流次数',
  `end_reason` TINYINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '0未知 1主播结束 2管理员结束 3断流超时 4主播封禁 5系统异常',
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
  KEY `idx_live_session_reconnect` (`status`, `reconnect_deadline_at`),
  KEY `idx_live_session_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 若旧 live_anchor 仍为秒字段，请只执行一次以下迁移：
-- ALTER TABLE live_anchor ADD COLUMN total_live_duration_ms BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '累计逻辑直播时长，单位毫秒';
-- UPDATE live_anchor SET total_live_duration_ms = total_live_duration * 1000 WHERE total_live_duration_ms = 0 AND total_live_duration > 0;
-- ALTER TABLE live_anchor DROP COLUMN total_live_duration;
