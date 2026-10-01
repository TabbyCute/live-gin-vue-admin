# Security Policy

## Reporting a Vulnerability

Please report security issues to qimiaojiangjizhao@gmail.com



/proxy/pages/income
/api/agent/center/income/overview


/proxy/pages/income-record
/shortVideo/api/agent/center/income/record/page


/shortVideo/api/agent/reward/calculator/calculate

十六、你现在最大的缺失：http_hooks
十七、我建议你暂时不要开 on_play

每个页面都需鉴权使用公共的组件user_auth_change处理；
部分页面要有个加载中的状态占位，防止数据加载成功后切换各种状态导致页面闪动；
在余额宝模块，状态管理中的获取余额的函数多次被高频触发，请优化；
Apis文件的apiCode不能重复，如果有重复请帮我修改；
余额宝部分使用的接口不能劫持接口请求报错，有公共的报错提示方便我通过apiCode找到报错位置；



请先参考我和gpt的对话，我后面会按照他的思路做：


我还想加一个录播功能，主播可以在直播间播放录播，说白了就是添加一个视频地址




请仅帮我完成下面这一步；
我希望一步一步的来请先帮我实现。请重新校验我的配置文件的live，有新增改动，下面的需要用到的尽可能的在面去取；
请先帮我在/session/prepare实现publishToken的生成。
publishToken由{
"ver": 1,
"scope": "live:publish",
"token_id": "82fd10bcfa8c4e28",
"session_no": "LS_20260923_00001",
"app": "tb_live",
"vhost": "push.xxx.com",
"stream": "A_1000009",
"credential_version": 1,
"iat": 1790204400,
"nbf": 1790204340,
"exp": 1790290800
}组成；把这个用hook-token加密进行加密，然后后面会在回调里面解密；
同时帮我在pushUrl中拼好推流token；

