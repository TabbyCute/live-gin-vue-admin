<template>
  <div class="wallet-guide">
    <section class="guide-hero">
      <div>
        <div class="guide-kicker">操作前请先读这里</div>
        <h1>钱包介绍</h1>
        <p>这是一份用简单话写的后台钱包说明。第一次操作时，请从上往下看一遍；真正充值前，再照着“人工充值六步法”逐项核对。</p>
      </div>
      <el-tag size="large" type="warning" effect="dark">钱款操作不可凭感觉处理</el-tag>
    </section>

    <el-alert class="guide-alert" type="error" :closable="false" show-icon>
      <template #title>最重要的一句话：没有看到真实收款凭证、没有确认钱已到账，就不要提交充值，更不要审核通过。</template>
    </el-alert>

    <section class="guide-section">
      <h2>先记住这 6 件事</h2>
      <div class="rule-grid">
        <article v-for="item in keyRules" :key="item.title" class="rule-card">
          <span>{{ item.no }}</span><div><h3>{{ item.title }}</h3><p>{{ item.text }}</p></div>
        </article>
      </div>
    </section>

    <section class="guide-section">
      <h2>钱包里的几个词是什么意思</h2>
      <el-table :data="terms" border stripe>
        <el-table-column prop="name" label="页面上的词" width="150" />
        <el-table-column prop="plain" label="简单解释" min-width="280" />
        <el-table-column prop="attention" label="要注意什么" min-width="320" />
      </el-table>
    </section>

    <section class="guide-section">
      <h2>一笔人工充值是怎样完成的</h2>
      <div class="flow-list">
        <div v-for="step in flow" :key="step.no" class="flow-step">
          <div class="flow-no">{{ step.no }}</div>
          <div><h3>{{ step.title }}</h3><p>{{ step.text }}</p><el-tag v-if="step.warning" type="warning" effect="plain">{{ step.warning }}</el-tag></div>
        </div>
      </div>
      <el-alert type="info" :closable="false" show-icon title="普通管理员采用申请人与审核人分开复核；固定顶级超级管理员 ID 1 可以自审，但系统仍会记录完整的管理员、IP、时间、原因和流水。" />
    </section>

    <section class="guide-section">
      <h2>人工充值六步法</h2>
      <el-steps direction="vertical" :active="6" finish-status="success">
        <el-step v-for="step in rechargeSteps" :key="step.title" :title="step.title" :description="step.description" />
      </el-steps>
      <div class="danger-box">
        <h3>下面这些情况必须停止操作</h3>
        <ul>
          <li>用户说“我已经付钱”，但后台人员没有看到收款账户真实到账。</li>
          <li>资金凭证号、到账金额、用户账号三者有任何一个对不上。</li>
          <li>有人要求用新的凭证号绕过“凭证已存在”的提示。</li>
          <li>审核页面显示的钱包、金额或凭证与申请材料不一致。</li>
          <li>页面报错、网络中断或一直转圈时，不要换请求编号重复提交；先按原凭证号查询。</li>
        </ul>
      </div>
    </section>

    <section class="guide-section">
      <h2>三个业务页面怎么用</h2>
      <el-collapse v-model="opened" accordion>
        <el-collapse-item v-for="page in pages" :key="page.name" :name="page.name" :title="page.title">
          <div class="page-help"><p><strong>什么时候用：</strong>{{ page.when }}</p><p><strong>怎么查询：</strong>{{ page.search }}</p><p><strong>能做什么：</strong>{{ page.actions }}</p><p><strong>不能做什么：</strong>{{ page.never }}</p></div>
        </el-collapse-item>
      </el-collapse>
    </section>

    <section class="guide-section">
      <h2>钱包状态、冲正和对账</h2>
      <div class="explain-grid">
        <article><h3>停用钱包</h3><p>怀疑风险时可以停用。停用后，充值审核入账和冲正都会被阻止，但原余额和历史流水不会消失。没有开户的钱包不能通过状态按钮自动创建。</p></article>
        <article><h3>冲正</h3><p>冲正用于撤回一笔已经入账的错误充值。它不是删除原流水，而是新增一条相反金额的流水。只能全额冲正，余额不足或钱包停用时不能完成。</p></article>
        <article><h3>完整对账</h3><p>对账只检查，不会自动修改钱。它会核对钱包余额、每一条流水、充值单和审核人是否一致。出现红色异常时，应先停用钱包并联系技术人员。</p></article>
      </div>
    </section>

    <section class="guide-section">
      <h2>常见问题</h2>
      <el-collapse>
        <el-collapse-item v-for="item in questions" :key="item.q" :title="item.q"><p>{{ item.a }}</p></el-collapse-item>
      </el-collapse>
    </section>

    <section class="guide-section final-check">
      <h2>提交或审核前，最后读一遍</h2>
      <p>我已经确认：用户账号正确、完整手机号仅用于帮助找人、真实资金已经到账、凭证号来自真实交易、金额完全一致、凭证材料可查看、原因写清楚。任何一项不能确认，就停止操作。</p>
    </section>
  </div>
</template>

<script setup>
import { ref } from 'vue'

defineOptions({ name: 'WalletIntroduction' })

const opened = ref('account')
const keyRules = [
  { no: '1', title: '查询不会开户', text: '查看余额或流水只是读数据，不会给用户创建钱包。未开户会显示 0。' },
  { no: '2', title: '余额不能手改', text: '余额只能由审核成功的充值或冲正流水改变，不能直接改数据库里的数字。' },
  { no: '3', title: '凭证永久防重复', text: '同一真实资金凭证只能使用一次，换管理员、换用户或换请求编号也不能再次入账。' },
  { no: '4', title: '流水不能删除', text: '成功入账后会留下永久流水。发现错误要走冲正，不是删除或覆盖原记录。' },
  { no: '5', title: '手机号只帮助找人', text: '手机号可以换绑，所以账务最终绑定用户内部ID和公开账号，不把手机号当永久账务编号。' },
  { no: '6', title: '金额是整数 COIN', text: '当前不接受小数、负数或带逗号的金额。页面显示的大数字仍按字符串安全处理。' }
]
const terms = [
  { name: '钱包账户', plain: '一个用户在 COIN 币种下的余额总账。', attention: '未开户显示 0；首次充值审核成功时才安全开户。' },
  { name: '可用余额', plain: '用户当前可以使用的 COIN 数量。', attention: '当前业务主要看这个数字；不能手工直接修改。' },
  { name: '余额流水', plain: '每次加钱或冲正留下的一条永久记录。', attention: '流水按钱包版本连续排列，不能修改或删除。' },
  { name: '人工充值单', plain: '后台根据真实到账资金创建的充值申请。', attention: '创建申请不会立刻加钱，审核通过后才入账。' },
  { name: '资金凭证号', plain: '银行、支付宝、微信或内部收款渠道的真实交易编号。', attention: '全系统永久防重，不能用随机数代替，驳回后也不会释放。' },
  { name: '请求编号', plain: '一次按钮请求的防重编号，网络重试时使用。', attention: '它不能代替资金凭证号；同一笔钱仍看资金凭证号防重。' },
  { name: '冲正', plain: '用一条负数流水撤回以前错误入账的钱。', attention: '原记录仍保留，只允许整笔撤回，不能偷偷改历史。' },
  { name: '对账', plain: '检查余额、流水、订单和审核记录是否互相一致。', attention: '只检查不修复；发现异常要停用钱包并联系技术人员。' }
]
const flow = [
  { no: '1', title: '真实资金到账', text: '运营人员先在受控收款账户中看到真实到账，并取得不能随意替换的凭证。', warning: '没有到账，流程到这里就停止' },
  { no: '2', title: '创建充值申请', text: '填写用户账号、真实资金凭证号、整数金额、凭证引用和清楚的原因。此时用户余额不变。' },
  { no: '3', title: '独立审核', text: '审核人重新检查收款账户、交易号、金额和用户，不能只看申请人写的文字。' },
  { no: '4', title: '原子入账', text: '审核通过时，系统在同一个数据库事务里更新钱包、追加流水、更新订单、额度和审计；任何一步失败都会整体回滚。' },
  { no: '5', title: '查询确认', text: '操作后到钱包账户看余额，到余额流水看最新流水，到充值单详情看审核和审计记录。' }
]
const rechargeSteps = [
  { title: '第一步：找到正确用户', description: '可以用完整用户账号或完整E.164手机号查询。最好再核对用户昵称和绑定手机号，防止给错人。' },
  { title: '第二步：核对真实到账', description: '打开受控收款渠道，确认资金确实到达公司账户，金额和付款信息一致。聊天截图不能代替真实到账。' },
  { title: '第三步：填写唯一凭证号', description: '按运营统一规则填写真实交易号。系统会转成大写并永久防重，不要自己编造编号。' },
  { title: '第四步：上传并填写凭证引用', description: '凭证应放在受控媒体库或文件系统；不要使用会过期、可被别人替换或公开访问的临时链接。' },
  { title: '第五步：提交申请', description: '金额只填正整数，不加逗号和单位；原因要写明资金来源、用途和核对结果。提交后等待审核。' },
  { title: '第六步：审核并再次确认', description: '审核人逐项核对后才通过。通过后立即产生余额和流水；驳回时要写清原因，凭证号仍不能再用。' }
]
const pages = [
  { name: 'account', title: '钱包账户', when: '想看用户现在有多少钱、钱包是否停用，或需要做完整对账时。', search: '输入完整用户账号或完整手机号；两项都填时，必须指向同一个用户。', actions: '查看可用余额、冻结余额、版本和状态；对已开户钱包启用或停用；执行只读对账。', never: '不能直接加钱、减钱或给未开户用户用状态按钮开户。' },
  { name: 'ledger', title: '余额流水', when: '想知道余额为什么变化、是哪一单造成的、谁审核操作时。', search: '输入完整用户账号或完整手机号。使用上一页、下一页查看历史。', actions: '查看发生金额、变动前后余额、业务单号、操作管理员昵称和ID、时间与原因。', never: '不能修改或删除任何一条流水。发现错误应回到充值单创建冲正申请。' },
  { name: 'recharge', title: '人工充值单', when: '需要根据真实资金到账创建、审核、驳回充值，或对错误充值发起冲正时。', search: '可按用户账号、手机号、资金凭证号、订单号、状态或类型查询。', actions: '创建充值、审核通过、驳回、查看详情、创建全额冲正申请。', never: '不能把未到账款项当充值；不能重复使用凭证；不能用冲正隐藏操作错误。' }
]
const questions = [
  { q: '用户还没有钱包，为什么余额是 0？', a: '这是正常的“未开户零余额”。查询不会创建钱包。第一次合法充值审核成功时，系统会在同一事务里开户并入账。' },
  { q: '输入手机号查不到用户怎么办？', a: '确认使用完整E.164格式，例如 +60123456789，不要空格、短横线或本地号码开头。也可以改用用户公开账号查询。' },
  { q: '网络断了，我能重新点提交吗？', a: '先用资金凭证号查询原单。创建请求有请求编号幂等，真实资金还有凭证号防重；不要为了重试而更换凭证号。' },
  { q: '凭证填错并被驳回，能重新使用同一个凭证吗？', a: '不能。凭证号永久保留防重。请联系负责人核查并按异常流程处理，不能换大小写或加符号绕过。' },
  { q: '充值给错用户了怎么办？', a: '不要改余额或删流水。找到原充值单，创建全额冲正申请，由有权限的管理员审核。余额不足时需要先停止并联系负责人。' },
  { q: '对账出现异常怎么办？', a: '立即停止继续入账，必要时停用该钱包，保存页面上的异常标识、用户账号和时间，联系技术人员排查。不要自行改数据库。' }
]
</script>

<style scoped lang="scss">
.wallet-guide { max-width: 1180px; margin: 0 auto; color: var(--el-text-color-primary); }
.guide-hero { display: flex; align-items: center; justify-content: space-between; gap: 24px; padding: 30px; color: #fff; background: linear-gradient(125deg, #172554, #1d4ed8 58%, #0ea5e9); border-radius: 14px; box-shadow: 0 12px 32px rgb(30 64 175 / 20%); }
.guide-hero h1 { margin: 5px 0 10px; font-size: 30px; }
.guide-hero p { max-width: 780px; margin: 0; color: rgb(255 255 255 / 82%); font-size: 15px; line-height: 1.8; }
.guide-kicker { color: #bfdbfe; font-size: 13px; font-weight: 700; letter-spacing: 2px; }
.guide-alert { margin-top: 18px; }
.guide-section { margin-top: 18px; padding: 24px; background: var(--el-bg-color); border: 1px solid var(--el-border-color-lighter); border-radius: 12px; box-shadow: 0 4px 16px rgb(15 23 42 / 5%); }
.guide-section h2 { margin: 0 0 18px; font-size: 20px; }
.rule-grid, .explain-grid { display: grid; grid-template-columns: repeat(3, 1fr); gap: 14px; }
.rule-card { display: flex; gap: 12px; padding: 16px; background: var(--el-fill-color-light); border-radius: 9px; }
.rule-card > span, .flow-no { display: grid; place-items: center; flex: 0 0 34px; width: 34px; height: 34px; color: #fff; background: #2563eb; border-radius: 50%; font-weight: 700; }
.rule-card h3, .flow-step h3, .explain-grid h3, .danger-box h3 { margin: 3px 0 7px; font-size: 15px; }
.rule-card p, .flow-step p, .explain-grid p, .page-help p, .guide-section li, .guide-section :deep(.el-step__description) { margin: 0; color: var(--el-text-color-regular); font-size: 14px; line-height: 1.8; }
.flow-list { display: grid; gap: 12px; margin-bottom: 18px; }
.flow-step { display: flex; gap: 14px; padding: 15px; border-left: 3px solid #60a5fa; background: var(--el-fill-color-lighter); }
.danger-box { margin-top: 18px; padding: 16px 20px; color: var(--el-color-danger); background: var(--el-color-danger-light-9); border: 1px solid var(--el-color-danger-light-7); border-radius: 9px; }
.danger-box ul { margin: 8px 0 0; padding-left: 22px; }
.explain-grid article { padding: 18px; border: 1px solid var(--el-border-color); border-radius: 9px; }
.final-check { color: #fff; background: #7f1d1d; border-color: #991b1b; }
.final-check h2, .final-check p { color: #fff; }
.final-check p { margin: 0; font-size: 16px; font-weight: 600; line-height: 2; }
@media (max-width: 900px) { .guide-hero { align-items: flex-start; flex-direction: column; } .rule-grid, .explain-grid { grid-template-columns: 1fr; } .guide-section { padding: 17px; } }
</style>
