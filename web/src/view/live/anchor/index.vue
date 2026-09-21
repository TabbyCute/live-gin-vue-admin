<template>
  <div class="anchor-page">
    <div class="anchor-overview">
      <div class="overview-copy">
        <div class="overview-title">主播管理</div>
        <div class="overview-desc">统一处理主播申请、开播权限、封禁、认证与运营属性</div>
      </div>
      <div class="overview-metrics">
        <div class="metric-item">
          <span class="metric-value">{{ total }}</span>
          <span class="metric-label">符合条件</span>
        </div>
        <div class="metric-item">
          <span class="metric-value warning">{{ pageMetrics.pending }}</span>
          <span class="metric-label">本页待审核</span>
        </div>
        <div class="metric-item">
          <span class="metric-value danger">{{ pageMetrics.blocked }}</span>
          <span class="metric-label">本页受限</span>
        </div>
        <div class="metric-item">
          <span class="metric-value success">{{ pageMetrics.liveEnabled }}</span>
          <span class="metric-label">本页可开播</span>
        </div>
      </div>
    </div>

    <div class="gva-search-box">
      <el-form :inline="true" :model="searchInfo" label-width="86px">
        <el-form-item label="主播编号">
          <el-input v-model.trim="searchInfo.anchorNo" clearable placeholder="精确或模糊搜索" />
        </el-form-item>
        <el-form-item label="用户 ID">
          <el-input-number v-model="searchInfo.userId" :min="1" :controls="false" clearable placeholder="请输入用户 ID" />
        </el-form-item>
        <el-form-item label="主播昵称">
          <el-input v-model.trim="searchInfo.nickname" clearable placeholder="请输入昵称" />
        </el-form-item>
        <el-form-item label="申请状态">
          <el-select v-model="searchInfo.applyStatus" clearable placeholder="全部" style="width: 150px">
            <el-option v-for="item in applyStatusOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
        <el-form-item label="账号状态">
          <el-select v-model="searchInfo.status" clearable placeholder="全部" style="width: 150px">
            <el-option v-for="item in anchorStatusOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="onSubmit">查询</el-button>
          <el-button @click="onReset">重置</el-button>
          <el-button link type="primary" @click="advancedVisible = !advancedVisible">
            {{ advancedVisible ? '收起筛选' : '更多筛选' }}
          </el-button>
        </el-form-item>

        <div v-show="advancedVisible" class="advanced-search">
          <el-form-item label="主播 ID">
            <el-input-number v-model="searchInfo.anchorId" :min="1" :controls="false" clearable />
          </el-form-item>
          <el-form-item label="主播类型">
            <el-select v-model="searchInfo.anchorType" clearable placeholder="全部" style="width: 150px">
              <el-option v-for="item in anchorTypeOptions" :key="item.value" :label="item.label" :value="item.value" />
            </el-select>
          </el-form-item>
          <el-form-item label="认证状态">
            <el-select v-model="searchInfo.certStatus" clearable placeholder="全部" style="width: 150px">
              <el-option v-for="item in certStatusOptions" :key="item.value" :label="item.label" :value="item.value" />
            </el-select>
          </el-form-item>
          <el-form-item label="风险等级">
            <el-select v-model="searchInfo.riskLevel" clearable placeholder="全部" style="width: 150px">
              <el-option v-for="item in riskOptions" :key="item.value" :label="item.label" :value="item.value" />
            </el-select>
          </el-form-item>
          <el-form-item label="开播权限">
            <el-select v-model="searchInfo.livePermission" clearable placeholder="全部" style="width: 150px">
              <el-option label="允许" :value="1" />
              <el-option label="禁止" :value="0" />
            </el-select>
          </el-form-item>
          <el-form-item label="是否签约">
            <el-select v-model="searchInfo.isSigned" clearable placeholder="全部" style="width: 150px">
              <el-option label="已签约" :value="1" />
              <el-option label="未签约" :value="0" />
            </el-select>
          </el-form-item>
          <el-form-item label="人工推荐">
            <el-select v-model="searchInfo.isRecommended" clearable placeholder="全部" style="width: 150px">
              <el-option label="已推荐" :value="1" />
              <el-option label="未推荐" :value="0" />
            </el-select>
          </el-form-item>
          <el-form-item label="分类 ID">
            <el-tree-select
              v-model="searchInfo.categoryId"
              :data="categorySearchOptions"
              :props="categoryTreeProps"
              node-key="id"
              check-strictly
              clearable
              filterable
              :render-after-expand="false"
              placeholder="全部分类"
              style="width: 190px"
            />
          </el-form-item>
          <el-form-item label="公会 ID">
            <el-input-number v-model="searchInfo.agencyId" :min="1" :controls="false" clearable />
          </el-form-item>
          <el-form-item label="渠道 ID">
            <el-input-number v-model="searchInfo.channelId" :min="1" :controls="false" clearable />
          </el-form-item>
          <el-form-item label="来源">
            <el-input v-model.trim="searchInfo.source" clearable placeholder="app/admin/import" />
          </el-form-item>
          <el-form-item label="创建日期">
            <el-date-picker
              v-model="searchInfo.createdAtRange"
              type="daterange"
              value-format="YYYY-MM-DD"
              range-separator="至"
              start-placeholder="开始日期"
              end-placeholder="结束日期"
            />
          </el-form-item>
        </div>
      </el-form>
    </div>

    <div class="gva-table-box">
      <div class="gva-btn-list table-toolbar">
        <div class="toolbar-tip">审核通过不会自动开启开播权限，请在“功能权限”中单独设置。</div>
        <el-button :loading="loading" @click="getTableData">刷新</el-button>
      </div>

      <el-table v-loading="loading" :data="tableData" row-key="id" stripe>
        <el-table-column label="主播" min-width="230" fixed="left">
          <template #default="scope">
            <div class="anchor-cell">
              <el-avatar :size="42" :src="scope.row.avatar">
                {{ scope.row.nickname?.slice(0, 1) || '主' }}
              </el-avatar>
              <div class="anchor-cell-main">
                <div class="anchor-name-line">
                  <span class="anchor-name">{{ scope.row.nickname || '-' }}</span>
                  <el-tag v-if="scope.row.isSigned === 1" size="small" effect="plain">签约</el-tag>
                </div>
                <div class="anchor-sub">{{ scope.row.anchorNo }} · UID {{ scope.row.userId }}</div>
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="申请 / 认证" width="150">
          <template #default="scope">
            <div class="stack-tags">
              <el-tag size="small" :type="statusMeta(applyStatusMap, scope.row.applyStatus).type">
                {{ statusMeta(applyStatusMap, scope.row.applyStatus).label }}
              </el-tag>
              <el-tag size="small" effect="plain" :type="statusMeta(certStatusMap, scope.row.certStatus).type">
                {{ statusMeta(certStatusMap, scope.row.certStatus).label }}
              </el-tag>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="账号状态" width="130">
          <template #default="scope">
            <el-tag :type="statusMeta(anchorStatusMap, scope.row.status).type">
              {{ statusMeta(anchorStatusMap, scope.row.status).label }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="功能权限" min-width="210">
          <template #default="scope">
            <div class="permission-list">
              <span :class="['permission-pill', { enabled: scope.row.livePermission === 1 }]">开播</span>
              <span :class="['permission-pill', { enabled: scope.row.pkPermission === 1 }]">PK</span>
              <span :class="['permission-pill', { enabled: scope.row.recommendPermission === 1 }]">推荐</span>
              <span :class="['permission-pill', { enabled: scope.row.withdrawPermission === 1 }]">提现</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="运营属性" min-width="150">
          <template #default="scope">
            <div class="stack-tags">
              <el-tag v-if="scope.row.isRecommended === 1" size="small" type="success">人工推荐</el-tag>
              <el-tag v-else size="small" type="info" effect="plain">未推荐</el-tag>
              <span class="muted-text">类型：{{ statusMeta(anchorTypeMap, scope.row.anchorType).label }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="风控" width="100">
          <template #default="scope">
            <el-tag size="small" :type="statusMeta(riskMap, scope.row.riskLevel).type">
              {{ statusMeta(riskMap, scope.row.riskLevel).label }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="分类 / 公会" min-width="140">
          <template #default="scope">
            <div>{{ categoryLabel(scope.row.categoryId) }}</div>
            <div class="muted-text">公会 {{ scope.row.agencyId || '无' }}</div>
          </template>
        </el-table-column>
        <el-table-column label="来源" width="125">
          <template #default="scope">
            <div>{{ scope.row.source || '-' }}</div>
            <div class="muted-text">渠道 {{ scope.row.channelId || '-' }}</div>
          </template>
        </el-table-column>
        <el-table-column label="创建时间" width="175">
          <template #default="scope">{{ formatDateTime(scope.row.createdAt) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="285" fixed="right">
          <template #default="scope">
            <el-button link type="primary" @click="openDetail(scope.row)">详情</el-button>
            <el-button
              v-if="scope.row.applyStatus === 1"
              link
              type="warning"
              @click="openAction('audit', scope.row)"
            >审核</el-button>
            <el-button link type="primary" @click="openAction('permission', scope.row)">功能权限</el-button>
            <el-dropdown trigger="click" @command="(command) => handleMore(command, scope.row)">
              <el-button link type="primary">更多</el-button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item command="status">账号状态</el-dropdown-item>
                  <el-dropdown-item command="profile">编辑资料</el-dropdown-item>
                  <el-dropdown-item command="cert">认证信息</el-dropdown-item>
                  <el-dropdown-item command="recommend">推荐运营</el-dropdown-item>
                  <el-dropdown-item command="signed">签约状态</el-dropdown-item>
                  <el-dropdown-item command="agency">公会归属</el-dropdown-item>
                  <el-dropdown-item command="risk">风险等级</el-dropdown-item>
                  <el-dropdown-item command="remark">后台备注</el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </template>
        </el-table-column>
      </el-table>

      <div class="gva-pagination">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="pageSize"
          :page-sizes="[10, 20, 50, 100]"
          :total="total"
          layout="total, sizes, prev, pager, next, jumper"
          @current-change="getTableData"
          @size-change="handleSizeChange"
        />
      </div>
    </div>

    <el-drawer v-model="detailVisible" size="72%" destroy-on-close>
      <template #header>
        <div v-if="detail" class="detail-header">
          <el-avatar :size="50" :src="detail.avatar">{{ detail.nickname?.slice(0, 1) || '主' }}</el-avatar>
          <div>
            <div class="detail-title">{{ detail.nickname || '-' }}</div>
            <div class="anchor-sub">{{ detail.anchorNo }} · 主播 ID {{ detail.id }} · 用户 ID {{ detail.userId }}</div>
          </div>
        </div>
        <span v-else>主播详情</span>
      </template>

      <div v-loading="detailLoading" class="detail-body">
        <template v-if="detail">
          <div class="detail-actions">
            <el-button v-if="detail.applyStatus === 1" type="warning" @click="openAction('audit', detail)">审核申请</el-button>
            <el-button type="primary" @click="openAction('permission', detail)">功能权限</el-button>
            <el-button @click="openAction('status', detail)">账号状态</el-button>
            <el-button @click="openAction('profile', detail)">编辑资料</el-button>
          </div>

          <div class="detail-section-title">身份与状态</div>
          <el-descriptions :column="3" border>
            <el-descriptions-item label="申请状态">
              <el-tag :type="statusMeta(applyStatusMap, detail.applyStatus).type">
                {{ statusMeta(applyStatusMap, detail.applyStatus).label }}
              </el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="账号状态">
              <el-tag :type="statusMeta(anchorStatusMap, detail.status).type">
                {{ statusMeta(anchorStatusMap, detail.status).label }}
              </el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="认证状态">
              <el-tag :type="statusMeta(certStatusMap, detail.certStatus).type">
                {{ statusMeta(certStatusMap, detail.certStatus).label }}
              </el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="主播类型">{{ statusMeta(anchorTypeMap, detail.anchorType).label }}</el-descriptions-item>
            <el-descriptions-item label="签约状态">{{ detail.isSigned === 1 ? '已签约' : '未签约' }}</el-descriptions-item>
            <el-descriptions-item label="风险等级">
              <el-tag :type="statusMeta(riskMap, detail.riskLevel).type">
                {{ statusMeta(riskMap, detail.riskLevel).label }}
              </el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="申请时间">{{ formatTimestamp(detail.applyAt) }}</el-descriptions-item>
            <el-descriptions-item label="审核时间">{{ formatTimestamp(detail.auditAt) }}</el-descriptions-item>
            <el-descriptions-item label="审核人 ID">{{ detail.auditUserId || '-' }}</el-descriptions-item>
            <el-descriptions-item label="拒绝原因" :span="3">{{ detail.rejectReason || '-' }}</el-descriptions-item>
            <el-descriptions-item label="状态原因" :span="2">{{ detail.statusReason || '-' }}</el-descriptions-item>
            <el-descriptions-item label="封禁截止">{{ banUntilText(detail) }}</el-descriptions-item>
          </el-descriptions>

          <div class="detail-section-title">功能权限</div>
          <div class="permission-cards">
            <div v-for="item in permissionItems" :key="item.key" class="permission-card">
              <span>{{ item.label }}</span>
              <el-tag :type="detail[item.key] === 1 ? 'success' : 'danger'">
                {{ detail[item.key] === 1 ? '允许' : '禁止' }}
              </el-tag>
            </div>
          </div>

          <div class="detail-section-title">公开资料</div>
          <el-descriptions :column="3" border>
            <el-descriptions-item label="昵称">{{ detail.nickname || '-' }}</el-descriptions-item>
            <el-descriptions-item label="性别">{{ statusMeta(genderMap, detail.gender).label }}</el-descriptions-item>
            <el-descriptions-item label="生日">{{ detail.birthday || '-' }}</el-descriptions-item>
            <el-descriptions-item label="国家">{{ detail.countryCode || '-' }}</el-descriptions-item>
            <el-descriptions-item label="地区 / 城市">{{ detail.regionCode || '-' }} / {{ detail.cityCode || '-' }}</el-descriptions-item>
            <el-descriptions-item label="语言">{{ detail.language || '-' }}</el-descriptions-item>
            <el-descriptions-item label="直播分类">{{ categoryLabel(detail.categoryId) }}</el-descriptions-item>
            <el-descriptions-item label="主播等级">{{ detail.level }}</el-descriptions-item>
            <el-descriptions-item label="标签 ID">{{ detail.tagIds?.length ? detail.tagIds.join(', ') : '-' }}</el-descriptions-item>
            <el-descriptions-item label="头像" :span="3">{{ detail.avatar || '-' }}</el-descriptions-item>
            <el-descriptions-item label="封面" :span="3">{{ detail.cover || '-' }}</el-descriptions-item>
            <el-descriptions-item label="签名" :span="3">{{ detail.signature || '-' }}</el-descriptions-item>
          </el-descriptions>

          <div class="detail-section-title">直播数据</div>
          <div class="stat-grid">
            <div class="stat-card"><strong>{{ detail.fansCount }}</strong><span>粉丝数</span></div>
            <div class="stat-card"><strong>{{ detail.totalLiveCount }}</strong><span>累计场次</span></div>
            <div class="stat-card"><strong>{{ durationText(detail.totalLiveDuration) }}</strong><span>累计时长</span></div>
            <div class="stat-card"><strong>{{ detail.maxOnlineCount }}</strong><span>最高在线</span></div>
            <div class="stat-card"><strong>{{ detail.totalViewCount }}</strong><span>累计观看</span></div>
          </div>
          <el-descriptions :column="2" border>
            <el-descriptions-item label="最近开播">{{ formatTimestamp(detail.lastLiveAt) }}</el-descriptions-item>
            <el-descriptions-item label="最近下播">{{ formatTimestamp(detail.lastLiveEndAt) }}</el-descriptions-item>
          </el-descriptions>

          <div class="detail-section-title">运营与内部信息</div>
          <el-descriptions :column="3" border>
            <el-descriptions-item label="人工推荐">{{ detail.isRecommended === 1 ? '是' : '否' }}</el-descriptions-item>
            <el-descriptions-item label="人工排序">{{ detail.sort }}</el-descriptions-item>
            <el-descriptions-item label="推荐权重">{{ detail.recommendWeight }}</el-descriptions-item>
            <el-descriptions-item label="新人期截止">{{ formatTimestamp(detail.newcomerUntil) }}</el-descriptions-item>
            <el-descriptions-item label="公会 ID">{{ detail.agencyId || '无公会' }}</el-descriptions-item>
            <el-descriptions-item label="加入公会时间">{{ formatTimestamp(detail.agencyJoinAt) }}</el-descriptions-item>
            <el-descriptions-item label="认证类型">{{ statusMeta(certTypeMap, detail.certType).label }}</el-descriptions-item>
            <el-descriptions-item label="认证名称">{{ detail.certName || '-' }}</el-descriptions-item>
            <el-descriptions-item label="来源">{{ detail.source || '-' }}</el-descriptions-item>
            <el-descriptions-item label="来源 ID">{{ detail.sourceId || '-' }}</el-descriptions-item>
            <el-descriptions-item label="渠道 ID">{{ detail.channelId || '-' }}</el-descriptions-item>
            <el-descriptions-item label="创建时间">{{ formatDateTime(detail.createdAt) }}</el-descriptions-item>
            <el-descriptions-item label="后台备注" :span="3">{{ detail.remark || '-' }}</el-descriptions-item>
            <el-descriptions-item label="扩展信息" :span="3"><pre class="extra-json">{{ formatExtra(detail.extra) }}</pre></el-descriptions-item>
          </el-descriptions>
        </template>
      </div>
    </el-drawer>

    <el-dialog
      v-model="actionVisible"
      :title="actionTitle"
      :width="actionType === 'profile' ? '780px' : '580px'"
      destroy-on-close
      :close-on-click-modal="false"
    >
      <div v-loading="actionLoading">
        <div v-if="actionDetail" class="dialog-anchor-summary">
          <el-avatar :size="36" :src="actionDetail.avatar">{{ actionDetail.nickname?.slice(0, 1) || '主' }}</el-avatar>
          <div>
            <div>{{ actionDetail.nickname || '-' }}</div>
            <div class="anchor-sub">{{ actionDetail.anchorNo }} · 主播 ID {{ actionDetail.id }}</div>
          </div>
        </div>

        <el-form v-if="!actionLoading" :model="actionForm" label-width="112px" @submit.prevent>
          <template v-if="actionType === 'audit'">
            <el-alert class="form-alert" type="info" :closable="false" show-icon title="审核通过只改变申请状态，不会自动开放开播、PK 或提现权限。" />
            <el-form-item label="审核结果" required>
              <el-radio-group v-model="actionForm.applyStatus">
                <el-radio-button :value="2">通过</el-radio-button>
                <el-radio-button :value="3">拒绝</el-radio-button>
              </el-radio-group>
            </el-form-item>
            <el-form-item v-if="actionForm.applyStatus === 3" label="拒绝原因" required>
              <el-input v-model.trim="actionForm.rejectReason" type="textarea" :rows="4" maxlength="255" show-word-limit />
            </el-form-item>
          </template>

          <template v-else-if="actionType === 'status'">
            <el-form-item label="账号状态" required>
              <el-select v-model="actionForm.status" style="width: 100%" :disabled="actionDetail?.status === 3">
                <el-option v-for="item in anchorStatusOptions" :key="item.value" :label="item.label" :value="item.value" />
              </el-select>
            </el-form-item>
            <el-form-item v-if="[0, 2].includes(actionForm.status)" label="状态原因" required>
              <el-input v-model.trim="actionForm.statusReason" type="textarea" :rows="3" maxlength="255" show-word-limit />
            </el-form-item>
            <template v-if="actionForm.status === 2">
              <el-form-item label="永久封禁">
                <el-switch v-model="actionForm.banForever" />
              </el-form-item>
              <el-form-item v-if="!actionForm.banForever" label="封禁截止" required>
                <el-date-picker v-model="actionForm.banUntil" type="datetime" value-format="x" placeholder="请选择未来时间" style="width: 100%" />
              </el-form-item>
            </template>
            <el-alert v-if="actionForm.status === 3" class="form-alert" type="error" :closable="false" show-icon title="注销后不能通过普通状态接口恢复，请谨慎操作。" />
          </template>

          <template v-else-if="actionType === 'permission'">
            <el-alert class="form-alert" type="warning" :closable="false" show-icon title="关闭开播权限时必须同时关闭 PK 权限。推荐准入与人工推荐是两个不同字段。" />
            <el-form-item label="开播权限">
              <el-switch v-model="actionForm.livePermission" :active-value="1" :inactive-value="0" active-text="允许" inactive-text="禁止" />
            </el-form-item>
            <el-form-item label="PK 权限">
              <el-switch v-model="actionForm.pkPermission" :active-value="1" :inactive-value="0" active-text="允许" inactive-text="禁止" />
            </el-form-item>
            <el-form-item label="推荐准入">
              <el-switch v-model="actionForm.recommendPermission" :active-value="1" :inactive-value="0" active-text="允许" inactive-text="禁止" />
            </el-form-item>
            <el-form-item label="提现权限">
              <el-switch v-model="actionForm.withdrawPermission" :active-value="1" :inactive-value="0" active-text="允许" inactive-text="禁止" />
            </el-form-item>
          </template>

          <template v-else-if="actionType === 'profile'">
            <el-row :gutter="18">
              <el-col :span="12"><el-form-item label="昵称" required><el-input v-model.trim="actionForm.nickname" maxlength="64" /></el-form-item></el-col>
              <el-col :span="12"><el-form-item label="主播类型"><el-select v-model="actionForm.anchorType" style="width: 100%"><el-option v-for="item in anchorTypeOptions" :key="item.value" :label="item.label" :value="item.value" /></el-select></el-form-item></el-col>
              <el-col :span="12"><el-form-item label="性别"><el-select v-model="actionForm.gender" style="width: 100%"><el-option v-for="item in genderOptions" :key="item.value" :label="item.label" :value="item.value" /></el-select></el-form-item></el-col>
              <el-col :span="12"><el-form-item label="生日"><el-date-picker v-model="actionForm.birthday" type="date" value-format="YYYY-MM-DD" clearable style="width: 100%" /></el-form-item></el-col>
              <el-col :span="12">
                <el-form-item label="直播分类">
                  <el-tree-select
                    v-model="actionForm.categoryId"
                    :data="categoryEditOptions"
                    :props="categoryTreeProps"
                    node-key="id"
                    check-strictly
                    clearable
                    filterable
                    :render-after-expand="false"
                    placeholder="未分类"
                    style="width: 100%"
                  />
                </el-form-item>
              </el-col>
              <el-col :span="12"><el-form-item label="语言"><el-input v-model.trim="actionForm.language" maxlength="16" placeholder="zh-CN / en-US" /></el-form-item></el-col>
              <el-col :span="12"><el-form-item label="国家代码"><el-input v-model.trim="actionForm.countryCode" maxlength="2" placeholder="CN / MY / TH" /></el-form-item></el-col>
              <el-col :span="12"><el-form-item label="地区代码"><el-input v-model.trim="actionForm.regionCode" maxlength="32" /></el-form-item></el-col>
              <el-col :span="12"><el-form-item label="城市代码"><el-input v-model.trim="actionForm.cityCode" maxlength="32" /></el-form-item></el-col>
              <el-col :span="12"><el-form-item label="标签 ID"><el-input v-model.trim="actionForm.tagIdsText" placeholder="多个 ID 用英文逗号分隔" /></el-form-item></el-col>
              <el-col :span="24"><el-form-item label="头像地址"><el-input v-model.trim="actionForm.avatar" maxlength="500" /></el-form-item></el-col>
              <el-col :span="24"><el-form-item label="封面地址"><el-input v-model.trim="actionForm.cover" maxlength="500" /></el-form-item></el-col>
              <el-col :span="24"><el-form-item label="签名"><el-input v-model="actionForm.signature" type="textarea" :rows="3" maxlength="255" show-word-limit /></el-form-item></el-col>
            </el-row>
          </template>

          <template v-else-if="actionType === 'recommend'">
            <el-alert v-if="actionDetail?.recommendPermission === 0" class="form-alert" type="warning" :closable="false" show-icon title="当前推荐准入权限已关闭，不能开启人工推荐。" />
            <el-form-item label="人工推荐"><el-switch v-model="actionForm.isRecommended" :active-value="1" :inactive-value="0" /></el-form-item>
            <el-form-item label="新人期截止"><el-date-picker v-model="actionForm.newcomerUntil" type="datetime" value-format="x" clearable placeholder="留空表示非新人" style="width: 100%" /></el-form-item>
            <el-form-item label="人工排序"><el-input-number v-model="actionForm.sort" :controls="false" style="width: 100%" /></el-form-item>
            <el-form-item label="推荐基础权重"><el-input-number v-model="actionForm.recommendWeight" :controls="false" style="width: 100%" /></el-form-item>
          </template>

          <template v-else-if="actionType === 'signed'">
            <el-form-item label="签约状态"><el-radio-group v-model="actionForm.isSigned"><el-radio-button :value="1">已签约</el-radio-button><el-radio-button :value="0">未签约</el-radio-button></el-radio-group></el-form-item>
            <el-alert class="form-alert" type="info" :closable="false" show-icon title="签约状态不会联动主播类型、公会归属或认证状态。" />
          </template>

          <template v-else-if="actionType === 'agency'">
            <el-form-item label="公会 ID"><el-input-number v-model="actionForm.agencyId" :min="0" :controls="false" style="width: 100%" /></el-form-item>
            <el-alert class="form-alert" type="info" :closable="false" show-icon title="填写 0 表示移出公会。当前版本只记录公会 ID，不校验公会是否存在。" />
          </template>

          <template v-else-if="actionType === 'risk'">
            <el-form-item label="风险等级"><el-select v-model="actionForm.riskLevel" style="width: 100%"><el-option v-for="item in riskOptions" :key="item.value" :label="item.label" :value="item.value" /></el-select></el-form-item>
            <el-alert class="form-alert" :type="actionForm.riskLevel === 3 ? 'error' : 'info'" :closable="false" show-icon title="高风险主播会被实时开播资格检查拦截；中风险当前不自动扩大限制。" />
          </template>

          <template v-else-if="actionType === 'remark'">
            <el-form-item label="后台备注"><el-input v-model="actionForm.remark" type="textarea" :rows="6" maxlength="500" show-word-limit placeholder="仅管理后台可见" /></el-form-item>
          </template>

          <template v-else-if="actionType === 'cert'">
            <el-form-item label="认证状态"><el-select v-model="actionForm.certStatus" style="width: 100%"><el-option v-for="item in certStatusOptions" :key="item.value" :label="item.label" :value="item.value" /></el-select></el-form-item>
            <el-form-item label="认证类型"><el-select v-model="actionForm.certType" style="width: 100%"><el-option v-for="item in certTypeOptions" :key="item.value" :label="item.label" :value="item.value" /></el-select></el-form-item>
            <el-form-item label="认证展示名称" :required="actionForm.certStatus === 2"><el-input v-model.trim="actionForm.certName" maxlength="64" placeholder="例如：官方主播 / 签约主播" /></el-form-item>
            <el-alert class="form-alert" type="info" :closable="false" show-icon title="认证信息只用于展示；第一版认证不是开播的硬条件。" />
          </template>
        </el-form>
      </div>

      <template #footer>
        <el-button @click="actionVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitLoading" :disabled="actionLoading" @click="submitAction">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { formatDate } from '@/utils/format'
import {
  auditAnchor,
  getAnchorDetail,
  getAnchorList,
  updateAnchorAgency,
  updateAnchorCert,
  updateAnchorPermission,
  updateAnchorProfile,
  updateAnchorRecommend,
  updateAnchorRemark,
  updateAnchorRisk,
  updateAnchorSigned,
  updateAnchorStatus
} from '@/api/live-anchor'
import { getCategoryTree } from '@/api/live-category'

const applyStatusMap = { 0: { label: '未申请', type: 'info' }, 1: { label: '待审核', type: 'warning' }, 2: { label: '已通过', type: 'success' }, 3: { label: '已拒绝', type: 'danger' } }
const anchorStatusMap = { 0: { label: '已禁用', type: 'info' }, 1: { label: '正常', type: 'success' }, 2: { label: '已封禁', type: 'danger' }, 3: { label: '已注销', type: 'info' } }
const certStatusMap = { 0: { label: '未认证', type: 'info' }, 1: { label: '认证中', type: 'warning' }, 2: { label: '已认证', type: 'success' }, 3: { label: '认证失败', type: 'danger' } }
const anchorTypeMap = { 0: { label: '普通主播', type: 'info' }, 1: { label: '官方主播', type: 'success' }, 2: { label: '内部运营', type: 'warning' } }
const riskMap = { 0: { label: '正常', type: 'success' }, 1: { label: '低风险', type: 'info' }, 2: { label: '中风险', type: 'warning' }, 3: { label: '高风险', type: 'danger' } }
const genderMap = { 0: { label: '未知', type: 'info' }, 1: { label: '男', type: 'info' }, 2: { label: '女', type: 'info' }, 3: { label: '其他', type: 'info' } }
const certTypeMap = { 0: { label: '无', type: 'info' }, 1: { label: '个人', type: 'info' }, 2: { label: '机构', type: 'info' }, 3: { label: '官方', type: 'info' } }

const toOptions = (map) => Object.entries(map).map(([value, meta]) => ({ value: Number(value), label: meta.label }))
const applyStatusOptions = toOptions(applyStatusMap)
const anchorStatusOptions = toOptions(anchorStatusMap)
const certStatusOptions = toOptions(certStatusMap)
const anchorTypeOptions = toOptions(anchorTypeMap)
const riskOptions = toOptions(riskMap)
const genderOptions = toOptions(genderMap)
const certTypeOptions = toOptions(certTypeMap)
const permissionItems = [
  { key: 'livePermission', label: '开播权限' },
  { key: 'pkPermission', label: 'PK 权限' },
  { key: 'recommendPermission', label: '推荐准入' },
  { key: 'withdrawPermission', label: '提现权限' }
]

const defaultSearchInfo = () => ({
  anchorId: null, anchorNo: '', userId: null, nickname: '', anchorType: null, categoryId: null,
  agencyId: null, applyStatus: null, certStatus: null, status: null, livePermission: null,
  isSigned: null, isRecommended: null, riskLevel: null, source: '', channelId: null, createdAtRange: []
})

const searchInfo = reactive(defaultSearchInfo())
const advancedVisible = ref(false)
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const tableData = ref([])
const categoryTree = ref([])
const loading = ref(false)
const detailVisible = ref(false)
const detailLoading = ref(false)
const detail = ref(null)
const actionVisible = ref(false)
const actionLoading = ref(false)
const submitLoading = ref(false)
const actionType = ref('')
const actionDetail = ref(null)
const actionForm = reactive({})

const decorateCategoryTree = (items, disableInactive) => items.map((item) => ({
  ...item,
  label: `${item.name}（${item.code}）${item.status === 0 ? ' · 已停用' : ''}`,
  disabled: disableInactive && item.status !== 1,
  children: decorateCategoryTree(item.children || [], disableInactive)
}))
const categorySearchOptions = computed(() => decorateCategoryTree(categoryTree.value, false))
const categoryEditOptions = computed(() => decorateCategoryTree(categoryTree.value, true))
const categoryTreeProps = { label: 'label', children: 'children', disabled: 'disabled', value: 'id' }
const categoryNameMap = computed(() => {
  const result = new Map()
  const visit = (items) => items.forEach((item) => {
    result.set(Number(item.id), item.name)
    visit(item.children || [])
  })
  visit(categoryTree.value)
  return result
})
const categoryLabel = (categoryId) => {
  const id = Number(categoryId || 0)
  if (!id) return '未分类'
  return categoryNameMap.value.get(id) || `分类 ID ${id}`
}

const actionTitles = {
  audit: '审核主播申请', status: '修改账号状态', permission: '修改功能权限', profile: '编辑主播资料',
  recommend: '设置推荐运营', signed: '修改签约状态', agency: '修改公会归属', risk: '修改风险等级',
  remark: '编辑后台备注', cert: '修改认证信息'
}
const actionTitle = computed(() => actionTitles[actionType.value] || '主播操作')
const pageMetrics = computed(() => ({
  pending: tableData.value.filter((item) => item.applyStatus === 1).length,
  blocked: tableData.value.filter((item) => [0, 2, 3].includes(item.status)).length,
  liveEnabled: tableData.value.filter((item) => item.status === 1 && item.applyStatus === 2 && item.livePermission === 1 && item.riskLevel !== 3).length
}))

const statusMeta = (map, value) => map[value] || { label: `未知(${value})`, type: 'info' }
const formatDateTime = (value) => value ? (formatDate(value) || '-') : '-'
const formatTimestamp = (value) => Number(value) ? (formatDate(Number(value)) || '-') : '-'
const durationText = (seconds) => {
  const value = Number(seconds || 0)
  if (value < 60) return `${value} 秒`
  if (value < 3600) return `${Math.floor(value / 60)} 分钟`
  return `${Math.floor(value / 3600)} 小时 ${Math.floor((value % 3600) / 60)} 分钟`
}
const formatExtra = (value) => {
  if (!value) return '-'
  try { return JSON.stringify(typeof value === 'string' ? JSON.parse(value) : value, null, 2) } catch { return String(value) }
}
const banUntilText = (anchor) => anchor.status !== 2 ? '-' : (Number(anchor.banUntil) === 0 ? '永久封禁' : formatTimestamp(anchor.banUntil))

const buildSearchParams = () => {
  const params = { page: page.value, pageSize: pageSize.value }
  Object.entries(searchInfo).forEach(([key, value]) => {
    if (key !== 'createdAtRange' && value !== '' && value !== null && value !== undefined) params[key] = value
  })
  if (searchInfo.createdAtRange?.length === 2) {
    params.createdAtStart = searchInfo.createdAtRange[0]
    params.createdAtEnd = searchInfo.createdAtRange[1]
  }
  return params
}

const getTableData = async () => {
  loading.value = true
  try {
    const res = await getAnchorList(buildSearchParams())
    if (res.code === 0) {
      tableData.value = res.data.list || []
      total.value = Number(res.data.total || 0)
      page.value = Number(res.data.page || page.value)
      pageSize.value = Number(res.data.pageSize || pageSize.value)
    }
  } finally { loading.value = false }
}
const getCategoryData = async () => {
  const res = await getCategoryTree()
  if (res.code === 0) categoryTree.value = res.data || []
}
const onSubmit = () => { page.value = 1; getTableData() }
const onReset = () => { Object.assign(searchInfo, defaultSearchInfo()); page.value = 1; getTableData() }
const handleSizeChange = () => { page.value = 1; getTableData() }
const fetchDetail = async (anchorId) => {
  const res = await getAnchorDetail({ anchorId })
  return res.code === 0 ? res.data : null
}
const openDetail = async (row) => {
  detailVisible.value = true
  detailLoading.value = true
  detail.value = null
  try { detail.value = await fetchDetail(row.id) } finally { detailLoading.value = false }
}

const initialActionForm = (type, data) => {
  const base = { anchorId: data.id }
  const forms = {
    audit: { ...base, applyStatus: 2, rejectReason: '' },
    status: { ...base, status: data.status, statusReason: data.statusReason || '', banUntil: data.banUntil ? String(data.banUntil) : null, banForever: data.status === 2 && Number(data.banUntil) === 0 },
    permission: { ...base, livePermission: data.livePermission, pkPermission: data.pkPermission, recommendPermission: data.recommendPermission, withdrawPermission: data.withdrawPermission },
    profile: {
      ...base, nickname: data.nickname || '', avatar: data.avatar || '', cover: data.cover || '', signature: data.signature || '',
      gender: data.gender, birthday: data.birthday || null, countryCode: data.countryCode || '', regionCode: data.regionCode || '',
      cityCode: data.cityCode || '', language: data.language || '', anchorType: data.anchorType, categoryId: data.categoryId,
      tagIdsText: data.tagIds?.join(',') || ''
    },
    recommend: { ...base, isRecommended: data.isRecommended, newcomerUntil: data.newcomerUntil ? String(data.newcomerUntil) : null, sort: data.sort, recommendWeight: data.recommendWeight },
    signed: { ...base, isSigned: data.isSigned }, agency: { ...base, agencyId: data.agencyId },
    risk: { ...base, riskLevel: data.riskLevel }, remark: { ...base, remark: data.remark || '' },
    cert: { ...base, certStatus: data.certStatus, certType: data.certType, certName: data.certName || '' }
  }
  return forms[type] || base
}

const openAction = async (type, row) => {
  actionType.value = type
  actionVisible.value = true
  actionLoading.value = true
  actionDetail.value = null
  Object.keys(actionForm).forEach((key) => delete actionForm[key])
  try {
    const latest = await fetchDetail(row.id)
    if (!latest) { actionVisible.value = false; return }
    actionDetail.value = latest
    Object.assign(actionForm, initialActionForm(type, latest))
  } finally { actionLoading.value = false }
}
const handleMore = (command, row) => openAction(command, row)
const parseTagIds = (value) => {
  if (!value?.trim()) return []
  const items = value.split(',').map((item) => Number(item.trim()))
  if (items.some((item) => !Number.isInteger(item) || item <= 0)) return null
  return [...new Set(items)]
}

const validateAction = () => {
  if (actionType.value === 'audit' && actionForm.applyStatus === 3 && !actionForm.rejectReason) return '拒绝申请时必须填写拒绝原因'
  if (actionType.value === 'status') {
    if (actionDetail.value?.status === 3) return '已注销主播不能通过普通状态接口恢复'
    if ([0, 2].includes(actionForm.status) && !actionForm.statusReason) return '禁用或封禁时必须填写原因'
    if (actionForm.status === 2 && !actionForm.banForever && (!actionForm.banUntil || Number(actionForm.banUntil) <= Date.now())) return '临时封禁必须选择未来的截止时间'
  }
  if (actionType.value === 'permission' && actionForm.livePermission === 0 && actionForm.pkPermission === 1) return '禁止开播时不能开启 PK 权限'
  if (actionType.value === 'profile') {
    if (!actionForm.nickname) return '主播昵称不能为空'
    if (actionForm.countryCode && !/^[A-Za-z]{2}$/.test(actionForm.countryCode)) return '国家代码必须是两个英文字母'
    const tagIds = parseTagIds(actionForm.tagIdsText)
    if (tagIds === null) return '标签 ID 必须是用英文逗号分隔的正整数'
    if (tagIds.length > 100) return '标签 ID 最多填写 100 个'
  }
  if (actionType.value === 'recommend' && actionForm.isRecommended === 1 && actionDetail.value?.recommendPermission === 0) return '请先开启推荐准入权限，再设置人工推荐'
  if (actionType.value === 'cert') {
    if (actionForm.certStatus === 0 && (actionForm.certType !== 0 || actionForm.certName)) return '未认证状态必须选择“无”认证类型并清空展示名称'
    if ([1, 2].includes(actionForm.certStatus) && actionForm.certType === 0) return '认证中或已认证必须选择认证类型'
    if (actionForm.certStatus === 2 && !actionForm.certName) return '已认证必须填写认证展示名称'
  }
  return ''
}

const confirmSensitiveAction = async () => {
  let message = ''
  if (actionType.value === 'audit' && actionForm.applyStatus === 3) message = '确认拒绝该主播申请吗？'
  if (actionType.value === 'status' && actionForm.status === 0) message = '确认禁用该主播账号吗？禁用后将无法开播。'
  if (actionType.value === 'status' && actionForm.status === 2) message = actionForm.banForever ? '确认永久封禁该主播吗？' : '确认临时封禁该主播吗？'
  if (actionType.value === 'status' && actionForm.status === 3) message = '确认注销该主播身份吗？注销后不能通过普通状态接口恢复。'
  if (!message) return true
  try {
    await ElMessageBox.confirm(message, '敏感操作确认', { confirmButtonText: '确认执行', cancelButtonText: '取消', type: 'warning' })
    return true
  } catch { return false }
}

const buildActionPayload = () => {
  const anchorId = Number(actionForm.anchorId)
  const payloads = {
    audit: { anchorId, applyStatus: actionForm.applyStatus, rejectReason: actionForm.applyStatus === 3 ? actionForm.rejectReason : '' },
    status: { anchorId, status: actionForm.status, statusReason: [0, 2].includes(actionForm.status) ? actionForm.statusReason : '', banUntil: actionForm.status === 2 && !actionForm.banForever ? Number(actionForm.banUntil) : 0 },
    permission: { anchorId, livePermission: actionForm.livePermission, pkPermission: actionForm.pkPermission, recommendPermission: actionForm.recommendPermission, withdrawPermission: actionForm.withdrawPermission },
    profile: {
      anchorId, nickname: actionForm.nickname, avatar: actionForm.avatar || '', cover: actionForm.cover || '', signature: actionForm.signature || '',
      gender: Number(actionForm.gender), birthday: actionForm.birthday || null, countryCode: (actionForm.countryCode || '').toUpperCase(),
      regionCode: actionForm.regionCode || '', cityCode: actionForm.cityCode || '', language: actionForm.language || '',
      anchorType: Number(actionForm.anchorType), categoryId: Number(actionForm.categoryId || 0), tagIds: parseTagIds(actionForm.tagIdsText)
    },
    recommend: { anchorId, isRecommended: actionForm.isRecommended, newcomerUntil: Number(actionForm.newcomerUntil || 0), sort: Number(actionForm.sort || 0), recommendWeight: Number(actionForm.recommendWeight || 0) },
    signed: { anchorId, isSigned: actionForm.isSigned }, agency: { anchorId, agencyId: Number(actionForm.agencyId || 0) },
    risk: { anchorId, riskLevel: actionForm.riskLevel }, remark: { anchorId, remark: actionForm.remark || '' },
    cert: { anchorId, certStatus: actionForm.certStatus, certType: actionForm.certType, certName: actionForm.certName || '' }
  }
  return payloads[actionType.value]
}

const actionApis = {
  audit: auditAnchor, status: updateAnchorStatus, permission: updateAnchorPermission, profile: updateAnchorProfile,
  recommend: updateAnchorRecommend, signed: updateAnchorSigned, agency: updateAnchorAgency, risk: updateAnchorRisk,
  remark: updateAnchorRemark, cert: updateAnchorCert
}
const submitAction = async () => {
  const message = validateAction()
  if (message) { ElMessage.warning(message); return }
  if (!(await confirmSensitiveAction())) return
  const api = actionApis[actionType.value]
  if (!api) return
  submitLoading.value = true
  try {
    const res = await api(buildActionPayload())
    if (res.code === 0) {
      ElMessage.success(res.msg || '保存成功')
      actionVisible.value = false
      await getTableData()
      if (detailVisible.value && detail.value?.id === actionDetail.value?.id) detail.value = await fetchDetail(detail.value.id)
    }
  } finally { submitLoading.value = false }
}

onMounted(() => Promise.all([getTableData(), getCategoryData()]))
</script>

<style scoped lang="scss">
.anchor-page {
  .anchor-overview { display: flex; align-items: center; justify-content: space-between; gap: 24px; padding: 22px 24px; color: #fff; background: linear-gradient(120deg, #172554 0%, #1d4ed8 58%, #0ea5e9 100%); border-radius: 10px; }
  .overview-title { font-size: 22px; font-weight: 700; }
  .overview-desc { margin-top: 7px; font-size: 13px; color: rgb(255 255 255 / 72%); }
  .overview-metrics { display: flex; gap: 10px; }
  .metric-item { display: flex; flex-direction: column; min-width: 96px; padding: 10px 16px; background: rgb(255 255 255 / 10%); border: 1px solid rgb(255 255 255 / 14%); border-radius: 8px; }
  .metric-value { font-size: 22px; font-weight: 700; &.warning { color: #fde68a; } &.danger { color: #fecaca; } &.success { color: #bbf7d0; } }
  .metric-label { margin-top: 3px; font-size: 12px; color: rgb(255 255 255 / 68%); }
  .advanced-search { padding-top: 4px; border-top: 1px dashed var(--el-border-color-lighter); }
  .table-toolbar { justify-content: space-between; }
  .toolbar-tip, .muted-text, .anchor-sub { color: var(--el-text-color-secondary); font-size: 12px; }
  .anchor-cell, .detail-header, .dialog-anchor-summary { display: flex; align-items: center; gap: 12px; }
  .anchor-cell-main { min-width: 0; }
  .anchor-name-line { display: flex; align-items: center; gap: 6px; }
  .anchor-name { max-width: 128px; overflow: hidden; font-weight: 600; text-overflow: ellipsis; white-space: nowrap; }
  .stack-tags { display: flex; align-items: flex-start; flex-direction: column; gap: 5px; }
  .permission-list { display: flex; flex-wrap: wrap; gap: 5px; }
  .permission-pill { padding: 2px 7px; color: var(--el-text-color-secondary); font-size: 12px; background: var(--el-fill-color-light); border-radius: 999px; &.enabled { color: var(--el-color-success-dark-2); background: var(--el-color-success-light-9); } }
  .detail-title { font-size: 18px; font-weight: 700; }
  .detail-body { min-height: 300px; }
  .detail-actions { display: flex; flex-wrap: wrap; gap: 8px; margin-bottom: 20px; }
  .detail-section-title { margin: 24px 0 10px; padding-left: 10px; font-weight: 700; border-left: 3px solid var(--el-color-primary); }
  .permission-cards, .stat-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 10px; }
  .permission-card, .stat-card { display: flex; align-items: center; justify-content: space-between; padding: 14px 16px; background: var(--el-fill-color-light); border-radius: 7px; }
  .stat-grid { grid-template-columns: repeat(5, minmax(0, 1fr)); margin-bottom: 10px; }
  .stat-card { align-items: flex-start; flex-direction: column; strong { font-size: 18px; } span { margin-top: 4px; color: var(--el-text-color-secondary); font-size: 12px; } }
  .extra-json { max-height: 180px; margin: 0; overflow: auto; white-space: pre-wrap; word-break: break-all; }
  .dialog-anchor-summary { padding: 10px 12px; margin-bottom: 18px; background: var(--el-fill-color-light); border-radius: 7px; }
  .form-alert { margin-bottom: 18px; }
}

@media (max-width: 1100px) {
  .anchor-page {
    .anchor-overview { align-items: flex-start; flex-direction: column; }
    .overview-metrics { width: 100%; overflow-x: auto; }
    .metric-item { flex: 1; }
    .permission-cards { grid-template-columns: repeat(2, minmax(0, 1fr)); }
    .stat-grid { grid-template-columns: repeat(3, minmax(0, 1fr)); }
  }
}
</style>
