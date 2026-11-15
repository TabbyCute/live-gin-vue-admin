# 提现加密密钥直接读取YAML配置

- 日期：2026-11-05。
- 用户要求：三个加密环境变量未设置，写入配置文件并补注释；随后明确“代码请直接读取配置”。此要求更新此前仅环境变量注入的约定。
- 实现：config.Server.Payout与config.Payout；InitializePayout显式传入当前YAML配置，LoadCrypto只读取active-key-id/encryption-keys/fingerprint-key，不再读取旧环境变量，不提供环境变量覆盖。
- 文件：server/config.prod.yaml、config.dev.yaml、config.docker.yaml分别填入独立的随机32字节AES和HMAC密钥，带用途、JSON/Base64格式、轮换和备份中文注释，权限0600。JSON密钥表保持字符串，防止Viper将key ID变成小写。不会在聊天、测试输出、审计或前端输出真实值。
- 保护：配置整段及字段json隐藏，后台读取不返回、提交不能绑定；系统配置保存显式跳过payout，防止密钥被清空或替换。原有密钥格式、历史key、密文与指纹核验保持。只在启动时加载，修改后重启。
- 准备：生成正式密钥前只读确认正式库payee/order/company/evidence四类加密记录均0；测试库只读检查未成功，未改数据库。没有覆盖已有payout配置或历史密钥。
- 私有备份：/Users/tabby/.codex/backups/live-gin-vue-admin/payout-keys-20261105/，目录0700、文件0600；不在本记忆记录密钥正文。
- 验证：config/core/payout/system/initialize相关包测试、定向race、go vet及全项目Go构建通过；覆盖YAML大小写key ID、加解密、旧环境变量不生效、错误配置拒绝、配置API隐藏和保存保护。三份真实YAML均通过密钥格式及API不暴露校验。
- 边界：本次没有发布或重启运行服务，既有打包脚本直接复制所选完整YAML，因此新构建会包含对应配置；当前旧二进制需要重新构建后才能使用YAML密钥。PAYOUT_EVIDENCE_DIR仍按人工出款存储约定单独配置。
